package main

import (
	"fmt"
	"syscall"
)

func FSMount(partDev, mountDir, FSFormat string, flag uintptr) error {

	switch FSFormat {
	case "ext2", "ext3", "ext4", "vfat", "fat", "fat12", "fat16", "fat32", "":
		if err := syscall.Mount(partDev, mountDir, FSFormat, flag, ""); err != nil {
			return fmt.Errorf("FSMount: %v", err)
		}
	case "squashfs":
		if flag == syscall.MS_RDONLY {
			if err := syscall.Mount(partDev, mountDir, FSFormat, flag, ""); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("Squashfs RW Unimplemented")
		}
	default:
		return fmt.Errorf("Unknown FS format %s", FSFormat)
	}
	return nil
}

func FSUmount(mountDir string) error {
	if err := syscall.Unmount(mountDir, 0); err != nil {
		return fmt.Errorf("FSUmount: %v", syscall.Unmount(mountDir, syscall.MNT_DETACH))
	}

	return nil
}