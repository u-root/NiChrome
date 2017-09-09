package main

//include a loading bar
//TODO: Use filePath.join
//automated "yes"er updating the config file
//TODO check if it is a device
//TODO append method name to error
//TODO proper output channels when you run commands
//TODO in the newest kernel pull the stable one if it fails, then go back to what was there, see the notes on the PR)
import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	configTxt = []byte(`loglevel=7
init=/init
rootwait
`)
	fetch         = flag.Bool("fetch", true, "Fetch all the things we need")
	keys          = flag.String("keys", "vboot_reference/tests/devkeys", "where the keys live")
	device        = flag.String("device", "", "What device to use, default is to ask you")
	kernelVersion = "4.12.7"
	workingDir    = ""
	linuxVersion  = "linux_stable"
	homeDir       = ""
	packageList   = []string{
		"git", "golang", "build-essential", "git-core", "gitk", "git-gui", "subversion", "curl", "python2.7", "libyaml-dev", "liblzma-dev", "uuid-dev"}
)

func cp(inputLoc string, outputLoc string) error {
	// Don't check for an error, there are all kinds of
	// reasons a remove can fail even if the file is
	// writeable
	os.Remove(outputLoc)

	if _, err := os.Stat(inputLoc); err != nil {
		return err
	}
	fileContent, err := ioutil.ReadFile(inputLoc)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(outputLoc, fileContent, 0777)
}

func tildeExpand(input string) string {
	if strings.Contains(input, "~") {
		input = strings.Replace(input, "~", homeDir, 1)
		fmt.Printf("Full filepath is : %s", input)
	}
	return input
}

func setup() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	workingDir = dir
	fmt.Printf("Working dir is %s\n", workingDir)
	usr, err := user.Current()
	if err != nil {
		return err
	}
	homeDir = usr.HomeDir
	fmt.Printf("Home dir is %s\n", homeDir)
	return nil
}

func aptget() error {
	fmt.Printf("Using apt-get to get %v\n", packageList)
	get := []string{"apt-get", "-y", "install"}
	get = append(get, packageList...)
	cmd := exec.Command("sudo", get...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil

}

func cleanup() error {
	filesToRemove := [...]string{linuxVersion, "linux-stable", "NiChrome", "vboot_reference"}
	fmt.Printf("-------- Removing problematic files %v\n", filesToRemove)
	for _, file := range filesToRemove {
		if _, err := os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				continue
			}
		}
		err := os.RemoveAll(file)
		if err != nil {
			return err
		}
	}
	return nil
}

func goCompatibility() error {
	fmt.Printf("--------Checking Go Compatibility \n")
	cmd := exec.Command("go", "version")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return err
	}
	// The string is originally in the form: go version go1.9rc2_cl165246139 linux/amd64 where 1.9 is the go version
	termString, err := strconv.ParseFloat(strings.Split(out.String(), " ")[2][2:5], 64)
	if err != nil {
		return err
	}
	if termString > 1.7 {
		fmt.Println("Compatible go version")
	} else {
		return errors.New("Please install go v1.7 or greater.")
	}
	return nil
}

func goGet() error {
	cmd := exec.Command("go", "get", "github.com/u-root/u-root/")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func goBuild() error {
	fmt.Printf("--------Got u-root \n")
	bbpath := filepath.Join(os.Getenv("GOPATH"), "src/github.com/u-root/u-root/bb")
	cmd := exec.Command("go", "build", "-x", ".")
	cmd.Dir = bbpath
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	// We need to run bb in the bb directory. Kind of a flaw in its
	// operation. Sorry.
	cmd = exec.Command("./bb", append([]string{"-add", workingDir + ":lib " + workingDir + ":usr "}, cmdlist...)...)
	cmd.Dir = bbpath
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if _, err := os.Stat("/tmp/initramfs.linux_amd64.cpio"); err != nil {
		return err
	}
	fmt.Printf("Created the initramfs in /tmp/")
	return nil
}

func kernelGet() error {
	var args = []string{"clone", "--depth", "1", "-b", "v4.12.7", "git://git.kernel.org/pub/scm/linux/kernel/git/stable/linux-stable.git"}
	fmt.Printf("-------- Getting the kernel via git %v\n", args)
	cmd := exec.Command("git", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("didn't clone kernel %v", err)
		return err
	}
	return nil
}

func buildKernel() error {
	if err := os.Symlink("/tmp/initramfs.linux_amd64.cpio", fmt.Sprintf("%s/initramfs.linux_amd64.cpio", "linux-stable")); err != nil {
		fmt.Printf("[warning only] Error creating symlink for initramfs: %v", err)
	}
	// NOTE: don't get confused. This means that .config in linux-stable
	// points to CONFIG, i.e. where we are.
	if err := cp("CONFIG", "linux-stable/.config"); err != nil {
		fmt.Printf("[warning only] Error creating symlink for .config: %v", err)
	}

	cmd := exec.Command("make", "--directory", "linux-stable", "-j64")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join("linux-stable", "/arch/x86/boot/bzImage")); err != nil {
		return err
	}
	fmt.Printf("bzImage created")
	return nil
}

