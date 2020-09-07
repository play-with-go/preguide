// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

const (
	pullImageMissing = "missing"
)

type usageErr struct {
	err error
	u   cmd
}

func (u usageErr) Error() string { return u.err.Error() }

type cmd interface {
	usage() string
	usageErr(format string, args ...interface{}) usageErr
}

type rootCmd struct {
	*runner
	fs           *flag.FlagSet
	flagDefaults string
	fDebug       *bool
}

func newFlagSet(name string, setupFlags func(*flag.FlagSet)) string {
	res := flag.NewFlagSet(name, flag.ContinueOnError)
	var b bytes.Buffer
	res.SetOutput(&b)
	setupFlags(res)
	res.PrintDefaults()
	res.SetOutput(ioutil.Discard)
	s := b.String()
	const indent = "\t"
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			lines[i] = ""
		} else {
			lines[i] = indent + strings.Replace(l, "\t", "    ", 1)
		}
	}
	return strings.Join(lines, "\n")
}

func newRootCmd(r *runner) *rootCmd {
	res := &rootCmd{
		runner: r,
	}
	res.flagDefaults = newFlagSet("preguide", func(fs *flag.FlagSet) {
		res.fs = fs
		res.fDebug = fs.Bool("debug", os.Getenv("PREGUIDE_DEBUG") == "true", "print debug output to os.Stderr")
	})
	return res
}

func (r *rootCmd) usage() string {
	return fmt.Sprintf(`
Usage of preguide:

    preguide <command>

The commands are:

    docker
    gen
    init

Use "preguide help <command>" for more information about a command.

preguide defines the following flags:

%s`[1:], r.flagDefaults)
}

func (r *rootCmd) usageErr(format string, args ...interface{}) usageErr {
	return usageErr{fmt.Errorf(format, args...), r}
}
