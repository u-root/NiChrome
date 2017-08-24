package main

import (
	"fmt"
	"os"
)

const (
	HEADER    = "\033[95m"
	OKBLUE    = "\033[94m"
	OKGREEN   = "\033[32m"
	WARNING   = "\033[33m"
	FAIL      = "\033[31m"
	ENDC      = "\033[0m"
	BOLD      = "\033[1m"
	UNDERLINE = "\033[4m"
)

func info(output string) {
	fmt.Println(OKGREEN + BOLD + "INFO: " + output + ENDC)
}

func fatal(output string) {
	fmt.Println(FAIL + BOLD + "ERROR: " + output + ENDC)
	os.Exit(1)
}

func warning(output string) {
	fmt.Println(WARNING + BOLD + "WARNING: " + output + ENDC)
}
