// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

//include a loading bar
//TODO proper output channels when you run commands
//TODO in the newest kernel pull the stable one if it fails, then go back to what was there, see the notes on the PR)
import (
	"bytes"
	"encoding/json"
	"fmt"
	flag "github.com/spf13/pflag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/u-root/u-root/pkg/mount/gpt"
)

const initramfs = "initramfs.linux_amd64.cpio"

var (
	configTxt = `loglevel=1
	init=/init
rootwait
`
	apt      = flag.Bool("apt", true, "apt-get all the things we need")
	fetch    = flag.Bool("fetch", false, "Fetch all the things we need")
	skiproot = flag.Bool("skiproot", false, "Don't put the root onto usb")
	skipkern = flag.Bool("skipkern", false, "Don't put the kern onto usb")
	keys     = flag.String("keys", "vboot/tests/devkeys", "where the keys live")
	dev      = flag.String("dev", "/dev/null", "What device to use")
	config   = flag.String("config", "CONFIG", "Linux config file")
	extra    = flag.String("extra", "", "Comma-separated list of extra packages to include")
	kernDev  string
	rootDev  string
	kernPart = 2

	kernelVersion = "v5.10"
	workingDir    = ""
	linuxVersion  = "linux-stable"
	homeDir       = ""
	packageList   = []string{
		"bc",
		"git",
		"golang",
		"build-essential",
		"git-core",
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
	threads = runtime.NumCPU() + 2 // Number of threads to use when calling make.
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

func checkDevice(n string) error {
	if filepath.Dir(n) != "/dev" {
		return nil
	}
	fi, err := os.Stat(n)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeDevice != os.ModeDevice {
		return fmt.Errorf("%q is in /dev and is not a device?", n)
	}
	return nil
}

func setup() error {
	dir, err := os.Getwd()
	syscall.Umask(0)
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
	if err := checkDevice(kernDev); err != nil {
		return err
	}
	if err := checkDevice(rootDev); err != nil {
		return err
	}
	return nil
}

func aptget() error {
	missing := []string{}
	for _, packageName := range packageList {
		cmd := exec.Command("dpkg", "-s", packageName)
		if err := cmd.Run(); err != nil {
			missing = append(missing, packageName)
		}
	}

	if len(missing) == 0 {
		fmt.Println("No missing dependencies to install")
		return nil
	}

	fmt.Printf("Using apt-get to get %v\n", missing)
	get := []string{"apt-get", "-y", "install"}
	get = append(get, missing...)
	cmd := exec.Command("sudo", get...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()

}

func cleanup() error {
	filesToRemove := [...]string{"linux-stable", "vboot", "linux-firmware"}
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
	return nil
	cmd := exec.Command("go", append([]string{"get", "github.com/u-root/u-root"}, dynamicCmdList...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func goBuildStatic() error {
	oFile := filepath.Join(workingDir, "linux-stable", initramfs)
	n, err := filepath.Glob("../u-root/cmds/[ce]*/*")
	if err != nil {
		return fmt.Errorf("../u-root/cmds/[ce]/*: %v", err)
	}
	args := append([]string{"-o", oFile}, n...)
	cmd := exec.Command("u-root", append(args, staticCmdList...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("Created %v\n", oFile)
	return nil
}

// A new change: we no longer need to add u-root to this partition,
// as we build it all for the "static" case.
func goBuildDynamic() error {
	args := []string{"-o", filepath.Join(workingDir, initramfs), "-nocmd", "-files", "rootfs:", "-files", "/usr/bin/bash", "-base", "/dev/null"}
	log.Print("WARNING: skipping pkg/sos/html; fix me")
	if false {
		args = append(args, "-files", "pkg/sos/html:etc/sos/html")
	}
	log.Printf("Run u-root with args %v", args)
	cmd := exec.Command("u-root", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("Created %v", initramfs)
	return nil
}

func getSUIDbinaries() error {
	if err := os.MkdirAll("usr/bin", 0755); err != nil {
		return err
	}
	binaries := []string{"/bin/fusermount"}
	for _, b := range binaries {
		cmd := exec.Command("sudo", "rsync", "-av", b, "rootfs/usr/bin/")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			return err
		}
	}
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
	var args = []string{"clone", "--depth", "1", "-b", kernelVersion, "git://git.kernel.org/pub/scm/linux/kernel/git/stable/" + linuxVersion + ".git"}
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
	// load NiChrome logo into kernel
	cp("logo/logo_linux_clut224.ppm", "linux-stable/drivers/video/logo/logo_linux_clut224.ppm")

	cmd := exec.Command("make", "--directory", "linux-stable", "-j"+strconv.Itoa(threads))
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
	cmd := exec.Command("git", "clone", "https://github.com/coreboot/vboot")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func buildVbutil() error {
	cmd := exec.Command("make", "-j"+strconv.Itoa(threads))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Dir = "vboot"
	if err := cmd.Run(); err != nil {
		fmt.Printf("Make failed. Please try to manually install vbutil")
		return err
	}
	return nil

}

func installUrootGpt() error {
	cmd := exec.Command("go", "install", "-x", "github.com/u-root/u-root/cmds/core/gpt/")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func vbutilIt() error {
	// Try to read a GPT header from our output file. If we can, add a root_guid
	// to config.txt, otherwise, don't bother.
	var pt = &gpt.PartitionTable{}
	var pg gpt.GUID
	if *dev == "/dev/null" {
		log.Printf("Skipping the gpt step, since *dev is /dev/null")
	} else {
		gptcmd, err := exec.LookPath("gpt")
		if err != nil {
			return fmt.Errorf("Can not find a GPT command: %v", err)
		}
		args := []string{gptcmd, *dev}
		msg, err := exec.Command("sudo", args...).Output()
		if err != nil {
			return fmt.Errorf("gpt %v failed: %v", args, err)
		}

		if err := json.NewDecoder(bytes.NewBuffer(msg)).Decode(&pt); err != nil {
			log.Printf("Reading in GPT JSON, warning only: %v", err)
		} else if pt.Primary == nil || pt.MasterBootRecord == nil {
			// With new changes to u-root/gpt we need to check to see if the contents
			// of the struct are nil
			log.Printf("Unable to parse GPT header of %v", *dev)
		} else {
			pg = gpt.GUID(pt.Primary.Parts[kernPart-1].UniqueGUID)
			// We may not be able to read a GPT, consider the case that dev is /dev/null.
			// But it is an error for it to be zero if we succeeded in reading it.
			var zeropg gpt.GUID
			if pg == zeropg {
				log.Fatalf("Partition GUID for part %d is zero", kernPart-1)
			}
		}
	}
	newKern := "newKern"
	configTxt := fmt.Sprintf("%sguid_root=%s\n", configTxt, pg.String())
	if err := ioutil.WriteFile("config.txt", []byte(configTxt), 0644); err != nil {
		return err
	}
	if err := ioutil.WriteFile("nocontent.efi", []byte("no content"), 0777); err != nil {
		return err
	}
	bzImage := "linux-stable/arch/x86/boot/bzImage"
	fmt.Printf("Bz image is located at %s \n", bzImage)
	keyblock := filepath.Join(*keys, "recovery_kernel.keyblock")
	sign := filepath.Join(*keys, "recovery_kernel_data_key.vbprivk")
	cmd := exec.Command("./vboot/build/futility/futility", "vbutil_kernel", "--pack", newKern, "--keyblock", keyblock, "--signprivate", sign, "--version", "1", "--vmlinuz", bzImage, "--bootloader", "nocontent.efi", "--config", "config.txt", "--arch", "x86")
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
	return run("tcz", append([]string{"-d", "-i=false", "-r=rootfs/tcz"}, tczList...)...)
}

func check() error {
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
		{f: getSUIDbinaries, skip: *skiproot, ignore: false, n: "Get SUID binaries"},
		{f: chrome, skip: true || *skiproot || !*fetch, ignore: false, n: "to hell with chrome -- let's get firefox"},
		{f: aptget, skip: !*apt, ignore: false, n: "apt get"},
		{f: goBuildDynamic, skip: *skiproot, ignore: false, n: "Build dynamic initramfs"},
		{f: rootdd, skip: *skiproot, ignore: false, n: "Put the tcz cpio onto the stick"},
		{f: kernelGet, skip: *skipkern || !*fetch, ignore: false, n: "Git clone the kernel"},
		{f: goBuildStatic, skip: *skipkern, ignore: false, n: "Build static initramfs"},
		{f: firmwareGet, skip: *skipkern || !*fetch, ignore: false, n: "Git clone the firmware files"},
		{f: buildKernel, skip: *skipkern, ignore: false, n: "build the kernel"},
		{f: getVbutil, skip: *skipkern || !*fetch, ignore: false, n: "git clone vbutil"},
		{f: buildVbutil, skip: *skipkern, ignore: false, n: "build vbutil"},
		{f: installUrootGpt, skip: *skipkern, ignore: false, n: "install u-root gpt"},
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
	if *extra != "" {
		dynamicCmdList = append(dynamicCmdList, strings.Split(*extra, ",")...)
	}
	log.Printf("Using kernel %v\n", kernelVersion)
	if err := allFunc(); err != nil {
		log.Fatalf("fail error is : %v", err)
	}
	log.Printf("execution completed successfully\n")
}
