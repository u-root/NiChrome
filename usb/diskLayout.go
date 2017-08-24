package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"bufio"
	"strings"
	"log"
	"strconv"
	"fmt"
)

const (
	diskLayoutPath = "./legacy_disk_layout.json"
)
func cgpt(args ...string) (string, string, error){
	return execute("./cgpt.py", args...)
}
func checkValidLayout(imageType string) {
	cgpt("layout", imageType, diskLayoutPath)
}

func writePartitionScript(image_type string, path string) error {

	tmpFile, err := ioutil.TempFile("", "tmp")
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(path), 0777)
	cgpt("write", image_type, diskLayoutPath, tmpFile.Name())
	os.Rename(tmpFile.Name(), path)
	os.Chmod(path, 444)

	return nil
}

func runPartitionScript(outdev string, path string) error {
	if _, _, err := execute(path); err != nil {
		return err
	}
	if _, _, err := execute("write_partition_table", outdev, "/dev/zero"); err != nil {
		return err
	}
	return nil
}

func formatMountText(dir, label, size_b, start_b, target string) string {

	mountRawText :=
		`(
		mkdir -p %[1]
		m=( sudo mount -o loop,offset=%[3],sizelimit=%[2] %[5] %[1] )
		if ! "${m[@]}"; then
		if ! "${m[@]}" -o ro; then
		rmdir %[1]
		exit 0
		fi
		fi
		ln -sfT %[1] "%[1]_%[2]"
		) &`

	return fmt.Sprintf(mountRawText, dir, start_b, size_b, label)

}

func formatUnpackText(ddArgs, file, label, start, target string) string {
	unpackRawText :=
		`
		dd if=%[5] of=%[3] %[1] skip=%[4]
		ln -sfT %[2] "%[2]_%[3]"
		`
	return fmt.Sprintf(unpackRawText, ddArgs, file, label, start, target)
}

func formatPackText(ddArgs, file, start, target string) string {

	packRawText :=
		`
		dd if=%[2] of=%[4] %[1] seek=%[3] conv=notrunc
		`
	return fmt.Sprintf(packRawText, ddArgs, file, start, target)
}

func formatUmountText(dir, label string) string {
	umountRawText :=
		`
		if [[ -d %[1] ]]; then
		  (
		  sudo umount %[1] || :
		  rmdir %[1]
		  rm -f "%[1]_%[2]"
		  ) &
		fi
		`
	return fmt.Sprintf(umountRawText, dir, label)
}

func formatHeaderText(label, part string) string {
	headerRawText :=
	`
	case ${PART:-%s[2]} in
	%[2]|"%[1]")
	`
	return fmt.Sprintf(headerRawText, label, part)
}
func emitGPTScripts(outdev string, directory string) error {
	templateContents, err := ioutil.ReadFile("templates/GPT")
	if err != nil {
		return err
	}

	cgptOutput, _, err := cgpt("show", outdev)
	if err != nil {
		return err
	}

	FrontOfLine, err := regexp.Compile("^")
	if err != nil {
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
			return err
		}

		if _, err = f.WriteString(formattedOutput); err != nil {
			return err
		}
		f.Close()
	}

	cgptShowOutput, _, err := cgpt("show", "-q=" + outdev)
	if err != nil {
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

		if val, err := strconv.Atoi(start);  err != nil {
			startB = strconv.Itoa(val * 512)
		} else {
			log.Fatal("Somethings Wrong with cgpt")
		}

		if val, err := strconv.Atoi(size);  err != nil {
			sizeB = strconv.Itoa(val * 512)
		} else {
			log.Fatal("Somethings Wrong with cgpt")
		}

		label, _, err := cgpt("show", outdev, "-i=" + part, "-l")
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
				return err
			}

			if _, err = f.WriteString(headerText); err != nil {
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
				return err
			}



			f.Close()
		}
	}

	for _, name := range []string{mount, umount} {
		f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}

		if _, err = f.WriteString("wait\n"); err != nil {
			return err
		}
	}

	for _, name := range names {
		os.Chmod(name, 777)
	}

	return nil

}

func buildGptImage(outdev string, diskLayout string) {
	partitionScriptPath := filepath.Join(outdev, "partition_script.sh")

	writePartitionScript(diskLayout, partitionScriptPath)
	runPartitionScript(outdev, partitionScriptPath)

	emitGPTScripts(outdev, filepath.Dir(outdev))
}
