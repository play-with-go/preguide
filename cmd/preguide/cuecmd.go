// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"

	cuecmd "cuelang.org/go/cmd/cue/cmd"
)

type cueCmd struct {
	*runner
	fs           *flag.FlagSet
	flagDefaults string
}

func newCueCmd(r *runner) *cueCmd {
	res := &cueCmd{
		runner: r,
	}
	res.flagDefaults = newFlagSet("preguide cue", func(fs *flag.FlagSet) {
		res.fs = fs
	})
	return res
}

func (cc *cueCmd) usage() string {
	return fmt.Sprintf(`
usage: preguide cue

%s`[1:], cc.flagDefaults)
}

func (cc *cueCmd) usageErr(format string, args ...interface{}) usageErr {
	return usageErr{fmt.Errorf(format, args...), cc}
}

func (cc *cueCmd) run(args []string) error {
	if err := cc.fs.Parse(args); err != nil {
		return cc.usageErr("failed to parse flags: %v", err)
	}
	// We now hand off to the cmd/cue/cmd. In order for this to work we need
	// to update os.Args
	os.Args = append([]string{"cue"}, cc.fs.Args()...)
	os.Exit(cuecmd.Main())
	panic("not here")
}
