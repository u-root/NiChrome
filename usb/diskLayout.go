package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"os/exec"
)

const (
	PartitionScriptPath = "usr/sbin/write_gpt.sh"
	diskLayoutPath      = "./legacy_disk_layout.json"
)

func cgpt(args ...string) (string, string, error) {
	return execute("./cgpt.py", args...)
}

func mkfs(format string, args ...string) (string, string, error) {
	return execute("mkfs."+format, args...)
}

func getPartitions(imageType string) (int, error) {
	out, _, err := cgpt("readpartitionnums", imageType, diskLayoutPath)
	if err != nil {
		return 0, err
	}

	val, err := strconv.Atoi(out)
	if err != nil {
		return 0, err
	}

	return val, nil
}

func writePartitionScript(image_type string, path string) error {

	tmpFile, err := ioutil.TempFile("", "tmp")
	if err != nil {
		log.Fatal(err)
		return err
	}

	defer tmpFile.Close()

	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		log.Fatal(err)
		return err
	}

	if out, stderror, err := cgpt("write", image_type, diskLayoutPath, tmpFile.Name()); err != nil {
		log.Fatal(err, out, stderror)
		return err
	}

	if err := os.Rename(tmpFile.Name(), path); err != nil {
		log.Fatal(err)
		return err
	}

	return os.Chmod(path, 444)
}

func runPartitionScript(outdev string, path string) error {
	//if _, _, err := execute(path); err != nil {
	//	log.Fatal(err)
	//	return err
	//}
	//if _, _, err := execute("write_partition_table", outdev, "/dev/zero"); err != nil {
	//	log.Fatal(err)
	//	return err
	//}
	//return nil

	cmd := exec.Command("/bin/sh", "-c", path + ";", "write_partition_table", outdev, "/dev/zero")

	return cmd.Run()
}

func formatMountText(dir, label, size_b, start_b, target string) string {

	mountRawText :=
		`(
		mkdir -p %[1]s
		m=( sudo mount -o loop,offset=%[3]s,sizelimit=%[2]s %[5]s %[1]s )
		if ! "${m[@]}"; then
		if ! "${m[@]}" -o ro; then
		rmdir %[1]s
		exit 0
		fi
		fi
		ln -sfT %[1]s "%[1]s_%[2]s"
		) &`

	return fmt.Sprintf(mountRawText, dir, start_b, size_b, label, target)

}

func formatUnpackText(ddArgs, file, label, start, target string) string {
	unpackRawText :=
		`
		dd if=%[5]s of=%[3]s %[1]s skip=%[4]s
		ln -sfT %[2]s "%[2]s_%[3]s"
		`
	return fmt.Sprintf(unpackRawText, ddArgs, file, label, start, target)
}

func formatPackText(ddArgs, file, start, target string) string {

	packRawText :=
		`
		dd if=%[2]s of=%[4]s %[1]s seek=%[3]s conv=notrunc
		`
	return fmt.Sprintf(packRawText, ddArgs, file, start, target)
}

func formatUmountText(dir, label string) string {
	umountRawText :=
		`
		if [[ -d %[1]s ]]; then
		  (
		  sudo umount %[1]s || :
		  rmdir %[1]s
		  rm -f "%[1]s_%[2]s"
		  ) &
		fi
		`
	return fmt.Sprintf(umountRawText, dir, label)
}

