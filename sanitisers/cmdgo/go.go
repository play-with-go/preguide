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

	pseudoVersion *regexp.Regexp
)

func init() {
	// pseudoVersion is based on the definition in internal/modfetch
	pseudoVersion = regexp.MustCompile(`(v[0-9]+\.(?:0\.0-|\d+\.\d+-(?:[^+]*\.)?0\.))\d{14}-[A-Za-z0-9]+(\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?`)
	pseudoVersion.Longest()
}

func CmdGoStmtSanitiser(s *sanitisers.S, stmt *syntax.Stmt) sanitisers.Sanitiser {
	if s.StmtHasCallExprPrefix(stmt, "go", "test") {
		return sanitiseGoTest
	}
	if s.StmtHasCallExprPrefix(stmt, "go", "get") {
		return sanitiseGoGet
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

// sanitiseGoGet finds lines that include {{.ENV}} patterns, and then replaces
// all psuedoversions on that line. If this logic needs to be refined for more
// precise replacing of pseudo versions, so be it: add the complexity below
func sanitiseGoGet(varNames []string, s string) string {
	lines := strings.Split(s, "\n")
	for _, v := range varNames {
		for i := range lines {
			vRegexp := regexp.MustCompile(regexp.QuoteMeta(v))
			if !vRegexp.MatchString(lines[i]) {
				continue
			}
			lines[i] = pseudoVersion.ReplaceAllString(lines[i], "${1}20060102150405-abcde12345${2}")
		}
	}
	return strings.Join(lines, "\n")
}
