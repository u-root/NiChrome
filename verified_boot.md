# Booting in Verified Mode

### Part 1: Enter Developer mode
*Warning: this deletes all local data. Back up any important files before continuing*
  1. On your Chromebook, press Esc-Reload-Power to enter Recovery mode
  2. Press Ctrl-D to reboot in Developer mode.

### Part 2: Build and install NiChrome
  1. On your Chromebook, install the Chrome OS Recovery tool and use it to format a USB stick
  2. On your build machine, run `go get github.com/u-root/u-root github.com/u-root/NiChrome`
  3. Navigate to NiChrome/usb and run `go build .`
  4. Insert your formatted USB stick and determine its dev directory (/dev/sdx)
  5. Navigate to NiChrome and run `./usb/usb -fetch=true -dev=/dev/sdx` to build NiChrome on the USB
  6. Boot into NiChrome by inserting the USB and pressing either Ctrl-U or Esc-Reload-Power
  7. Run `install /dev/mmcblkx` (x will be either 0 or 1, depending on your system. Tab-complete to be safe)
  8. Set NiChrome's boot priority by running `/vboot_reference/build/cgpt/cgpt add -i 4 -P 2 -T 1 -S 0 /dev/mmcblkx`

*On your next reboot, press Ctrl-D to boot into NiChrome from disk*

### Part 3: Re-key your Chromebook and return to Verified mode
  1. Disable firmware write protection by removing the WP screw or disconnecting the battery
  2. Boot into Chrome OS. If you're stuck in NiChrome, run `/vboot_reference/build/cgpt/cgpt add -i 4 -P 0 /dev/mmcblkx` and reboot
  3. Open the VT2 terminal on Chrome OS by pressing Ctrl-Alt-Forward, login as root
  4. Sign your Chromebook firmware with developer keys by running `/usr/share/vboot/bin/make_dev_firmware.sh`
  5. Sign Chrome OS by running `/usr/share/vboot/bin/make_dev_ssd.sh --partitions 2`
  5. Sign NiChrome by running `/usr/share/vboot/bin/make_dev_ssd.sh --partitions 4`
  6. Save the key backups externally. When you exit Developer mode, your data will be wiped and you will not be able to revert
     to default keys in the future.
  7. Reset NiChrome's boot priority by running `cgpt add -i 4 -P -2 -T 1 /dev/mmcblkx`
  8. Reboot and press Spacebar to enter Verified mode.

*You should now be able to boot into NiChrome/Chrome OS in verified mode with the Developer keys*

### Notes
  Tries gets decremented on each boot. To remain in NiChrome, run `/vboot_reference/build/cgpt/cgpt add -i 4 -T 1 /dev/mmcblkx` every time you start.

  If you forget to reset tries and are stuck in Verified mode, you can boot your NiChrome USB stick by inserting it and pressing Esc-Reload-Power. From here, you can run the same command as above: `/vboot_reference/build/cgpt/cgpt add -i 4 -T 1 /dev/mmcblkx`

  If you create your own signing keys, add `-k /path/to/keys` to all `make_dev_*` commands in Part 3. Additionally, insert the NiChrome USB and run `/usr/share/vboot/bin/make_dev_ssd.sh -i /dev/sdx -k /path/to/keys --recovery_key` to sign your USB as a recovery stick. This way, you can boot from your USB in Verified mode.
