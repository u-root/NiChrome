// This is the basic chromebook uinit.
package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/u-root/u-root/pkg/gpt"
	"github.com/u-root/u-root/pkg/uroot/util"
)

// For now we are going to stick with a single
// version of tcz packages. It's not possible
// with their design to mix versions.
const tczs = "/tcz/8.x/*/tcz/*.tcz"

var (
	cmdline = make(map[string]string)
	debug   = func(string, ...interface{}) {}
	verbose bool
)

func tczSetup() error {
	g, err := filepath.Glob(tczs)
	if err != nil {
		log.Printf("Glob of %v: %v", tczs, err)
	}
	debug("Tcz file list: %v", g)
	// Now get the basenames, and then install them.
	// TODO: fix up tcz to take a path name?
	// The glob ensured they all end in .tcz.
	// We can just take all but the last 4 chars of the name.
	var tczlist []string
	for _, p := range g {
		b := filepath.Base(p)
		tczlist = append(tczlist, b[:len(b)-4])
	}

	log.Printf("Installing %d tinycore packages...", len(tczlist))
	cmd := exec.Command("tcz", append([]string{"-v", "8.x"}, tczlist...)...)
	log.Printf("Done")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func parseCmdline() {
	b, err := ioutil.ReadFile("/proc/cmdline")
	if err != nil {
		log.Printf("Can't read command line: %v", err)
	}
	for _, s := range strings.Fields(string(b)) {
		f := strings.SplitN(s, "=", 2)
		if len(f) == 0 {
			continue
		}
		if len(f) == 1 {
			f = []string{f[0], "1"}
		}
		cmdline[f[0]] = f[1]
	}
}

// Find the root GUID.
func findRoot(devs ...string) (string, error) {
	rg, ok := cmdline["guid_root"]
	if !ok {
		return "", fmt.Errorf("No root_guid cmdline parameter")
	}
	for _, d := range devs {
		fi, err := os.Stat(d)
		if fi == nil || err != nil {
			log.Print(err)
			continue
		}
		if fi.Mode()&os.ModeType != os.ModeDevice {
			log.Printf("%v is not a device", d)
			continue
		}
		f, err := os.Open(d)
		if err != nil {
			log.Print(err)
			continue
		}
		g, _, err := gpt.New(f)
		f.Close()
		if err != nil {
			log.Print(err)
			continue
		}
		for i, p := range g.Parts {
			var zero uuid.UUID
			if p.UniqueGUID == zero {
				continue
			}
			if p.UniqueGUID.String() == rg {
				log.Printf("%v: GUID %s matches for partition %d (map to %d)\n", d, rg, i, i+2)
				// non standard naming. Grumble.
				var hack string
				if strings.HasPrefix(d, "/dev/mmc") {
					hack = "p"
				}
				return fmt.Sprintf("%s%s%d", d, hack, i+2), nil
			}
			log.Printf("%v: part %d, Device GUID %v, GUID %s no match", d, i, p.UniqueGUID.String(), rg)
		}
	}
	return "", fmt.Errorf("A device with that GUID was not found")
}

func main() {
	log.Printf("Welcome to NiChrome!")
	parseCmdline()

	if _, ok := cmdline["uinitdebug"]; ok {
		debug = log.Printf
		verbose = true
	}

	var cpio bool
	// USB sucks.
	// We've tried a few variants of this loop so far trying for
	// 10 seconds and waiting for 1 second each time has been the best.
	for i := 0; i < 10; i++ {
		r, err := findRoot("/dev/sda", "/dev/sdb", "/dev/mmcblk0", "/dev/mmcblk1")
		if err != nil {
			log.Printf("Could not find root: %v", err)
		} else {
			log.Printf("Try device %v", r)
			cmd := exec.Command("cpio", "-v", "i")
			if cmd.Stdin, err = os.Open(r); err == nil {
				o, err := cmd.CombinedOutput()
				if err != nil {
					log.Printf("cpio of tcz failed %v; continuing", err)
				} else {
					cpio = true
				}
				err = ioutil.WriteFile(r+".log", o, 0666)
				if err != nil {
					log.Printf("Can't write log file: %v", err)
				}
			} else {
				log.Printf("Can't open (%v); not trying to cpio it", r)
			}
			if cpio {
				break
			}
		}
		time.Sleep(time.Second)
	}

	if err := tczSetup(); err != nil {
		log.Printf("tczSetup: %v", err)
	}

	// buildbin was not populated, potentially, so we have to do it again.
	c, err := filepath.Glob("/src/github.com/u-root/*/cmds/[a-z]*")
	if err != nil || len(c) == 0 {
		log.Printf("In a break with tradition, you seem to have NO u-root commands: %v", err)
	}
	o, err := filepath.Glob("/src/*/*/*")
	if err != nil {
		log.Printf("Your filepath glob for other commands seems busted: %v", err)
	}
	c = append(c, o...)
	for _, v := range c {
		name := filepath.Base(v)
		if name == "installcommand" || name == "init" {
			continue
		} else {
			destPath := filepath.Join("/buildbin", name)
			source := "/buildbin/installcommand"
			if err := os.Symlink(source, destPath); err != nil {
				log.Printf("Symlink %v -> %v failed; %v", source, destPath, err)
			}
		}
	}

	a := []string{"build"}
	envs := os.Environ()
	debug("envs %v", envs)
	//os.Setenv("GOBIN", "/buildbin")
	a = append(a, "-o", "/buildbin/installcommand", filepath.Join(util.CmdsPath, "installcommand"))
	icmd := exec.Command("go", a...)
	installenvs := envs
	installenvs = append(envs, "GOBIN=/buildbin")
	icmd.Env = installenvs
	icmd.Dir = "/"

	icmd.Stdin = os.Stdin
	icmd.Stderr = os.Stderr
	icmd.Stdout = os.Stdout
	debug("Run %v", icmd)
	if err := icmd.Run(); err != nil {
		log.Printf("%v\n", err)
	}

	cmd := exec.Command("ip", "addr", "add", "127.0.0.1/24", "lo")
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
