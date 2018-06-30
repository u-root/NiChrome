// install installs a NiChrome image from a USB stick onto the local drive.
// It verifies, first, that it can enumerate the partitions correctly
// on the destination. It uses guid_root to rewrite the GPT partition
// guids for the target partition(s).
// It defaults to only installing the B images, since we assume
// for now we are still in hacker mode.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/u-root/u-root/pkg/cpio"
	"github.com/u-root/u-root/pkg/gpt"
)

var cmdline = make(map[string]string)

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

// Find the boot media containing the root GUID.
func findKernDev(devs ...string) (string, gpt.GUID, error) {
	rg, ok := cmdline["guid_root"]
	if !ok {
		return "", gpt.GUID{}, fmt.Errorf("No guid_root cmdline parameter")
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
		// install media is always KERN-A, second partition.
		if pt.Primary.Parts[1].UniqueGUID.String() == rg {
			log.Printf("%v: GUID %s matches for partition 2\n", d, rg)
			return fmt.Sprintf("%s", d), pt.Primary.Parts[1].UniqueGUID, nil
		}
	}
	return "", gpt.GUID{}, fmt.Errorf("A device with that GUID was not found")
}

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		flag.Usage()
		log.Fatalf("install [options] dest-device")
	}

	parseCmdline()

	d, u, err := findKernDev("/dev/sda", "/dev/sdb")
	if err != nil {
		log.Fatal(err)
	}
	// For now we always go with the B side. This means we need to change the
	// GUID in the GPT for partition 4.
	log.Printf("Install Media is on %s", d)

	// We're going to be a bit paranoid in the order in which we do things.
	// open the device and read the GPT.
	// open both output partitions writeable to make sure that works.
	// open input partitions.
	// write the unchanged GPT back to the target as a test.
	// write KERN-B
	// write ROOT-B
	// write the changed GPT with the new KERN-B UniqueGUID
	// With luck, if there's a problem, we hit it long before
	// we doing anything irreversible.

	dest := flag.Args()[0]
	destDev, err := os.OpenFile(dest, os.O_RDWR, 0)
	if err != nil {
		log.Fatal(err)
	}
	pt, err := gpt.New(destDev)
	if err != nil {
		log.Fatal(err)
	}

	// What a hack. mmc doesn't follow the venerable
	// naming conventions.
	var hack string
	if strings.HasPrefix(dest, "/dev/mmc") {
		log.Printf("It's mmc, add hack")
		hack = "p"
	}
	log.Printf("Installing on %s%s{4,5}", dest, hack)
	destKern, err := os.OpenFile(dest+hack+"4", os.O_RDWR, 0)
	if err != nil {
		log.Fatal(err)
	}
	destRoot, err := os.OpenFile(dest+hack+"5", os.O_RDWR, 0)
	if err != nil {
		log.Fatal(err)
	}

	kern, err := os.Open(d + "2")
	if err != nil {
		log.Fatal(err)
	}
	root, err := os.Open(d + "3")
	if err != nil {
		log.Fatal(err)
	}
	// Point of no return. Fix the GUID on the device.
	// Write the old GPT back first to see if writes even work.
	if err := gpt.Write(destDev, pt); err != nil {
		log.Fatal(err)
	}
	if _, err := io.Copy(destKern, kern); err != nil {
		log.Fatal(err)
	}
	// TODO: create a pass function in cpio package
	// that takes an io.Write, an io.Reader, and
	// does this. But let's get it right first.
	archiver, err := cpio.Format("newc")
	if err != nil {
		log.Fatalf("newc not supported: %v", err)
	}

	r := archiver.Reader(root)
	w := archiver.Writer(destRoot)
	for {
		rec, err := r.ReadRecord()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("error reading records: %v", err)
		}
		if err := w.WriteRecord(rec); err != nil {
			log.Fatalf("Writing record %q failed: %v", rec, err)
		}
	}
	if err := cpio.WriteTrailer(w); err != nil {
		log.Fatalf("Writing Trailer failed: %v", err)
	}
	pt.Primary.Parts[3].UniqueGUID = u
	pt.Backup.Parts[3].UniqueGUID = u
	if err := gpt.Write(destDev, pt); err != nil {
		log.Fatal(err)
	}
	log.Printf("All done. restart in chromeos and cgpt add -i 4 -P 2 -S 0 -T 1 %v", dest)
}
