// +build linux

package main

import "path/filepath"

func isAbsolute(p string) bool {
	return filepath.IsAbs(p)
}
