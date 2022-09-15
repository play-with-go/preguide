// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package sanitisers

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type StmtSanitiser func(*S, *syntax.Stmt) Sanitiser
type Sanitiser interface {
	Output(envVars []string, output string) string
	ComparisonOutput(envVars []string, output string) string
}

type S struct {
	printer *syntax.Printer
}

func NewS() *S {
	return &S{
		printer: syntax.NewPrinter(syntax.SingleLine(true)),
	}
}

func (sm *S) StmtHasCallExprPrefix(stmt *syntax.Stmt, words ...string) bool {
	return sm.compare(func(words []string, stmts []*syntax.Word) bool {
		return len(words) <= len(stmts)
	}, stmt, words...)
}

func (sm *S) StmtIsCallExpr(stmt *syntax.Stmt, words ...string) bool {
	return sm.compare(func(words []string, stmts []*syntax.Word) bool {
		return len(words) == len(stmts)
	}, stmt, words...)
}

func (sm *S) compare(cmp func([]string, []*syntax.Word) bool, stmt *syntax.Stmt, words ...string) bool {
	ce, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok {
		return false
	}
	if !cmp(words, ce.Args) {
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

func (sm *S) StmtHasStringPrefix(stmt *syntax.Stmt, prefix string) bool {
	var sb strings.Builder
	sm.printer.Print(&sb, stmt)
	return strings.HasPrefix(sb.String(), prefix)
}
