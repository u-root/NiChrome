// This is the basic chromebook uinit.
package main

import (
	"log"
	"os"
	"os/exec"
)

// For now we are going to stick with a single
// version of tcz packages. It's not possible
// with their design to mix versions.
const tczs = "/tcz/8.x/*/tcz/*.tcz"

func tczSetup() error {
	g, err := filepath.Glob(tczs)
	if err != nil {
		log.Printf("Glob of %v: %v", tczs, err)
	}
	log.Printf("Tcz file list: %v", g)
	// Now get the basenames, and then install them.
	// TODO: fix up tcz to take a path name?
	// The glob ensured they all end in .tcz.
	// We can just take all but the last 4 chars of the name.
	var tczlist []string
	for _, p := range g {
		b := filepath.Base(p)
		tczlist = append(tczlist, b[:len(b)-4])
	}

	cmd := exec.Command("tcz", append([]string{"-v", "8.x"}, tczlist...)...)
	log.Printf("Get Tczlist: %v", tczlist)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func main() {
	log.Printf("Welcome to NiChrome!")
	var err error
	cmd := exec.Command("cpio", "i")
	if cmd.Stdin, err = os.Open("/dev/sda3"); err == nil {
		if o, err := cmd.CombinedOutput(); err != nil {
			log.Printf("cpio of tcz failed (%v, %v); continuing", o, err)
		}
	} else {
		log.Printf("Can't open /dev/sda3 (%v); not trying to cpio it")
	}

	if err := tczSetup(); err != nil {
		log.Printf("tczSetup: %v", err)
	}

	cmd = exec.Command("ip", "addr", "add", "127.0.0.1/24", "lo")
	if o, err := cmd.CombinedOutput(); err != nil {
		log.Printf("ip link failed(%v, %v); continuing", string(o), err)
	}
	if err := os.Symlink("/bin/bash", "/bin/sh"); err != nil {
		log.Printf("symlink /bin/bash to /bin/sh: ", err)
	}
	cmd = exec.Command("xinit")
	if o, err := cmd.CombinedOutput(); err != nil {
		log.Printf("xinit failed (%v, %v); continuing", string(o), err)
	}
}
