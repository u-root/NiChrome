package main

//include a loading bar
//TODO: Use filePath.join
//automated "yes"er updating the config file
//TODO check if it is a device
//TODO append method name to error
import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"errors"
	"log"
	"path/filepath"
	"os/user"
	//"bufio"
)

var kernelVersion = "4.12.7"
var workingDir = ""
var linuxVersion = fmt.Sprintf("linux-%s", kernelVersion)
var homeDir = ""


func setup() error{
	fmt.Printf("-------- Setting up \n")
	dir,err := os.Getwd(); 
	if err != nil {
		return err
	}
	workingDir = dir
	fmt.Printf("Working dir is %s\n", workingDir)
	cmd16 := exec.Command("sudo", "apt-get", "install", "git", "golang", "build-essential", "git-core", "gitk", "git-gui", "subversion", "curl", "python2.7", "libyaml-dev", "liblzma-dev") 
	err = cmd16.Run() 
	if err != nil { 
		return err
	}
	/*err = blankBootstick()
	if err != nil {
		return err
	}*/
	usr, err := user.Current()
    	if err != nil {
        	return err
    	}
	homeDir = usr.HomeDir
	return nil

}

//User input for putting custom chrome image on bootstick
func blankBootstick() error{
	fmt.Printf("-------- Creating bootstick \n")
	var imageLoc string
	var location string 	
	for true {
		fmt.Printf("What image would you like to put onto your bootstick (provide location for iso file)?\n")	
		_, err := fmt.Scanf("%s",&imageLoc)
		if err != nil {
			return err	
		}
		if _, err = os.Stat(imageLoc); err != nil{
			fmt.Printf("Please provide a valid file name. %s has error %v", imageLoc, err)
		} else {
			break
		}
	}
	for true {
		fmt.Printf("Where is your bootstick (/dev/sda?)")	
		_, err := fmt.Scanf("%s",&location)
		if err != nil {
			return err	
		}
		if _, err = os.Stat(location); err != nil{
			fmt.Printf("Please provide a valid location name. %s has error %v", location, err)
		} else {
			break
		}
	}
	cmd20 := exec.Command("sudo", "dd", fmt.Sprintf("if=%s", imageLoc), fmt.Sprintf("of=%s", location))
	err := cmd20.Run()
	if err != nil {
		return err
	}
	return nil	
}


func cleanup() error {	
	filesToRemove := [...]string{linuxVersion, "NiChrome", "vboot_reference"}
	fmt.Printf("-------- Removing problematic files %v\n", filesToRemove)
	for _,file := range filesToRemove{
		if _, err := os.Stat(file); err != nil {
			continue		
		} 
		err := os.Remove(file)
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
	fmt.Printf("--------Getting u-root \n")
	cmd1 := exec.Command("go", "get", "github.com/u-root/u-root")
	err := cmd1.Run()
	//TODO: Figure why this ^ isn't working
	/*if err != nil {
		return err
	}*/
	fmt.Printf("--------Gotu-root \n")
	gopath := fmt.Sprintf("GOPATH=%s/go", homeDir)
	bbpath := fmt.Sprintf("GOPATH=%s/go/src/github.com/u-root/u-root/bb/bb", homeDir)
	cmd3 := exec.Command(gopath, bbpath)
	err = cmd3.Run()
	if err != nil {
		return err
	}
	fmt.Printf("--------Getting bb \n")
/* Removing in favor of bb
	cmd4 := exec.Command("go", "run", "scripts/ramfs.go")
	err = cmd4.Run()
	if err != nil {
		return err
	}
*/
	if _, err := os.Stat("/tmp/initramgs.linux_amd64.cpio"); err != nil {
		return err
	}
	fmt.Printf("Created the initramfs in /tmp/")
	return nil
}

// Get the right kernel for the current config from github (needs work)
// https://github.com/LaconicPneumonic/linux-4.12.7.git
func kernelGet() error {
	fmt.Printf("-------- Getting the kernel \n")
	cmd8 := exec.Command("git", "clone", "https://github.com/LaconicPneumonic/linux-4.12.7.git")
	err := cmd8.Run()
	if err != nil {
		fmt.Printf("didn't clone kernel from Anthony's repo")
		return err
	}
	return nil
}



// Get the right kernel for the current config in NiChrome (Needs work)
func linuxKernelGet() error {
	fmt.Printf("-------- Getting the kernel \n")
	fmt.Printf("Current version of Kernel is %v\n", kernelVersion)
	name := fmt.Sprintf("%s.tar", linuxVersion)
	out, err := os.Create(name)
	fmt.Printf("Kernel saved in %s\n", name)
	if err != nil {
		return err
	}
	defer out.Close()
	linkName := fmt.Sprintf("%s%s%s", "https://cdn.kernel.org/pub/linux/kernel/v4.x/linux-", kernelVersion, ".tar.xz")
	fmt.Printf("Pulling from link %s\n", linkName)
	resp, err := http.Get(linkName)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}
	cmd5 := exec.Command("tar", "xf", fmt.Sprintf("%s.tar",linuxVersion))
	err = cmd5.Run()
	if err != nil {
		return err
	}
	return nil
}

// To get the newest Kernel from kernel.org
func newestkernelGet() error {
	jsonFile, err := http.Get("http://www.kernel.org/releases.json")
	if err != nil {
		return err
	}
	defer jsonFile.Body.Close()
	d, err := ioutil.ReadAll(jsonFile.Body)
	version := strings.Split(strings.SplitAfter(strings.Split(strings.Split(string(d), "\"moniker\": \"stable\",")[1], "\n")[1], "https:")[1], "\",")[0]
	fmt.Printf("\n Version of Kernel is %v", version)
	name := fmt.Sprintf("%s", "linux.tar")
	out, err := os.Create(name)
	fmt.Printf("\n Kernel saved in %s", name)
	if err != nil {
		return err
	}
	defer out.Close()
	resp, err := http.Get(fmt.Sprintf("http:%s", version))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	cmd5 := exec.Command("tar", "xf", fmt.Sprintf("%s.tar",linuxVersion))
	err = cmd5.Run()
	if err != nil {
		return err
	}
	return nil
}


