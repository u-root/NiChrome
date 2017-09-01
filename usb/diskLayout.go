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
)

const (
	PartitionScriptPath = "usr/sbin/write_gpt.sh"
	diskLayoutPath      = "./legacy_disk_layout.json"
)

var (
	removeWhiteSpace = regexp.MustCompile("\\s+")
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
		return fmt.Errorf("writePartitionScript: %v", err)

	}

	defer tmpFile.Close()

	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return fmt.Errorf("writePartitionScript: %v", err)

	}

	if stdout, stderror, err := cgpt("write", image_type, diskLayoutPath, tmpFile.Name()); err != nil {
		return fmt.Errorf("writePartitionScript: %v\n%v\n%v", stdout, stderror, err)

	}

	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("writePartitionScript: %v", err)

	}

	return os.Chmod(path, 444)
}

func runPartitionScript(outdev string, path string) error {
	if stdout, stderr, err := execute("/bin/sh", path, outdev, "/dev/zero"); err != nil {
		return fmt.Errorf("runPartitionScript: %v\n%v\n%v", stdout, stderr, err)
	}

	return nil
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
		return fmt.Errorf("emitGPTScripts: %v", err)

	}

	pack := filepath.Join(directory, "pack_partitions.sh")
	unpack := filepath.Join(directory, "unpack_partitions.sh")
	mount := filepath.Join(directory, "mount_image.sh")
	umount := filepath.Join(directory, "unmount_image.sh")
	names := []string{pack, unpack, mount, umount}

	for _, name := range names {
		ioutil.WriteFile(name, templateContents, 777)
		f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("emitGPTScripts: %v", err)
		}

		f.Close()
	}

	cgptShowOutput, _, err := execute("./cgpt", "show", "-q", outdev)
	if err != nil {
		return err
	}

	whiteSpace, err := regexp.Compile("\\s+")

	outputScanner := bufio.NewScanner(strings.NewReader(cgptShowOutput))
	for outputScanner.Scan() {
		values := whiteSpace.Split(outputScanner.Text(), -1)
		if len(values) < 6 {
			log.Fatalf("Somethings Wrong with cgpt")
		}
		start := values[1]
		size := values[2]
		part := values[3]
		file := "part_" + part
		dir := "dir_" + part
		target := "${TARGET}"
		ddArgs := "bs=512 count=" + size
		startB := ""
		sizeB := ""

		if val, err := strconv.Atoi(start); err == nil {
			startB = strconv.Itoa(val * 512)
		} else {
			return fmt.Errorf("emitGPTScripts %v", err)
		}

		if val, err := strconv.Atoi(size); err == nil {
			sizeB = strconv.Itoa(val * 512)
		} else {
			return fmt.Errorf("emitGPTScripts %v", err)
		}

		label, stderror, err := execute("./cgpt", "show", outdev, "-i "+part, "-l")
		if err != nil {
			return fmt.Errorf("emitGPTScripts %v\n%v", stderror, err)
		}

		headerText := formatHeaderText(label, part)
		unpackText := formatUnpackText(ddArgs, file, label, start, target)
		packText := formatPackText(ddArgs, file, start, target)
		mountText := formatMountText(dir, label, sizeB, startB, target)
		umountText := formatUmountText(dir, label)

		for _, name := range names {
			f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return fmt.Errorf("emitGPTScripts %v", err)

			}

			if _, err = f.WriteString(headerText); err != nil {
				return fmt.Errorf("emitGPTScripts %v", err)

			}

			switch name {
			case pack:
				f.WriteString(packText)

			case unpack:
				f.WriteString(unpackText)
			}
			if val, err := strconv.Atoi(size); err != nil {
				return fmt.Errorf("emitGPTScripts %v", err)
			} else if val > 1 {
				switch name {
				case mount:
					f.WriteString(mountText)
				case umount:
					f.WriteString(umountText)
				}
			}

			if _, err = f.WriteString("esac\n"); err != nil {
				return fmt.Errorf("emitGPTScripts %v", err)

			}

			f.Close()
		}
	}

	for _, name := range []string{mount, umount} {
		f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("emitGPTScripts %v", err)

		}

		if _, err = f.WriteString("wait\n"); err != nil {
			return fmt.Errorf("emitGPTScripts %v", err)
		}
	}

	for _, name := range names {
		os.Chmod(name, 777)
	}

	return nil

}

