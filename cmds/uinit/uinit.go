// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is the basic chromebook uinit.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/u-root/u-root/pkg/cpio"
	"github.com/u-root/u-root/pkg/mount/gpt"
)

// For now we are going to stick with a single
// version of tcz packages. It's not possible
// with their design to mix versions.
const (
	tczs    = "/tcz/8.x/*/tcz/*.tcz"
	homeEnv = "/home/user"
	userEnv = "user"
	passwd  = "root:x:0:0:root:/:/bin/bash\nuser:x:1000:1000:" + userEnv + ":" + homeEnv + ":/bin/bash\n"
	hosts   = "127.0.0.1 localhost\n"
)

var (
	rootStartCmds = []string{"sos", "wifi", "timesos"}
	userStartCmds = []string{"wingo", "AppChrome", "upspinsos", "chrome"}
	cmdline       = make(map[string]string)
	debug         = func(string, ...interface{}) {}
	usernamespace = flag.Bool("usernamespace", false, "Set up user namespaces and spawn login")
	user          = flag.Bool("user", false, "Ru as a user")
	login         = flag.Bool("login", false, "Login as a user")
	verbose       bool
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
	log.Print("Done")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
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
		pt, err := gpt.New(f)
		f.Close()
		if err != nil {
			log.Print(err)
			continue
		}
		for i, p := range pt.Primary.Parts {
			var zero gpt.GUID
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

func x11(n string, args ...string) error {
	out := os.Stdout
	f, err := ioutil.TempFile("", filepath.Base(n))
	if err != nil {
		log.Print(err)
	} else {
		out = f
	}
	cmd := exec.Command(n, args...)
	cmd.Env = append(os.Environ(), "DISPLAY=:0")
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("X11 start %v %v: %v", n, args, err)
	}
	return nil
}

// When we make the transition to a new user we need to set up a new namespace for that user.
// So far the only thing we know we need to do is remount ubin, tmp, env, and go/pkg
// The tmp is particularly useful as it avoids races between root-owned files and files
// for this user.
var (
	namespace = []Creator{
		Mount{Source: "tmpfs", Target: "/dev/shm", FSType: "tmpfs"},
	}
	rootFileSystem = []Creator{
		Dir{Name: "/go/pkg/linux_amd64", Mode: 0777},
		Dir{Name: "/dev/shm", Mode: 0777},
		Dir{Name: "/pkg", Mode: 0777},
		Dir{Name: "/ubin", Mode: 0777},
		// fusermount requires this. When we write our own we can remove this.
		Symlink{NewPath: "/etc/mtab", Target: "/proc/mounts"},
		// Resolve localhost name
		File{Name: "/etc/hosts", Contents: "127.0.0.1\tlocalhost\n::1\tlocalhost ip6-localhost ip6-loopback\n", Mode: 0644},
	}
)

func xrun() error {
	// At this point we are still root.
	if err := os.Symlink("/usr/local/bin/bash", "/bin/bash"); err != nil {
		return err
	}
	if err := os.Symlink("/lib/ld-linux-x86-64.so.2", "/lib64/ld-linux-x86-64.so.2"); err != nil {
		return err
	}
	go func() {
		cmd := exec.Command("Xfbdev")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("X11 startup: %v", err)
		}
	}()
	for {
		s, err := filepath.Glob("/tmp/.X*/X?")
		if err != nil {
			return err
		}
		if len(s) > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	return nil
}

func dousernamespace() error {
	// start us as a child again with a private name space.
	// Limitations of the Go runtime mandate doing it this way.

	// due to limits of Go runtime we have to run ourselves again with -login
	// and build a namespace.
	cmd := exec.Command("/bbin/uinit", "--login")
	cmd.SysProcAttr = &syscall.SysProcAttr{Unshareflags: syscall.CLONE_NEWNS}
	cmd.Env = append(os.Environ(), fmt.Sprintf("USER=%v", userEnv), fmt.Sprintf("HOME=%v", homeEnv))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("donamespace: %v", err)
	}
	return nil
}

func dologin() error {
	// Here we need to create the new namespace and then start our children.
	var err error
	for _, c := range namespace {
		if err = c.Create(); err != nil {
			return fmt.Errorf("Error creating %s: %vi; not starting user x11 programs", c, err)
		}
	}

	if err == nil {
		// due to limits of Go runtime we have to run ourselves again with -user.
		cmd := exec.Command("/bbin/uinit", "--user")
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1000, Gid: 1000, NoSetGroups: true}}
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("X11 user startup: %v", err)
		}
	}
	return nil
}

func homedir() error {
	f, err := os.Open("/usr/user.cpio")
	if err != nil {
		return err
	}
	archiver, err := cpio.Format("newc")
	if err != nil {
		return err
	}
	rr := archiver.Reader(f)
	for {
		rec, err := rr.ReadRecord()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading records: %v", err)
		}
		debug("Creating %s\n", rec)
		if err := cpio.CreateFile(rec); err != nil {
			log.Printf("Creating %q failed: %v", rec.Name, err)
		}
	}
	return nil
}

