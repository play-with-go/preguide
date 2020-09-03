package sanitisers

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type StmtSanitiser func(*S, *syntax.Stmt) Sanitiser
type Sanitiser func(envVars []string, output string) string

type S struct {
	printer *syntax.Printer
}

func NewS() *S {
	return &S{
		printer: syntax.NewPrinter(),
	}
}

func (sm *S) StmtHasCallExprPrefix(stmt *syntax.Stmt, words ...string) bool {
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