func mkFS(imageFile, imageType, partNum string) error {

	FSFormatRaw, _, err := cgpt("readfsformat", imageType, diskLayoutPath, partNum)
	FSFormat := removeWhiteSpace.ReplaceAllString(FSFormatRaw, "")
	if err != nil {
		return fmt.Errorf("mkFS %v", err)

	} else if len(FSFormat) == 0 {
		return nil
	}

	FSOptionsRaw, _, err := cgpt("readfsoptions", imageType, diskLayoutPath, partNum)
	FSOptions := removeWhiteSpace.ReplaceAllString(FSOptionsRaw, "")
	if err != nil {
		return fmt.Errorf("mkFS %v", err)

	}

	FSBytesRaw, _, err := cgpt("readpartsize", imageType, diskLayoutPath, partNum)
	FSBytes := removeWhiteSpace.ReplaceAllString(FSBytesRaw, "")
	if err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	FSBlockSizeRaw, _, err := cgpt("readfsblocksize", diskLayoutPath)
	FSBlockSize := removeWhiteSpace.ReplaceAllString(FSBlockSizeRaw, "")
	if err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	bytes, err := strconv.Atoi(FSBytes)
	if err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	blocks, err := strconv.Atoi(FSBlockSize)
	if err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	if bytes < blocks {
		return nil
	}

	FSLabelRaw, _, err := cgpt("readlabel", imageType, diskLayoutPath, partNum)
	FSLabel := removeWhiteSpace.ReplaceAllString(FSLabelRaw, "")
	if err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	FSUUIDRaw, _, err := cgpt("readuuid", imageType, diskLayoutPath, partNum)
	FSUUID := removeWhiteSpace.ReplaceAllString(FSUUIDRaw, "")
	if err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	FSTypeRaw, _, err := cgpt("readuuid", imageType, diskLayoutPath, partNum)
	FSType := removeWhiteSpace.ReplaceAllString(FSTypeRaw, "")
	if err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	partOffsetStringRaw, _, err := execute("./cgpt", "show", "-s", "-i", partNum, imageFile)
	partOffsetString := removeWhiteSpace.ReplaceAllString(partOffsetStringRaw, "")
	if err != nil {
		info(partOffsetString)
		return fmt.Errorf("mkFS %v", err)
	}

	val, err := strconv.Atoi(partOffsetString)
	if err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	offset := strconv.Itoa(val * 512)

	partDevRaw, _, err := execute("losetup", "-f", "--show", "--offset="+offset, "--sizelimit="+FSBytes, imageFile)
	partDev := removeWhiteSpace.ReplaceAllString(partDevRaw, "")
	if err != nil {
		return fmt.Errorf("mkFS %v", err)
	} else if len(partDev) == 0 {
		return fmt.Errorf("losetup: no device")
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

		val := strconv.Itoa(int(float64(bytes) / float64(blocks)))
		if stdout, stderr, err := execute("/bin/sh", "-c", "mkfs."+FSFormat+" -F -q -O ext_attr "+UUIDOption+" -E lazy_itable_init=0 -b "+FSBlockSize+" "+partDev+" "+val); err != nil {
			return fmt.Errorf("mkFS %v\n%v", stderr, err)

		} else {
			log.Printf("stdout: '%s', stderr: '%s'\n", stdout, stderr)
		}

		if stdout, stderr, err := execute("/bin/sh", "-c", "tune2fs -L "+FSLabel+" -c 0 -i 0 -T 20091119110000 -m 0 -r 0 -e remount-ro "+partDev+" "+FSOptions); err != nil {
			info("tune2fs -L " + FSLabel + " -c 0 -i 0 -T 20091119110000 -m 0 -r 0 -e remount-ro " + partDev + " " + FSOptions)
			return fmt.Errorf("mkFS %v\n%v", stderr, err)

		} else {
			log.Printf("stdout: '%s', stderr: '%s'\n", stdout, stderr)
		}

	case "fat12", "fat16", "fat32":
		execute("/bin/sh", "-c", "mkfs.vfat -F "+FSFormat+" -n "+FSLabel+" "+partDev+" "+FSOptions)
	case "fat", "vfat":
		execute("/bin/sh", "-c", "mkfs.vfat -n "+FSLabel+" "+partDev+" "+FSOptions)
	case "squashfs":
		squashDir, err := ioutil.TempDir("/tmp", "")
		if err != nil {
			return fmt.Errorf("mkFS %v", err)

		}
		squashFile, err := ioutil.TempFile("/tmp", "")
		if err != nil {
			return fmt.Errorf("mkFS %v", err)
		}

		os.Chmod(squashDir, 0755)

		if _, _, err := execute("mksquashfs", squashDir, squashFile.Name(), "-noappend", "-all-root", "-no-progress", "-no-recovery", FSOptions); err != nil {
			return fmt.Errorf("mkFS %v", err)
		}

		if _, _, err := execute("dd", "if="+squashFile.Name(), "of="+partDev, "bs=4096", "status=none"); err != nil {
			return fmt.Errorf("mkFS %v", err)
		}

		if _, _, err := mkfs(FSFormat, "-b "+FSBytes, "-d single", "-m single", "-M", "-L "+FSLabel, "-O "+FSOptions, partDev); err != nil {
			return fmt.Errorf("mkFS %v", err)
		}

		if err := os.Remove(squashFile.Name()); err != nil {
			return fmt.Errorf("mkFS %v", err)
		}

	default:
		return fmt.Errorf("Not a recognized file type: %s", FSFormat)
	}

	mountDir, err := ioutil.TempDir("/tmp", "")
	if err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	if err := FSMount(partDev, mountDir, FSFormat, 0); err != nil {
		return fmt.Errorf("mkFS %v", err)
	}
	if err := os.Chown(mountDir, 0, 0); err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	if FSLabel == "STATE" {
		if err := os.MkdirAll(filepath.Join(mountDir, "dev_image"), 0755); err != nil {
			return fmt.Errorf("mkFS %v", err)
		}
		if err := os.MkdirAll(filepath.Join(mountDir, "var_overlay"), 0755); err != nil {
			return fmt.Errorf("mkFS %v", err)
		}
	}

	if FSType == "rootfs" {
		if err := os.MkdirAll(filepath.Join(mountDir, "mnt", "stateful_partition"), 0755); err != nil {
			return fmt.Errorf("mkFS %v", err)
		}
		if err := os.MkdirAll(filepath.Join(mountDir, "usr", "local"), 0755); err != nil {
			return fmt.Errorf("mkFS %v", err)
		}
		if err := os.MkdirAll(filepath.Join(mountDir, "usr", "share", "oem"), 0755); err != nil {
			return fmt.Errorf("mkFS %v", err)
		}
		if err := os.MkdirAll(filepath.Join(mountDir, "var"), 0755); err != nil {
			return fmt.Errorf("mkFS %v", err)
		}
	}

	if err := FSUmount(mountDir); err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	if err := os.RemoveAll(mountDir); err != nil {
		return fmt.Errorf("mkFS %v", err)
	}

	if _, stderror, err := execute("losetup", "-d", partDev); err != nil {
		return fmt.Errorf("mkFS %v\n%v", stderror, err)

	}

	return nil
}

