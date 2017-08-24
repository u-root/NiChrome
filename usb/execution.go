package main

import (
	"os"
	"os/exec"
)

func execute(path string,  args ...string) (string, string, error) {
	var o, e bytes.Buffer
	cmd := exec.Command(path, args...)
	cmd.Stderr = o
	cmd.Stdout = e
	err := cmd.Run()

	return o.String(), e.String(), err
}
