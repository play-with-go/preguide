package main

import (
	"fmt"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	testTime  = `\d+(\.\d+)?s`
	magicTime = `0.042s`
)

var (
	passRunHeading = regexp.MustCompile(`^( *--- (PASS|FAIL): .+\()` + testTime + `\)$`)
	failSummary    = regexp.MustCompile(`^((FAIL|ok  )\t.+\t)` + testTime + `$`)
)

func sanitiseGoTest(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = passRunHeading.ReplaceAllString(lines[i], fmt.Sprintf("${1}%v)", magicTime))
		lines[i] = failSummary.ReplaceAllString(lines[i], "${1}"+magicTime)
	}
	return strings.Join(lines, "\n")
}

type sanitiser func(string) string

type sanitiserMatcher struct {
	printer *syntax.Printer
}

func (sm *sanitiserMatcher) deriveSanitiser(stmt *syntax.Stmt) sanitiser {
	switch {
	case sm.matchGoTest(stmt):
		return sanitiseGoTest
	}
	return nil
}

func (sm *sanitiserMatcher) matchGoTest(stmt *syntax.Stmt) bool {
	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		return sm.matchWords([]string{"go", "test"}, cmd.Args)
	}
	return false
}

func (sm *sanitiserMatcher) matchWords(want []string, args []*syntax.Word) bool {
	if len(args) < len(want) {
		return false
	}
	for i := range want {
		var sb strings.Builder
		sm.printer.Print(&sb, args[i])
		if want[i] != sb.String() {
			return false
		}
	}
	return true
}
