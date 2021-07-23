// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

// preguide is a pre-processor for Play With Docker-based guides
package main

// A note on Go types, CUE definitions and code generation
// =======================================================
// Ideally we would have Go types be the source of truth for this entire
// program. The Go package github.com/play-with-go/preguide/internal/types
// would be the source of truth for the github.com/play-with-go/preguide CUE
// definitions, and the Go types defined in github.com/play-with-go/preguide
// would be the source of truth for the types defined in
// github.com/play-with-go/preguide/out CUE definitions.
//
// However, as github.com/cuelang/cue/discussions/462 concludes, there isn't currently
// a good story on how to handle converting Go interface types to CUE definitions.
// So for now we manually define the two.
//
// Theoretically we could code generate some of these types

//go:generate go run cuelang.org/go/cmd/cue cmd genimagebases

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/play-with-go/preguide"
	"github.com/play-with-go/preguide/internal/util"
)

func main() { os.Exit(main1()) }

func main1() int {
	r := newRunner()

	r.rootCmd = newRootCmd(r)
	r.genCmd = newGenCmd(r)
	r.initCmd = newInitCmd(r)
	r.helpCmd = newHelpCmd(r)
	r.dockerCmd = newDockerCmd(r)
	r.cueCmd = newCueCmd(r)

	err := r.mainerr()
	if err == nil {
		return 0
	}
	switch err := err.(type) {
	case usageErr:
		if err.err != flag.ErrHelp {
			fmt.Fprintln(os.Stderr, err.err)
		}
		fmt.Fprint(os.Stderr, err.u.usage())
		return 2
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}

type runner struct {
	*rootCmd
	genCmd    *genCmd
	initCmd   *initCmd
	helpCmd   *helpCmd
	dockerCmd *dockerCmd
	cueCmd    *cueCmd

	// runtime is the cue.Runtime used for all CUE operations
	context *cue.Context

	// buildInfo is the Go runrimte/debug.BuildInfo associated with the running
	// binary. This information is hashed as part of the calculation to
	// determine whether re-running preguide for a given guide is necessary
	// (because a change in the preguide binary should result in a cache miss)
	buildInfo *debug.BuildInfo

	versionString string

	// guides is the set of guides that we successfully processed, gathered as
	// part of processDir
	guides []*guide

	// schemas are definitions used in the course of validating config, input
	// and re-reading output
	schemas preguide.Schemas

	// seenPrestepPkgs is a cache of the presteps we have seen and resolved
	// to a version in a given run of preguide
	seenPrestepPkgs map[string]string

	// cwd is the current working directory of the process, used when
	// calcuating relative paths to files
	cwd string

	// self is the path to the current executable, as returned by os.Executable()
	self string
}

func newRunner() *runner {
	res := &runner{
		seenPrestepPkgs: make(map[string]string),
	}
	cwd, err := os.Getwd()
	if err != nil {
		panic(err) // we have bigger problems than proper error handling
	}
	res.cwd = cwd
	res.context = cuecontext.New()
	return res
}

func (r *runner) mainerr() (err error) {
	defer util.HandleKnown(&err)

	self, err := os.Executable()
	check(err, "failed to determine self: %v", err)
	r.self, err = filepath.EvalSymlinks(self)
	check(err, "failed to determine path to self from %q: %v", self, err)

	r.readBuildInfo()

	if err := r.fs.Parse(os.Args[1:]); err != nil {
		return usageErr{err, r.rootCmd}
	}

	args := r.fs.Args()
	if len(args) == 0 {
		return r.usageErr("missing command")
	}
	cmd := args[0]
	switch cmd {
	case "gen":
		return r.genCmd.run(args[1:])
	case "init":
		return r.initCmd.run(args[1:])
	case "docker":
		return r.dockerCmd.run(args[1:])
	case "help":
		return r.helpCmd.run(args[1:])
	case "cue":
		return r.cueCmd.run(args[1:])
	default:
		return r.usageErr("unknown command: " + cmd)
	}
}

func (r *runner) readBuildInfo() {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		raise("failed to read build info")
	}
	r.buildInfo = bi
	if bi.Main.Replace != nil {
		bi.Main = *bi.Main.Replace
	}
	if bi.Main.Sum != "" {
		r.versionString = bi.Main.Version + " " + bi.Main.Sum
		return
	}

	// For testing we need a stable version
	if os.Getenv("PREGUIDE_NO_DEVEL_HASH") == "true" {
		r.versionString = "(devel)"
		return
	}

	// Use a sha256 sum of self
	h := sha256.New()
	selfF, err := os.Open(r.self)
	check(err, "failed to open self: %v", err)
	defer selfF.Close()
	_, err = io.Copy(h, selfF)
	check(err, "failed to hash self: %v", err)
	r.versionString = string(h.Sum(nil))
}

// relpath returns p relative to r.cwd, or p in the case of any error
func (r *runner) relpath(p string) string {
	rel, err := filepath.Rel(r.cwd, p)
	if err != nil {
		return p
	}
	return rel
}

func (r *runner) debugf(format string, args ...interface{}) {
	if *r.fDebug {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}
