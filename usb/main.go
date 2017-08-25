package main

import (
	os "os"
	"fmt"
	"path/filepath"
)

const (
	BuildDir    = "build"
	BuildLib    = "lib"
	ScriptsDir  = "scripts"
	Executables = "bin"
	ImageType   = "usb"
)

func mount_image(args ...string) error {
	_, error, err := execute("mount_image.sh", args...)

	if err != nil {
		return err
	}

	if len(error) != 0 {
		return fmt.Errorf(error)
	}

	return nil
}

func createBaseImage(image_name string, rootfs_verfication bool, bootcache bool, output_dev string) error {
	// checkValidLayout(image_name)
	info("Using image type " + image_name)
	os.Mkdir(BuildDir, 0755)

	rootDir := BuildDir + "/rootfs"
	statefulDir := BuildDir + "/stateful"
	espDir := BuildDir + "/esp"

	os.Mkdir(rootDir, 0755)
	os.Mkdir(statefulDir, 0755)
	os.Mkdir(espDir, 0755)

	buildGptImage(output_dev, ImageType)

	if err := mount_image(output_dev, rootDir, statefulDir, espDir); err != nil {
		return err
	}

	// Insert pubkey?
	// Write boot desc not neccesary if script is run from start to finish.
	// Write partition scripts

	if err := writePartitionScript(ImageType, filepath.Join(rootDir, PartitionScriptPath)); err !=  nil {
		return err
	}

	if err := os.Chown(filepath.Join(rootDir, PartitionScriptPath), 0, 0); err != nil {
		return err
	}

	


	return nil

}

func main() {
	execute("echo", "hello")
	execute("id")
}