func formatHeaderText(label, part string) string {
	headerRawText :=
		`
	case ${PART:-%[2]s} in
	%[2]s|"%[1]s")
	`
	return fmt.Sprintf(headerRawText, label, part)
}
func emitGPTScripts(outdev string, directory string) error {
	templateContents, err := ioutil.ReadFile("template/GPT")
	if err != nil {
		log.Fatal(err)
		return err
	}

	cgptOutput, stderror, err := execute("./cgpt", "show", outdev)
	if err != nil {
		fmt.Println(outdev)
		log.Fatal(cgptOutput, stderror, err)
		return err
	}

	FrontOfLine, err := regexp.Compile("^")
	if err != nil {
		log.Fatal(err)
		return err
	}

	formattedOutput := FrontOfLine.ReplaceAllLiteralString(cgptOutput, "# ")

	pack := "pack_partitions.sh"
	unpack := "unpack_partitions.sh"
	mount := "mount_image.sh"
	umount := "unmount_image.sh"
	names := []string{pack, unpack, mount, umount}

	for _, name := range names {
		ioutil.WriteFile(name, templateContents, 777)
		f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatal(err)
			return err
		}

		if _, err = f.WriteString(formattedOutput); err != nil {
			log.Fatal(err)
			return err
		}
		f.Close()
	}

	cgptShowOutput, _, err := cgpt("show", "-q="+outdev)
	if err != nil {
		log.Fatal(err)
		return err
	}

	whiteSpace, err := regexp.Compile("\\s+")

	outputScanner := bufio.NewScanner(strings.NewReader(cgptShowOutput))
	for outputScanner.Scan() {
		values := whiteSpace.Split(outputScanner.Text(), -1)
		if len(values) < 4 {
			log.Fatalf("Somethings Wrong with cgpt")
		}
		start := values[0]
		size := values[1]
		part := values[2]
		file := "part_" + part
		dir := "dir_" + part
		target := "${TARGET}"
		ddArgs := "bs=512 count=" + size
		startB := ""
		sizeB := ""

		if val, err := strconv.Atoi(start); err != nil {
			startB = strconv.Itoa(val * 512)
		} else {
			log.Fatal("Somethings Wrong with cgpt")
		}

		if val, err := strconv.Atoi(size); err != nil {
			sizeB = strconv.Itoa(val * 512)
		} else {
			log.Fatal("Somethings Wrong with cgpt")
		}

		label, _, err := cgpt("show", outdev, "-i="+part, "-l")
		if err != nil {
			log.Fatal("Something is Wrong with cgpt")
		}

		headerText := formatHeaderText(label, part)
		unpackText := formatUnpackText(ddArgs, file, label, start, target)
		packText := formatPackText(ddArgs, file, start, target)
		mountText := formatMountText(dir, label, sizeB, startB, target)
		umountText := formatUmountText(dir, label)

		for _, name := range names {
			f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				log.Fatal(err)
				return err
			}

			if _, err = f.WriteString(headerText); err != nil {
				log.Fatal(err)
				return err
			}

			switch name {
			case pack:
				f.WriteString(packText)

			case unpack:
				f.WriteString(unpackText)
			}
			if val, err := strconv.Atoi(size); err != nil {
				log.Fatal("Something is Wrong with cgpt")
			} else if val > 1 {
				switch name {
				case mount:
					f.WriteString(mountText)
				case umount:
					f.WriteString(umountText)
				}
			}

			if _, err = f.WriteString("esac\n"); err != nil {
				log.Fatal(err)
				return err
			}

			f.Close()
		}
	}

	for _, name := range []string{mount, umount} {
		f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatal(err)
			return err
		}

		if _, err = f.WriteString("wait\n"); err != nil {
			log.Fatal(err)
			return err
		}
	}

	for _, name := range names {
		os.Chmod(name, 777)
	}

	return nil

}

