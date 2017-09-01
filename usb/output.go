package main

import (
	"fmt"
	"os"
)

const (
	OKGREEN   = "\033[32m"
	WARNING   = "\033[33m"
	FAIL      = "\033[31m"
	ENDC      = "\033[0m"
	BOLD      = "\033[1m"
)

func info(output ...interface{}) {
	for _, v := range output {
		fmt.Print(OKGREEN + BOLD + "INFO: ")
		fmt.Print(v)
		fmt.Println(ENDC)
	}
}

func fatal(output ...interface{}) {
	for _, v := range output {
		fmt.Print(FAIL + BOLD + "ERROR: ")
		fmt.Print(v)
		fmt.Println(ENDC)
	}
	os.Exit(1)
}

func warning(output ...interface{}) {
	for _, v := range output {
		fmt.Print(WARNING + BOLD + "WARNING: ")
		fmt.Print(v)
		fmt.Println(ENDC)
	}
}
