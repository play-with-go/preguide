package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type sanitiser func(string) string

type sanitiserMatcher struct {
	printer *syntax.Printer
}

func (sm *sanitiserMatcher) deriveSanitiser(stmt *syntax.Stmt) []sanitiser {
	var res []sanitiser
	if sm.stmtHasCallExprPrefix(stmt, "go", "test") {
		res = append(res, sanitiseGoTest)
	}
	if sm.stmtHasCallExprPrefix(stmt, "git", "commit") {
		res = append(res, sanitiseGitCommit)
	}
	return res
}

func (sm *sanitiserMatcher) stmtHasCallExprPrefix(stmt *syntax.Stmt, words ...string) bool {
	ce, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok {
		return false
	}
	if len(words) > len(ce.Args) {
		return false
	}
	for i, word := range words {
		var sb strings.Builder
		sm.printer.Print(&sb, ce.Args[i])
		if word != sb.String() {
			return false
		}
	}
	return true
}