func mkFS(imageFile, imageType, partNum string) error {

	FSFormat, _, err := cgpt("readfsformat", imageType, diskLayoutPath, partNum)
	if err != nil {
		log.Fatal(err)
		return err
	}

	FSOptions, _, err := cgpt("readfsoptions", imageType, diskLayoutPath, partNum)
	if err != nil {
		log.Fatal(err)
		return err
	} else if len(FSOptions) == 0 {
		return nil
	}

	FSBytes, _, err := cgpt("readpartsize", imageType, diskLayoutPath, partNum)
	if err != nil {
		log.Fatal(err)
		return err
	}

	FSBlockSize, _, err := cgpt("readfsblocksize", diskLayoutPath)
	if err != nil {
		log.Fatal(err)
		return err
	}

	bytes, err := strconv.Atoi(FSBytes)
	if err != nil {
		log.Fatal(err)
		return err
	}

	blocks, err := strconv.Atoi(FSBlockSize)
	if err != nil {
		log.Fatal(err)
		return err
	}

	if bytes < blocks {
		return nil
	}

	FSLabel, _, err := cgpt("readlabel", imageType, diskLayoutPath, partNum)
	if err != nil {
		log.Fatal(err)
		return err
	}

	FSUUID, _, err := cgpt("readuuid", imageType, diskLayoutPath, partNum)
	if err != nil {
		log.Fatal(err)
		return err
	}

	FSType, _, err := cgpt("readuuid", imageType, diskLayoutPath, partNum)
	if err != nil {
		log.Fatal(err)
		return err
	}

	partOffsetString, _, err := cgpt("show", "-s", "-i", partNum, imageType)
	if err != nil {
		log.Fatal(err)
		return err
	}

	val, err := strconv.Atoi(partOffsetString)
	if err != nil {
		log.Fatal(err)
		return err
	}

	offset := strconv.Itoa(val * 512)

	partDev, _, err := execute("losetup", "-f", "--show", "--offset="+offset, "--sizelimit="+FSBytes, imageFile)
	if err != nil {
		log.Fatal(err)
		return err
	} else if len(partDev) == 0 {
		return fmt.Errorf("Somethings wrong with losetup")
	}

	switch FSFormat {
	case "ext2", "ext3", "ext4":
		UUIDOption := ""
		if FSUUID == "clear" {
			FSUUID = "00000000-0000-0000-0000-000000000000"
		}

		if FSUUID != "random" {
			UUIDOption = "-U " + FSUUID
		}

		val := strconv.FormatFloat(float64(bytes)/float64(blocks), 'E', -1, 64)
		mkfs(FSFormat, "-F", "-q", "-O", "ext_attr", UUIDOption, "-E lazy_itable_init=0", "-b "+FSBlockSize, partDev, val)

		execute("tune2fs", "-L", FSLabel, "-c 0", "-i 0", "-T 20091119110000", "-m 0", "-r 0", "-e remount-ro", partDev, FSOptions, "</dev/null")

	case "fat12", "fat16", "fat32":
		mkfs("vfat", "-F "+FSFormat, "-n "+FSLabel, partDev, FSOptions)
	case "fat", "vfat":
		mkfs("vfat", "-n "+FSLabel, partDev, FSOptions)
	case "squashfs":
		squashDir, err := ioutil.TempDir("/tmp", "")
		if err != nil {
			log.Fatal(err)
			return err
		}
		squashFile, err := ioutil.TempFile("/tmp", "")
		if err != nil {
			log.Fatal(err)
			return err
		}

		os.Chmod(squashDir, 0755)

		if _, _, err := execute("mksquashfs", squashDir, squashFile.Name(), "-noappend", "-all-root", "-no-progress", "-no-recovery", FSOptions); err != nil {
			log.Fatal(err)
			return err
		}

		if _, _, err := execute("dd", "if="+squashFile.Name(), "of="+partDev, "bs=4096", "status=none"); err != nil {
			log.Fatal(err)
			return err
		}

		if _, _, err := mkfs(FSFormat, "-b "+FSBytes, "-d single", "-m single", "-M", "-L "+FSLabel, "-O "+FSOptions, partDev); err != nil {
			log.Fatal(err)
			return err
		}

		if err := os.Remove(squashFile.Name()); err != nil {
			log.Fatal(err)
			return err
		}

	default:
		return fmt.Errorf("Not a recognized file type: %s", FSFormat)
	}

	mountDir, err := ioutil.TempDir("/tmp", "")
	if err != nil {
		log.Fatal(err)
		return err
	}

	FSMount(partDev, mountDir, FSFormat, syscall.MS_RDONLY)
	if err := os.Chown(mountDir, 0, 0); err != nil {
		log.Fatal(err)
		return err
	}

	if FSLabel == "STATE" {
		os.MkdirAll(filepath.Join(mountDir, "dev_image"), 0755)
		os.MkdirAll(filepath.Join(mountDir, "var_overlay"), 0755)
	}

	if FSType == "rootfs" {
		os.MkdirAll(filepath.Join(mountDir, "mnt", "stateful_partition"), 0755)
		os.MkdirAll(filepath.Join(mountDir, "usr", "local"), 0755)
		os.MkdirAll(filepath.Join(mountDir, "usr", "share", "oem"), 0755)
		os.MkdirAll(filepath.Join(mountDir, "var"), 0755)
	}

	FSUmount(partDev, mountDir, FSFormat, FSOptions)
	os.RemoveAll(mountDir)

	return nil
}

func buildGptImage(outdev, diskLayout string) error {
	partitionScriptPath := filepath.Join(BuildDir, "partition_script.sh")

	fmt.Println("Writing script")
	if err := writePartitionScript(diskLayout, partitionScriptPath); err != nil {
		log.Fatal(err)
		return err
	}

	fmt.Println("Running partition scripts")
	if err := runPartitionScript(outdev, partitionScriptPath); err != nil {
		log.Fatal(err)
		return err
	}

	fmt.Println("Emitting GPT scripts")
	if err := emitGPTScripts(outdev, filepath.Dir(outdev)); err != nil {
		log.Fatal(err)
		return err
	}

	fmt.Println("Getting partitions")
	partitions, err := getPartitions(diskLayout)
	if err != nil {
		log.Fatal(err)
		return err
	}

	for i := 1; i <= partitions; i++ {
		mkFS(outdev, diskLayout, strconv.Itoa(i))
	}

	if _, _, err := execute("./cgpt", "add", "-i 2", "-S 1", outdev); err != nil {
		log.Fatal(err)
		return err
	}

	return nil
}
