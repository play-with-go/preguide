// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

// +build tools

package tools

import (
	_ "cuelang.org/go/cmd/cue"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
