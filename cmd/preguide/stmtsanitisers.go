// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"github.com/play-with-go/preguide/sanitisers"
	"github.com/play-with-go/preguide/sanitisers/cmdgo"
)

// stmtSanitisers is a list of stmtSanitisers for statement sanitisers. The
// hard-coded list will ultimately be replaced by a more extensible solution,
// as described in github.com/play-with-go/preguide/issues/73. The main
// question to answer in that issue is how this map comes to be populated.
var stmtSanitisers = map[string]sanitisers.StmtSanitiser{
	"github.com/play-with-go/preguide/cmd/preguide/sanitisers/cmdgo.CmdGoStmtSanitiser": cmdgo.CmdGoStmtSanitiser,
}
