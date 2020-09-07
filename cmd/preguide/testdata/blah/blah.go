// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Printf("#!/usr/bin/env bash\n")
	fmt.Printf("export GREETING=%v\n", os.Args[1])
}
