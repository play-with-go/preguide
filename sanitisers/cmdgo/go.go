// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package cmdgo

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/play-with-go/preguide/sanitisers"
	"mvdan.cc/sh/v3/syntax"
)

const (
	goTestTestTime = `\d+(\.\d+)?`

	goGetModVCSPathMagic = "0123456789abcdef"
)

var (
	goTestPassRunHeading = regexp.MustCompile(`^( *--- (PASS|FAIL): .+\()` + goTestTestTime + `s\)$`)
	goTestFailSummary    = regexp.MustCompile(`^((FAIL|ok  )\t.+\t)` + goTestTestTime + `s$`)
	goTestBench          = regexp.MustCompile(`^([^\s]+)\s+\d+\s+` + goTestTestTime + ` ns/op$`)
	goTestBenchOSArch    = regexp.MustCompile(`(?m)^goos: .*\ngoarch: .*\n`)

	goGetModVCSPath = regexp.MustCompile(`(pkg/mod/cache/vcs/)[0-9a-f]+`)

	goEnvToolDir    = regexp.MustCompile(`^(GOTOOLDIR=.*/)[^/]*`)
	goEnvGOOS       = regexp.MustCompile(`^(GOOS=).*`)
	goEnvGOARCH     = regexp.MustCompile(`^(GO(HOST)?ARCH=).*`)
	goEnvGoGCCFlags = regexp.MustCompile(`(^GOGCCFLAGS=).*`)
)

func CmdGoStmtSanitiser(s *sanitisers.S, stmt *syntax.Stmt) sanitisers.Sanitiser {
	if s.StmtHasCallExprPrefix(stmt, "go", "test") {
		// We know it's a call expression
		ce := stmt.Cmd.(*syntax.CallExpr)
		bench := false
	Args:
		for _, a := range ce.Args {
			switch p := a.Parts[0].(type) {
			case *syntax.Lit:
				if strings.HasPrefix(p.Value, "-bench") {
					bench = true
					break Args
				}
			}
		}
		return sanitiseGoTest{
			bench: bench,
		}
	}
	// TODO: need to work out how to generalise the hack for subshell go get
	if s.StmtHasCallExprPrefix(stmt, "go", "get") || s.StmtHasStringPrefix(stmt, "(cd $(mktemp -d); GO111MODULE=on go get") {
		return sanitiseGoGet{}
	}
	if s.StmtHasCallExprPrefix(stmt, "go", "env") {
		return sanitiseGoEnv{}
	}
	return nil
}

type sanitiseGoTest struct {
	bench bool
}

func (gt sanitiseGoTest) Output(varNames []string, s string) string {
	if gt.bench {
		s = goTestBenchOSArch.ReplaceAllString(s, "goos: linux\ngoarch: amd64\n")
	}
	return s
}

func (gt sanitiseGoTest) ComparisonOutput(varNames []string, s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		if gt.bench {
			lines[i] = goTestBench.ReplaceAllString(lines[i], "${1} NN N ns/op")
		}
		lines[i] = goTestPassRunHeading.ReplaceAllString(lines[i], "${1}N.NNs)")
		lines[i] = goTestFailSummary.ReplaceAllString(lines[i], "${1}N.NNs")
	}
	s = strings.Join(lines, "\n")
	return s
}

type sanitiseGoGet struct{}

func (sanitiseGoGet) Output(varNames []string, s string) string {
	// If we ever see something that looks like it's from the module vcs cache
	// sanitise that to something standard.. because there is no command that
	// can be run to list that path
	s = goGetModVCSPath.ReplaceAllString(s, fmt.Sprintf("${1}%v", goGetModVCSPathMagic))
	return s
}

func (sanitiseGoGet) ComparisonOutput(varNames []string, s string) string {
	// TODO: be more precise, and only do when "downloading" appears?
	lines := strings.Split(s, "\n")
	sort.Stable(sort.StringSlice(lines))
	return strings.Join(lines, "\n")
}

type sanitiseGoEnv struct{}

func (sanitiseGoEnv) Output(varNames []string, s string) string {
	// Deal with the fact that go env is not stable when it comes to GOGCCFLAGS=
	// by removing that line. It's more hassle than it's worth right now to
	// do anything sensible with it
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = goEnvToolDir.ReplaceAllString(lines[i], `${1}linux_amd64"`)
		lines[i] = goEnvGOOS.ReplaceAllString(lines[i], `${1}"linux"`)
		lines[i] = goEnvGOARCH.ReplaceAllString(lines[i], `${1}"amd64"`)
		lines[i] = goEnvGoGCCFlags.ReplaceAllString(lines[i], `${1}"fake_gcc_flags"`)
	}
	return strings.Join(lines, "\n")
}

func (sanitiseGoEnv) ComparisonOutput(varNames []string, s string) string {
	return s
}
