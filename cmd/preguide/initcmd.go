// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"flag"
	"fmt"
)

type initCmd struct {
	*runner
	fs           *flag.FlagSet
	flagDefaults string
}

func newInitCmd(r *runner) *initCmd {
	res := &initCmd{
		runner: r,
	}
	res.flagDefaults = newFlagSet("preguide init", func(fs *flag.FlagSet) {
		res.fs = fs
	})
	return res
}

func (ic *initCmd) usage() string {
	return fmt.Sprintf(`
usage: preguide init

%s`[1:], ic.flagDefaults)
}

func (ic *initCmd) usageErr(format string, args ...interface{}) usageErr {
	return usageErr{fmt.Errorf(format, args...), ic}
}

func (ic *initCmd) run(args []string) error {
	return nil
}
