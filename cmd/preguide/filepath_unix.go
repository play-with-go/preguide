// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris

package main

import "path/filepath"

func isAbsolute(p string) bool {
	return filepath.IsAbs(p)
}
