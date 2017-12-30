// +build go1.9

package main

//include a loading bar
//TODO proper output channels when you run commands
//TODO in the newest kernel pull the stable one if it fails, then go back to what was there, see the notes on the PR)
import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/u-root/u-root/pkg/gpt"
)

const initramfs = "initramfs.linux_amd64.cpio"

var (
	configTxt = `loglevel=1
	init=/init
rootwait
`
	apt      = flag.Bool("apt", false, "apt-get all the things we need")
	fetch    = flag.Bool("fetch", false, "Fetch all the things we need")
	skiproot = flag.Bool("skiproot", false, "Don't put the root onto usb")
	skipkern = flag.Bool("skipkern", false, "Don't put the kern onto usb")
	keys     = flag.String("keys", "vboot_reference/tests/devkeys", "where the keys live")
	dev      = flag.String("dev", "/dev/null", "What device to use")
	config   = flag.String("config", "CONFIG", "Linux config file")
	kernDev  string
	rootDev  string
	kernPart = 2

	kernelVersion = "4.12.7"
	workingDir    = ""
	linuxVersion  = "linux_stable"
	homeDir       = ""
	packageList   = []string{
		"bc",
		"git",
		"golang",
		"build-essential",
		"git-core",
		"gitk",
		"git-gui",
		"subversion",
		"curl",
		"python2.7",
		"libyaml-dev",
		"liblzma-dev",
		"uuid-dev",
		"libssl-dev",
	}
	optional = map[string]bool{
		".ssh":   true,
		"upspin": true,
		"etc":    true,
	}
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

	if *dev == "/dev/null" {
		kernDev, rootDev = *dev, *dev
		return nil
	}
	kernDev, rootDev = *dev+"2", *dev+"3"
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
	filesToRemove := [...]string{linuxVersion, "linux-stable", "NiChrome", "vboot_reference", "linux-firmware"}
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

func goGet() error {
	cmd := exec.Command("go", "get", "github.com/u-root/u-root/", "github.com/u-root/wingo", "upspin.io/cmd/...", "github.com/nsf/godit")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func goBuildStatic() error {
	oFile := filepath.Join(workingDir, "linux-stable", initramfs)
	bbpath := filepath.Join(os.Getenv("GOPATH"), "src/github.com/u-root/u-root")
	cmd := exec.Command("go", append([]string{"run", "u-root.go", "-o", oFile, "-build=bb"}, staticCmdList...)...)
	cmd.Dir = bbpath
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("Created %v\n", oFile)
	return nil
}

func goBuildDynamic() error {
	optional := []string{"usr", "lib", "tcz", "etc", "upspin", ".ssh"}
	var extraFiles string
	for _, v := range optional {
		if _, err := os.Stat(v); err != nil {
			continue
		}
		extraFiles = fmt.Sprintf("%s %s:%s", extraFiles, filepath.Join(workingDir, v), v)
	}
	bbpath := filepath.Join(os.Getenv("GOPATH"), "src/github.com/u-root/u-root")
	cmd := exec.Command("go", append([]string{"run", "u-root.go", "-o", filepath.Join(workingDir, initramfs), "-files", extraFiles}, dynamicCmdList...)...)
	cmd.Dir = bbpath
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("Created %v", initramfs)
	return nil
}

func chrome() error {
	if err := os.MkdirAll("usr/bin", 0755); err != nil {
		return err
	}
	resp, err := http.Get("https://nichromeos.org/index.php/s/NEC5bZOqm0atPYe/download")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("non-200 HTTP status: %d", resp.StatusCode)
	}
	o, err := os.OpenFile("usr/bin/chrome", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer o.Close()
	if _, err := io.Copy(o, resp.Body); err != nil {
		return err
	}
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

func firmwareGet() error {
	var args = []string{"clone", "git://git.kernel.org/pub/scm/linux/kernel/git/iwlwifi/linux-firmware.git"}
	fmt.Printf("-------- Getting the firmware via git %v\n", args)
	cmd := exec.Command("git", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("didn't clone firmware %v", err)
		return err
	}
	return nil
}
func buildKernel() error {
	if err := cp(*config, "linux-stable/.config"); err != nil {
		fmt.Printf("copying %v to linux-stable/.config: %v", *config, err)
	}

	cmd := exec.Command("make", "--directory", "linux-stable", "-j64")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// TODO: this is OK for now. Later we'll need to do something
	// with a map and GOARCH.
	cmd.Env = append(os.Environ(), "ARCH=x86_64")
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
	// Try to read a GPT header from our output file. If we can, add a root_guid
	// to config.txt, otherwise, don't bother.
	args := []string{filepath.Join(os.Getenv("GOPATH"), "bin/gpt"), *dev}
	msg, err := exec.Command("sudo", args...).Output()
	if err != nil {
		log.Printf("gpt %v failed (warning only): %v", args, err)
	}
	var pg uuid.UUID
	if err == nil {
		var g = make([]gpt.GPT, 2)
		if err := json.NewDecoder(bytes.NewBuffer(msg)).Decode(&g); err != nil {
			log.Printf("Reading in GPT JSON, warning only: %v", err)
		} else {
			pg = uuid.UUID(g[0].Parts[kernPart-1].UniqueGUID)
			// We may not be able to read a GPT, consider the case that dev is /dev/null.
			// But it is an error for it to be zero if we succeeded in reading it.
			var zeropg uuid.UUID
			if pg == zeropg {
				log.Fatalf("Partition GUID for part %d is zero", kernPart-1)
			}
		}
	}
	newKern := "newKern"
	configTxt := fmt.Sprintf("%sguid_root=%s\n", configTxt, pg)
	if err := ioutil.WriteFile("config.txt", []byte(configTxt), 0644); err != nil {
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
	return err
}

func dd(name, dev, file string) error {
	fmt.Printf("Running dd to put %v onto %v", file, dev)
	args := []string{"dd", "if=" + file, "of=" + dev}
	msg, err := exec.Command("sudo", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dd %v failed: %v: %v", args, string(msg), err)
	}
	return nil
}

func kerndd() error {
	return dd("Kernel image", kernDev, "newKern")
}

func rootdd() error {
	return dd("tcz initramfs CPIO archive", rootDev, filepath.Join(workingDir, "initramfs.linux_amd64.cpio"))
}

func lsr(dirs []string, w io.Writer) error {
	var err error
	for _, n := range dirs {
		err = filepath.Walk(n, func(name string, fi os.FileInfo, err error) error {
			if err != nil {
				if optional[name] {
					return nil
				}
				return err
			}
			if fi.IsDir() {
				return nil
			}
			fmt.Fprintf(w, "%v\n", name)
			return nil
		})
	}
	return err
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v %v: %v", name, args, err)
	}
	return nil
}

func tcz() error {
	t := filepath.Join(os.Getenv("GOPATH"), "bin/tcz")
	if _, err := os.Stat(t); err != nil {
		// let's try to be nice about this
		if err := run("go", "install", "github.com/u-root/u-root/cmds/tcz"); err != nil {
			return fmt.Errorf("Building tcz: %v", err)
		}
	}
	if _, err := os.Stat(t); err != nil {
		return err
	}
	return run(t, append([]string{"-d", "-i=false", "-r=tcz"}, tczList...)...)
}

func check() error {
	if os.Getenv("GOPATH") == "" {
		return fmt.Errorf("You have to set GOPATH.")
	}
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
		{f: check, skip: false, ignore: false, n: "check environment"},
		{f: setup, skip: false, ignore: false, n: "setup"},
		{f: cleanup, skip: *skipkern || *skiproot || !*fetch, ignore: false, n: "cleanup"},
		{f: goGet, skip: *skipkern || !*fetch, ignore: false, n: "Get u-root source"},
		{f: tcz, skip: *skiproot || !*fetch, ignore: false, n: "run tcz to create the directory of packages"},
		{f: chrome, skip: *skiproot || !*fetch, ignore: false, n: "Fetch chrome"},
		{f: aptget, skip: !*apt, ignore: false, n: "apt get"},
		{f: goBuildDynamic, skip: *skiproot, ignore: false, n: "Build dynamic initramfs"},
		{f: rootdd, skip: *skiproot, ignore: false, n: "Put the tcz cpio onto the stick"},
		{f: kernelGet, skip: *skipkern || !*fetch, ignore: false, n: "Git clone the kernel"},
		{f: goBuildStatic, skip: *skipkern, ignore: false, n: "Build static initramfs"},
		{f: firmwareGet, skip: *skipkern || !*fetch, ignore: false, n: "Git clone the firmware files"},
		{f: buildKernel, skip: *skipkern, ignore: false, n: "build the kernel"},
		{f: getVbutil, skip: *skipkern || !*fetch, ignore: false, n: "git clone vbutil"},
		{f: buildVbutil, skip: *skipkern, ignore: false, n: "build vbutil"},
		{f: vbutilIt, skip: *skipkern, ignore: false, n: "vbutil and create a kernel image"},
		{f: kerndd, skip: *skipkern, ignore: false, n: "Put the kernel image onto the stick"},
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