func xrunuser() error {
	for _, f := range userStartCmds {
		log.Printf("Run %v", f)
		go x11(f)
	}

	// we block on the aterm. When the aterm exits, we do too.
	return x11("/usr/local/bin/aterm")
}

func cpioRoot(r string) error {
	var err error
	log.Printf("Try device %v", r)
	cmd := exec.Command("cpio", "i")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if cmd.Stdin, err = os.Open(r); err != nil {
		return fmt.Errorf("%v", err)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cpio of tcz failed %v", err)
	}
	return nil
}

// This is a best effort. It does not return an error because
// an error may not really be an error; we may be intentionally
// running without a root.
func guidRoot() {
	// USB sucks.
	// We've tried a few variants of this loop so far trying for
	// 10 seconds and waiting for 1 second each time has been the best.
	for i := 0; i < 10; i++ {
		r, err := findRoot("/dev/sda", "/dev/sdb", "/dev/mmcblk0", "/dev/mmcblk1")
		if err != nil {
			log.Printf("Could not find root: %v", err)
		} else {
			if err := cpioRoot(r); err == nil {
				break
			}
		}
		time.Sleep(time.Second)
	}
}

func main() {
	// nasty, but I'm sick of losing boot messages.
	f, err := os.OpenFile("/log", os.O_RDWR|os.O_CREATE, 0755)
	if err == nil {
		fd := int(f.Fd())
		if err := syscall.Dup2(fd, 1); err != nil {
			log.Printf("Could not dup %v over 1: %v", fd, err)
		}
		if err := syscall.Dup2(fd, 2); err != nil {
			log.Printf("Could not dup %v over 2: %v", fd, err)
		}
	} else {
		log.Printf("Can't open /log: %v", err)
	}
	log.Print("Welcome to NiChrome!")
	flag.Parse()
	if *usernamespace {
		if err := dousernamespace(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	if *login {
		if err := dologin(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	if *user {
		log.Print("Starting up user mode processes")
		if err := homedir(); err != nil {
			log.Printf("Could not populate %v, err %v: continuing anyway", homeEnv, err)
		}
		if err := xrunuser(); err != nil {
			log.Fatalf("x11 user failed: %v", err)
		}
		os.Exit(0)
	}

	parseCmdline()

	if _, ok := cmdline["uinitdebug"]; ok {
		debug = log.Printf
		verbose = true
	}

	if d, ok := cmdline["nichromeroot"]; ok {
		cpioRoot(d)
	} else {
		guidRoot()
	}

	if err := tczSetup(); err != nil {
		log.Printf("tczSetup: %v", err)
	}

	cmd := exec.Command("ip", "addr", "add", "127.0.0.1/24", "lo")
	if o, err := cmd.CombinedOutput(); err != nil {
		log.Printf("ip link failed(%v, %v); continuing", string(o), err)
	}
	cmd = exec.Command("ip", "link", "set", "dev", "lo", "up")
	if o, err := cmd.CombinedOutput(); err != nil {
		log.Printf("ip link up failed(%v, %v); continuing", string(o), err)
	}

	for _, c := range rootFileSystem {
		if err = c.Create(); err != nil {
			log.Printf("Error creating %s: %vi; not starting user x11 programs", c, err)
		}
	}

	// AppImages need /dev/fuse to be 0666, even though they also use the
	// suid fusermount, which does not need /dev/fuse to be 0666. Oh well.
	if err := os.Chmod("/dev/fuse", 0666); err != nil {
		log.Printf("chmod of /dev/fuse to 0666 failed: %v", err)
	}

	// If they did not supply a password file, we have to supply a simple
	// one or tools like fusermount will fail. We hope soon to have a
	// u-root implementation of fusermount that's not so particular.
	if _, err := os.Stat("/etc/passwd"); err != nil {
		if err := ioutil.WriteFile("/etc/passwd", []byte(passwd), os.FileMode(0644)); err != nil {
			log.Printf("Error creating /etc/passwd: %v", err)
		}
	}
	// If they did not supply a hosts file, we need one for localhost.
	if _, err := os.Stat("/etc/hosts"); err != nil {
		if err := ioutil.WriteFile("/etc/hosts", []byte(hosts), os.FileMode(0644)); err != nil {
			log.Printf("Error creating /etc/hosts: %v", err)
		}
	}
	if err := xrun(); err != nil {
		log.Fatalf("xrun failed %v:", err)
	}

	// HACK.
	// u-root is setting bogus modes on /. fix it.
	// hack for new u-root cpio bug.
	// We may just leave this here forever, since the failure is so hard
	// to diagnose.
	if err := os.Chmod("/", 0777); err != nil {
		log.Print(err)
	}

	for _, f := range rootStartCmds {
		log.Printf("Run %v", f)
		go x11(f)
		// we have to give it a little time until we make it smarter
		time.Sleep(2 * time.Second)
	}

	if err := dousernamespace(); err != nil {
		log.Printf("dousernamespace: %v", err)
	}

	// kick off one user shell so they can do what needs to be done.
	// When this ends we exit everything.
	if err := x11("/usr/local/bin/aterm"); err != nil {
		log.Printf("Starting root aterm: %v", err)
	}
}
