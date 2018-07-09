# Booting in Verified Mode

### Part 1: Jailbreak your Chromebook
  1. Enable Developer mode using Esc-Reload-Power then pressing Ctrl-D.
  2. Disable firmware write protection by removing the WP screw or disconnecting the battery
  3. If you want, generate your own keys, storing them in a known directory on Chrome OS (path/to/keys)
  4. Open a terminal window on Chrome OS using Ctrl-Alt-Forward, login as root
  5. Sign your firmware by running `/usr/share/vboot/bin/make_dev_firmware.sh -k "path/to/keys"`
  6. Sign ChromeOS by running `/usr/share/vboot/bin/make_dev_ssd.sh --partitions 2 -k "path/to/keys"`

*You should now be able to boot into Chrome OS in verified mode with your own keys*

### Part 2: Build the NiChrome stick
  1. On your Chromebook, install the Chrome OS Recovery tool and use it to format your USB stick
  2. On your build machine, run `go get https://github.com/u-root/u-root.git https://github.com/u-root/NiChrome.git`
  3. Navigate to NiChrome/usb and run `go build .`
  4. Insert your formatted USB stick and determine its dev directory (/dev/sdx)
  5. Navigate to NiChrome and run `./usb/usb -fetch=true -dev=/dev/sdx`

*You should now be able to boot NiChrome from USB in Developer mode*

### Part 3: Sign the NiChrome USB stick

*If you signed your firmware with Developer keys this step is unnecessary, NiChrome signs its USB with the Developer recovery keys by default*

  1. On your Chromebook, insert your NiChrome USB stick and determine its dev directory
  2. Open a root terminal and sign NiChrome by running `/usr/share/vboot/bin/make_dev_ssd.sh -i /dev/sdx -k "path/to/keys" --recovery_key`

### Part 4: Install and sign NiChrome, revert to Verified mode
  1. Boot into NiChrome and run `install /dev/mmcblkx` (x will be either 0 or 1, depending on your system. Tab-complete to be safe)
  2. Run `/vboot_reference/build/cgpt/cgpt add -i 4 -P 2 -T 1 -S 0 /dev/mmcblkx`
  3. Return to Verified mode by pressing spacebar on powerup

*You should be dropped into NiChrome*

On next reboot, you will return to Chrome OS. To stay in NiChrome, run `cgpt add -i 4 -T 1 /dev/mmcblkx` every time you start.
