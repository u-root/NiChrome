#!/bin/bash

# Use this script to back up your /etc and /tcz directories to the stateful
# partition on your local hard drive. Makes testing sound configuration easier.
# It assumes you have installed the three main ALSA tcz packages already.

# make sure the first arg is only save or load
if [ -n $1 ] && ([ $1 = "save" ] || [ $1 = "load" ]); then
  # if the mount directory does not exist, make it
  if [ ! -d /mnt ]; then
    mkdir mnt
  fi
  # if the mount directory is empy, mount partition 1 to it
  if [ -z "$(ls -a /mnt)" ]; then
    mount -t ext4 /dev/mmcblk*p1 /mnt/
  fi
  # if the user requested to save, save
  if [ $1 = "save" ]; then
    cp -r /tcz /mnt
    cp -r /etc /mnt
  # if the user requested to load, load and install alsa tcz packages.
  elif [ $1 = "load" ]; then
    cp -r /mnt/tcz /
    cp -r /mnt/etc /
    tcz -i alsa-config alsa-plugins alsa
  fi
  # otherwise, print usage
else
  echo "Usage: ./usr/sound.sh [save load]"
fi
