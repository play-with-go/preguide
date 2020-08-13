// +build linux

package main

import (
	"os"
	"syscall"
)

func runSelf(path string) {
	args := []string{"preguide"}
	args = append(args, os.Args[1:]...)
	if err := syscall.Exec(path, args, os.Environ()); err != nil {
		panic(err)
	}
}
