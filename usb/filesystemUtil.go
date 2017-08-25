package main

import (
	"fmt"
	"syscall"
)

func FSMount(partDev, mountDir, FSFormat string, flag uintptr) error {
	if !(flag == syscall.MS_RDONLY || flag == syscall.MS_MGC_VAL) {
		return fmt.Errorf("ro or rw not: %b", flag)
	}

	switch FSFormat {
	case "ext2", "ext3", "ext4", "vfat", "fat", "fat12", "fat16", "fat32", "":
		syscall.Mount(partDev, mountDir, FSFormat, flag, "")
	case "squashfs":
		if flag == syscall.MS_RDONLY {
			syscall.Mount(partDev, mountDir, FSFormat, flag, "")
		} else {
			return fmt.Errorf("Squashfs RW Unimplemented")
		}
	default:
		return fmt.Errorf("Unknown FS format %s", FSFormat)
	}

	return nil
}

func FSUmount(partDev, mountDir, FSFormat, FSOptions string) error {
	/*
	 TODO(laconicpneumonic)

	*/
	if err := syscall.Unmount(mountDir, 0); err != nil {
		return syscall.Unmount(mountDir, syscall.MNT_DETACH)
	}

	return nil
}