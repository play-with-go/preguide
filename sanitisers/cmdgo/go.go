// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package cmdgo

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/play-with-go/preguide/sanitisers"
	"mvdan.cc/sh/v3/syntax"
)

const (
	goTestTestTime  = `\d+(\.\d+)?s`
	goTestMagicTime = `0.042s`
)

var (
	goTestPassRunHeading = regexp.MustCompile(`^( *--- (PASS|FAIL): .+\()` + goTestTestTime + `\)$`)
	goTestFailSummary    = regexp.MustCompile(`^((FAIL|ok  )\t.+\t)` + goTestTestTime + `$`)
)

func CmdGoStmtSanitiser(s *sanitisers.S, stmt *syntax.Stmt) sanitisers.Sanitiser {
	if s.StmtHasCallExprPrefix(stmt, "go", "test") {
		return sanitiseGoTest
	}
	return nil
}

func sanitiseGoTest(varNames []string, s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = goTestPassRunHeading.ReplaceAllString(lines[i], fmt.Sprintf("${1}%v)", goTestMagicTime))
		lines[i] = goTestFailSummary.ReplaceAllString(lines[i], "${1}"+goTestMagicTime)
	}
	return strings.Join(lines, "\n")
}
