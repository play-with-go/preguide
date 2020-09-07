// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package git

import (
	"regexp"
	"strings"

	"github.com/play-with-go/preguide/sanitisers"
	"mvdan.cc/sh/v3/syntax"
)

var (
	gitCommitFirstLine = regexp.MustCompile(`^(\[[[:alpha:]]+ \(root-commit\) )[0-9a-f]+(\] .*)$`)
)

func GitStmtSanitiser(s *sanitisers.S, stmt *syntax.Stmt) sanitisers.Sanitiser {
	if s.StmtHasCallExprPrefix(stmt, "git", "commit") {
		return sanitiseGitCommit
	}
	return nil
}

func sanitiseGitCommit(varNames []string, s string) string {
	lines := strings.SplitN(s, "\n", 2)
	lines[0] = gitCommitFirstLine.ReplaceAllString(lines[0], "${1}abcd123${2}")
	return strings.Join(lines, "\n")
}
