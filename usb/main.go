package main

import (
	"os"
	"path/filepath"
	"log"
)

const (
	BuildDir    = "build"
	BuildLib    = "lib"
	ScriptsDir  = "scripts"
	Executables = "bin"
	ImageType   = "usb"
)

func mountImage(output_dev, root, stateful string) error {

	os.MkdirAll(root, 755)
	os.MkdirAll(stateful, 755)

	if err := FSMount(output_dev + "1", stateful, "ext4", 0); err != nil {
		log.Fatal(err)
		return err
	}

	return FSMount(output_dev + "3", root, "ext4", 0)
}

func umountImage(root, stateful string) error {

	if err := FSUmount(root); err != nil {

		return err
	}

	return FSUmount(stateful)
}

func createBaseImage(image_name string, output_dev string) error {
	info("Using image type " + image_name)
	os.Mkdir(BuildDir, 0755)

	rootDir := BuildDir + "/rootfs"
	statefulDir := BuildDir + "/stateful"

	os.MkdirAll(rootDir, 0755)
	os.MkdirAll(statefulDir, 0755)

	if err := buildGptImage(output_dev, ImageType); err != nil {
		return err
	}

	if err := mountImage(output_dev, rootDir, statefulDir); err != nil {
		return err
	}


	// TODO (@laconicpneumonic)
	// Copy U-Root Filesystem to rootfs


	if err := os.Chown(filepath.Join(rootDir, PartitionScriptPath), 0, 0); err != nil {
		return err
	}

	// TODO (@laconicpneumonic)
	// Add DM Verity hashes

	umountImage(rootDir, statefulDir)
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if err := createBaseImage(os.Args[1], os.Args[2]); err != nil {
		log.Fatalf("LOL: %v", err)
	}
}
