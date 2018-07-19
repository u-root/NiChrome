# Booting in Verified Mode

*This process is known to work on all devices listed below. Feel free to try it out on others, and add it to the list if it works!*

### Part 1: Enter Developer mode
*Warning: this deletes all local data. Back up any important files before continuing*
  1. On your Chromebook, press Esc-Reload-Power to enter Recovery mode.
  2. Press Ctrl-D then Enter to reboot in Developer mode.

### Part 2: Build and install NiChrome
  1. On your Chromebook, install the Chrome OS Recovery tool from the Chrome Web Store and format the USB stick.
  2. On your build machine, run `go get github.com/u-root/u-root github.com/u-root/NiChrome` to fetch the source code.
  3. Navigate to $GOPATH/src/github.com/u-root/NiChrome/usb and run `go build .` to compile the USB build tool.
  4. Insert your formatted USB stick and determine its dev directory (/dev/sdX)
  5. Move back up to the NiChrome directory and run `./usb/usb -fetch=true -dev=/dev/sdX` to build NiChrome and load it onto the USB.
  6. On your Chromebook, boot into NiChrome by inserting the USB and pressing Ctrl-U. If this fails, see the Notes below.
  7. Run `install /dev/mmcblkX` to install NiChrome on the secondary boot partition (X will be either 0 or 1, depending on your system. Tab-complete to be safe)
  8. Set NiChrome's boot priority by running `/vboot_reference/build/cgpt/cgpt add -i 4 -P 2 -T 1 -S 0 /dev/mmcblkX`

*On your next reboot, press Ctrl-D to boot into NiChrome from disk*

### Part 3: Re-key your Chromebook and return to Verified mode
  1. Disable firmware write protection by cracking open the laptop's shell and removing the WP screw (older devices only) or disconnecting the battery.
  2. Boot into Chrome OS. If you're stuck in NiChrome, run `/vboot_reference/build/cgpt/cgpt add -i 4 -P 0 /dev/mmcblkX` to disable NiChrome, then reboot.
  3. Open the VT2 terminal on Chrome OS by pressing Ctrl-Alt-Forward, login as root.
  4. Sign your Chromebook firmware with developer keys by running `/usr/share/vboot/bin/make_dev_firmware.sh`
  5. Sign Chrome OS by running `/usr/share/vboot/bin/make_dev_ssd.sh --partitions 2`
  5. Sign NiChrome by running `/usr/share/vboot/bin/make_dev_ssd.sh --partitions 4`
  6. Save the key backups externally. When you exit Developer mode, your data will be wiped and you will not be able to revert
     to default keys in the future.
  7. Reset NiChrome's boot priority by running `cgpt add -i 4 -P -2 -T 1 /dev/mmcblkX`
  8. Reboot and press Spacebar then Enter to return to Verified mode!

*You should now be able to boot into NiChrome/Chrome OS in verified mode with the Developer keys*

### Notes
  * From the Developer mode warning screen, press Ctrl-D to boot from disk, and Ctrl-U to boot from USB.

  * We format the USB using the Chrome Recovery tool so that the partition system is in a form the bootloader can understand. It does not matter what Chromebook you format the USB for, as NiChrome overwrites it anyway.

  * If the Developer mode warning yells at you when trying to boot from USB, boot into Chrome OS, enter VT2, and run `enable_dev_usb_boot`.

  * Tries gets decremented on each boot. To remain in NiChrome, run `/vboot_reference/build/cgpt/cgpt add -i 4 -T 1 /dev/mmcblkX` every time you start.

  * If you forget to reset tries and are stuck in Verified mode, you can boot your NiChrome USB stick by inserting it and pressing Esc-Reload-Power. From here, you can run the same command as above: `/vboot_reference/build/cgpt/cgpt add -i 4 -T 1 /dev/mmcblkX`

  * If you create your own signing keys, add `-k /path/to/keys` to all `make_dev_*` commands in Part 3. Also, insert the NiChrome USB and run `/usr/share/vboot/bin/make_dev_ssd.sh -i /dev/sdX -k /path/to/keys --recovery_key` to sign your USB as a recovery stick. This way, you can boot from your USB in Verified mode.

### Known Working Boards

*Find your board name in Recovery mode at the bottom of your screen, without a USB stick inserted.*

  * Basking
  * Lava
  * Sentry
