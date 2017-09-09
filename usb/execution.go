package main

import (
	"bytes"
	"os/exec"

)

func execute(path string, args ...string) (string, string, error) {
	var o, e bytes.Buffer
	cmd := exec.Command(path, args...)
	cmd.Stderr = &e
	cmd.Stdout = &o

	err := cmd.Run()

	return o.String(), e.String(), err
}
