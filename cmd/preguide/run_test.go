// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

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