func getVbutil() error {
	fmt.Printf("-------- Building in Vbutil\n")
	cmd := exec.Command("git", "clone", "https://chromium.googlesource.com/chromiumos/platform/vboot_reference")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func buildVbutil() error {
	cmd := exec.Command("git", "checkout", "3f3a496a23088731e4ab5654b02fbc13a6881c65")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Dir = "vboot_reference"
	if err := cmd.Run(); err != nil {
		fmt.Printf("couldn't checkout the right branch")
		return err
	}
	cmd = exec.Command("make", "-j64")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Dir = "vboot_reference"
	if err := cmd.Run(); err != nil {
		fmt.Printf("Make failed. Please try to manually install vbutil")
		return err
	}
	return nil

}

func vbutilIt() error {
	newKern := "newKern"
	if err := ioutil.WriteFile("config.txt", configTxt, 0644); err != nil {
		return err
	}
	if err := ioutil.WriteFile("nocontent.efi", []byte("no content"), 0777); err != nil {
		return err
	}
	bzImage := "linux-stable/arch/x86/boot/bzImage"
	fmt.Printf("Bz image is located at %s \n", bzImage)
	keyblock := filepath.Join(*keys, "kernel.keyblock")
	sign := filepath.Join(*keys, "kernel_data_key.vbprivk")
	cmd := exec.Command("./vboot_reference/build/futility/futility", "vbutil_kernel", "--pack", newKern, "--keyblock", keyblock, "--signprivate", sign, "--version", "1", "--vmlinuz", bzImage, "--bootloader", "nocontent.efi", "--config", "config.txt", "--arch", "x86")
	stdoutStderr, err := cmd.CombinedOutput()
	fmt.Printf("%s\n", stdoutStderr)
	if err != nil {
		return err
	}
	if err = dd(); err != nil {
		return err
	}
	return nil
}

func dd() error {
	for *device == "" {
		var location string
		fmt.Printf("Where do you want to put this kernel ")
		_, err := fmt.Scanf("%s", &location)
		if err != nil {
			return err
		}
		if _, err = os.Stat(location); err != nil {
			fmt.Printf("Please provide a valid location name. %s has error %v", location, err)
		} else {
			*device = location
		}
	}
	fmt.Printf("Running dd to put the new kernel onto the desired location on the usb.\n")
	args := []string{"dd", "if=newKern", "of=" + *device}
	msg, err := exec.Command("sudo", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dd %v failed: %v: %v", args, string(msg), err)
	}
	fmt.Printf("%v ran ok\n", args)
	return nil
}

//TODO : final Error
//TODO: absolute filepath things
func allFunc() error {
	var cmds = []struct {
		f      func() error
		skip   bool
		ignore bool
		n      string
	}{
		{f: cleanup, skip: !*fetch, ignore: false, n: "cleanup"},
		{f: setup, skip: false, ignore: false, n: "setup"},
		{f: aptget, skip: !*fetch, ignore: false, n: "apt get"},
		{f: goCompatibility, skip: false, ignore: true, n: "Check Go Version"},
		{f: goGet, skip: !*fetch, ignore: false, n: "Get u-root source"},
		{f: goBuild, skip: false, ignore: false, n: "Build u-root source"},
		{f: kernelGet, skip: !*fetch, ignore: false, n: "Git clone the kernel"},
		{f: buildKernel, skip: false, ignore: false, n: "build the kernel"},
		{f: getVbutil, skip: !*fetch, ignore: false, n: "git clone vbutil"},
		{f: buildVbutil, skip: false, ignore: false, n: "build vbutil"},
		{f: vbutilIt, skip: false, ignore: false, n: "vbutil and create a kernel image"},
	}

	for _, c := range cmds {
		log.Printf("-----> Step %v: ", c.n)
		if c.skip {
			log.Printf("-------> Skip")
			continue
		}
		log.Printf("----------> Start")
		err := c.f()
		if c.ignore {
			log.Printf("----------> Ignore result")
			continue
		}
		if err != nil {
			return fmt.Errorf("%v: %v", c.n, err)
		}
		log.Printf("----------> Finished %v\n", c.n)
	}
	return nil
}

func main() {
	flag.Parse()
	log.Printf("Using kernel %v\n", kernelVersion)
	if err := allFunc(); err != nil {
		log.Fatalf("fail error is : %v", err)
	}
	log.Printf("execution completed successfully\n")
}