func unpackKernel() error {
	fmt.Printf("-------- Unpack the kernel\n")
	cmd8 := exec.Command("git", "clone", "https://github.com/u-root/NiChrome.git")
	err := cmd8.Run()
	if err != nil {
		fmt.Printf("didn't clone Nichrome")
		return err
	}
	cmd9 := exec.Command("cp", "NiChrome/CONFIG", fmt.Sprintf("%s/.config", linuxVersion))
	err = cmd9.Run()
	if err != nil {
		fmt.Println("3")
		return err
	}
	cmd10 := exec.Command("cp", "/tmp/initramfs.linux_amd64.cpio", fmt.Sprintf("%s/", linuxVersion))
	err = cmd10.Run()
	if err != nil {
		fmt.Println("2")
		return err
	}
	fmt.Printf("pwd before make is %s\n", workingDir)	
	cmd11 := exec.Command("make", "--directory", linuxVersion, "-j64")
	err = cmd11.Run()
	if err != nil {
		fmt.Println("4")
		return err
	}
	if _, err := os.Stat(filepath.Join(linuxVersion, "/arch/x86/boot/bzImage")); err != nil {
		return err
	}
	fmt.Printf("bzImage created")
	return nil
}



// TODO: add the chroot to github
func findVbutil() error{
	path, err := exec.LookPath("futility")
	if err != nil {
		log.Fatal("Make the chromium package to access Futility")
	}
	fmt.Printf("futility is available at %s\n", path)
	return nil
}

func buildVbutil() error{
	fmt.Printf("-------- Building in Vbutil\n")
	cmd8 := exec.Command("git", "clone", "https://chromium.googlesource.com/chromiumos/platform/vboot_reference")
	err := cmd8.Run()
	if err != nil {
		fmt.Printf("didn't get chromium repo")
		return err
	}
	return nil
	cmd9 := exec.Command("git", "checkout", "3f3a496a23088731e4ab5654b02fbc13a6881c65")
	err = cmd9.Run()
	if err != nil {
		fmt.Printf("couldn't checkout the right branch")
		return err
	}
	return nil
	cmd10 := exec.Command("make", "-j64")
	err = cmd10.Run()
	if err != nil {
		fmt.Printf("Make failed. Please try to manually install vbutil")
		return err
	}
	return nil

}

func vbutilIt() error {
	fmt.Printf("-------- VBUTILING\n")
	buildVbutil()
	findVbutil()
	fmt.Printf("-------- VBUTILING  contd. \n")
	newKern := "newKern"
	if err := ioutil.WriteFile("config.txt", []byte("loglevel=7"), 0777); err != nil {
		return err
	}
	if err := ioutil.WriteFile("nocontent.efi", []byte("no content"), 0777); err != nil {
		return err
	}
	bzImage := fmt.Sprintf("%s/arch/x86/boot/bzImage",linuxVersion)
	fmt.Printf("Bz image is located at %s \n", bzImage)
	cmd14 := exec.Command("futility", "vbutil_kernel",  "--pack", newKern, "--keyblock", "/usr/share/vboot/devkeys/kernel.keyblock", "--signprivate", "/usr/share/vboot/devkeys/kernel_data_key.vbprivk", "--version", "1", "--vmlinuz", "linux-4.12.7/arch/x86/boot/bzImage" , "--bootloader", "nocontent.efi", "--config", "config.txt",  "--arch", "x86")	
	stdoutStderr, err := cmd14.CombinedOutput()	
	fmt.Printf("%s\n", stdoutStderr)
	if err != nil {
		fmt.Printf("\ny\n")
		return err
	}
	dd()
	return nil
}

func dd() error{
	var location = "/dev/sda2"
	for true {
		fmt.Printf("Where do you want to put this kernel (%s)", location)	
		_, err := fmt.Scanf("%s",&location)
		if err != nil {
			return err	
		}
		if _, err = os.Stat(location); err != nil{
			fmt.Printf("Please provide a valid location name. %s has error %v", location, err)
		} else {
			break
		}
	}
	ofLocation := fmt.Sprintf("of%s", location)
	cmd15 := exec.Command("sudo", "dd", "if=newKern", ofLocation)
	err := cmd15.Run()
	if err != nil {
		return err
	}
	return nil
}

//TODO : final Error
func allFunc() error {
	if err:= cleanup(); err != nil{
		log.Printf("ERROR: %v\n", err)		
	}
	if err:= setup(); err != nil{
		log.Printf("ERROR: %v\n", err)		
	}
	if err:= goCompatibility(); err != nil{
		log.Printf("ERROR: %v\n", err)		
	}
	if err:= goGet(); err != nil{
		log.Printf("ERROR: %v\n", err)		
	}
	if err:= kernelGet(); err != nil{
		log.Printf("ERROR: %v\n", err)		
	}
	if err:= unpackKernel(); err != nil{
		log.Printf("ERROR: %v\n", err)		
	}
	if err:= vbutilIt(); err != nil{
		log.Printf("ERROR: %v\n", err)		
	}
	return nil
}



 
func main() {
//all paramters: name of new kernel, location for dd, kernel version, 
	fmt.Printf("Using kernel default as 4.12.7 from Anthony's Github for now \n")
	if err := allFunc(); err != nil {
		fmt.Printf("fail error is : %v", err)	
		os.Exit(1)	
	}
	fmt.Printf("execution completed successfully")

}
