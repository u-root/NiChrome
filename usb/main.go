package main

const (
	BuildDir    = "build"
	BuildLib    = "lib"
	ScriptsDir  = "scripts"
	Executables = "bin"
	ImageType   = "usb"
)

func createBaseImage(image_name string, rootfs_verfication bool, bootcache bool, output_dev string) {
	checkValidLayout(image_name)
	info("Using image type " + image_name)
	os.Mkdir(BuildDir, 0777)

	rootDir := BuildDir + "/rootfs"
	statefulDir := BuildDir + "/stateful"
	espDir := BuildDir + "/esp"

	os.Mkdir(rootDir, 0777)
	os.Mkdir(statefulDir, 0777)
	os.Mkdir(espDir, 0777)

	buildGptImage(output_dev, ImageType)

	mount_image(output_dev, rootDir, statefulDir, espDir)
}

func main() {
	execute("echo", "hello")
	execute("id")
}
