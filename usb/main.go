package main

import (
	os "os"
	"fmt"
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

func createBaseImage(image_name string, output_dev string) error {
	// checkValidLayout(image_name)
	info("Using image type " + image_name)
	os.Mkdir(BuildDir, 0755)

	rootDir := BuildDir + "/rootfs"
	statefulDir := BuildDir + "/stateful"
	espDir := BuildDir + "/esp"

	os.Mkdir(rootDir, 0755)
	os.Mkdir(statefulDir, 0755)
	os.Mkdir(espDir, 0755)

	if err := buildGptImage(filepath.Dir(output_dev), ImageType); err != nil {
		return err
	}

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
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	/*out, error, err := execute(os.Args[1], os.Args[1:]...)
	fmt.Println(out, error)
	if err != nil {

		log.Fatal(err)
	}

	out, _, _ = execute("id")
	fmt.Println(out)
	*/

	if err := createBaseImage(os.Args[1], os.Args[2]); err != nil {
		log.Fatalf("LOL: %v", err)
	}
}
