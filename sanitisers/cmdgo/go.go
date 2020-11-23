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

	goGetModVCSPath = regexp.MustCompile(`(pkg/mod/cache/vcs/)[0-9a-f]+`)

	goEnvGoGCCFlags = regexp.MustCompile(`(^GOGCCFLAGS=.*-fdebug-prefix-map=)[^ ]*(.*$)`)
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
	return strings.Join(lines, "\n")
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
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = goEnvGoGCCFlags.ReplaceAllString(l, "${1}/tmp/go-build${2}")
	}
	return strings.Join(lines, "\n")
}

func (sanitiseGoEnv) ComparisonOutput(varNames []string, s string) string {
	return s
}
