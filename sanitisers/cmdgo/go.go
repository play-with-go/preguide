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
	goTestTestTime  = `\d+(\.\d+)?s`
	goTestMagicTime = `0.042s`

	goGetModVCSPathMagic = "0123456789abcdef"
)

var (
	goTestPassRunHeading = regexp.MustCompile(`^( *--- (PASS|FAIL): .+\()` + goTestTestTime + `\)$`)
	goTestFailSummary    = regexp.MustCompile(`^((FAIL|ok  )\t.+\t)` + goTestTestTime + `$`)

	goGetModVCSPath = regexp.MustCompile(`(pkg/mod/cache/vcs/)[0-9a-f]+`)
)

func CmdGoStmtSanitiser(s *sanitisers.S, stmt *syntax.Stmt) sanitisers.Sanitiser {
	if s.StmtHasCallExprPrefix(stmt, "go", "test") {
		return sanitiseGoTest{}
	}
	if s.StmtHasCallExprPrefix(stmt, "go", "get") {
		return sanitiseGoGet{}
	}
	return nil
}

type sanitiseGoTest struct{}

func (sanitiseGoTest) Output(varNames []string, s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = goTestPassRunHeading.ReplaceAllString(lines[i], fmt.Sprintf("${1}%v)", goTestMagicTime))
		lines[i] = goTestFailSummary.ReplaceAllString(lines[i], "${1}"+goTestMagicTime)
	}
	return strings.Join(lines, "\n")
}

func (sanitiseGoTest) ComparisonOutput(varNames []string, s string) string {
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