func buildGptImage(outdev, diskLayout string) error {
	partitionScriptPath := filepath.Join(BuildDir, "partition_script.sh")

	info("Writing script")
	if err := writePartitionScript(diskLayout, partitionScriptPath); err != nil {
		return fmt.Errorf("buildGptImage %v", err)

	}

	file, err := os.OpenFile(partitionScriptPath, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		return fmt.Errorf("buildGptImage %v", err)
	}

	if _, err := file.WriteString("write_partition_table $1 $2\n"); err != nil {
		return fmt.Errorf("buildGptImage %v", err)
	}

	file.Close()

	info("Running partition scripts")
	if err := runPartitionScript(outdev, partitionScriptPath); err != nil {
		return fmt.Errorf("buildGptImage %v", err)
	}

	info("Emitting GPT scripts")
	if err := emitGPTScripts(outdev, BuildDir); err != nil {
		return fmt.Errorf("buildGptImage %v", err)
	}

	info("Making FS")

	for i := 1; i <= 12; i++ {
		if err := mkFS(outdev, diskLayout, strconv.Itoa(i)); err != nil {
			return fmt.Errorf("buildGptImage %v", err)
		}
	}

	if stdout, stderror, err := execute("./cgpt", "add", "-i", "2", "-S", "1", outdev); err != nil {
		warning(stdout, stderror, err)
		warning("repairing image")

		if _, stderror, err := execute("./cgpt", "repair", "-v", outdev); err != nil {
			return fmt.Errorf("buildGptImage %v\n%v", stderror, err)
		}

		if _, stderror, err := execute("./cgpt", "add", "-i", "2", "-S", "1", outdev); err != nil {
			return fmt.Errorf("buildGptImage %v\n%v", stderror, err)
		}
	}

	return nil
}
