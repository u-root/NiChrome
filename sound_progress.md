## Sound Progress (or lack thereof)
### What We've Done
#### Build kernel with sound and PWM drivers:

Enable Device Drivers > Sound card support > ALSA

in ALSA panel of kernel configuration, enable:

* Sequencer support
* Sequencer dummy client
* PCM timer interface
* HR-timer backend support
* Use HR-timer as default sequencer timer
* Dynamic device file minor numbers
* Support old ALSA API
* Sound Proc FS Support
* Verbose procfs contents
* Generic sound devices
* PCI sound devices

    * Enable all Intel drivers
* HD-Audio

    * Enable all options
* SPI sound devices
* USB sound devices
* PCMCIA sound devices
* ALSA for SoC audio support

    * Enable all Intel Cherrytrail & Braswell drivers
    * Enable all SKL drivers

Enable Device Drivers > Pulse-Width Modulation > ChromeOS EC PWM driver


*This kernel may include many unnecessary drivers, or be missing something important. We're not sure.*

#### Use TinyCore to install ALSA packages:
After connecting to the internet in Nichrome:

`tcz -i -skip alsa-modules-KERNEL alsa alsa-config alsa-plugins`

*this gives us access to aplay, alsamixer, speaker-test, etc*

`aplay -l` has shown us most of the Chromebooks we have looked at (3 total) have 5 HDMI audio devices. `alsamixer` shows 5 S/PDIF digital outputs locked at 00, and (depending on the machine) a functioning PCM slider. Digital out usually does not mean output to speakers, which makes me think it has something to do with CRAS.

#### Make "staff" group:
`godit /etc/group`, writing `staff::1:root,user` to that file. This is just to appease `speaker-test`, which requires a group named "staff" to run.

#### Make ALSA config file:
`godit /etc/asound.conf` and write the following:
```
pcm.!default {
    type hw
    card 0
    device 3
}
```

As seen using `aplay -l`, the five HDMI outputs are listed as devices 3, 7, 8, 9, and 10. We've arbitrarily chosen 3 as the default device.

At this point, `speaker-test` will force you to run with output to 2 channels, `-c 2`. Everything seems good to go, speaker-test outputs sound to the default audio device as configured in /etc/asound.conf, but no sound comes out of any speakers. This is where we are stuck.

### Ideas

* Lots of ALSA documentation (for other Linux distros) heavily refers to modular kernels. Making the ALSA drivers modular may be a possible solution. Pulling the config from Chrome OS is a good way to see what's enabled/disabled/modular when it comes to sound.

* Relying on TinyCore's ALSA implementation may be a bad idea. It seems that many ALSA configurations are pretty kernel/device specific. We could build the ALSA tools ourselves and add them to Nichrome manually, rather than using TinyCore's version of ALSA.

* Chrome OS uses it's own audio server (CRAS) to interface with ALSA and do most of the dirty work. This may be why ALSA's only listed outputs are digital. In that case, we might need to pull in CRAS as well.

### Websites

ALSA: http://www.alsa-project.org/main/index.php/Main_Page

asound.conf format: http://www.alsa-project.org/main/index.php/Asoundrc

In-depth ALSA config: https://wiki.archlinux.org/index.php/Advanced_Linux_Sound_Architecture

TinyCore Packages: http://tinycorelinux.net/9.x/x86_64/tcz/

CRAS source: https://chromium.googlesource.com/chromiumos/third_party/adhd/+/master/cras

### Other helpful things

#### Getting ChromeOS kernel configuration file:
  1. boot into ChromeOS in developer mode
  2. open VT2 and run:
  ```
  modprobe configs
  cat /proc/config.gz > path/to/save/config
  ```

From here, you can copy this file to Google Drive/USB stick and open it using the kernel config tools on your build machine.

#### Saving NiChrome ALSA data:
Since Nichrome isn't stateful yet, you can use the stateful partition on your chromebook to backup your modifications to etc and tcz.

to backup:
```
mkdir mnt
mount -t ext4 /dev/mmcblk*p1 /mnt
cp -r /etc /mnt
cp -r /tcz /mnt
```
to reload after a restart:
```
mkdir mnt
mount -t ext4 /dev/mmcblk*p1 /mnt
cp -r /mnt/etc /
cp -r /mnt/tcz /
tcz -i alsa alsa-config alsa-plugins`
```
