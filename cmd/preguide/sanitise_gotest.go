package main

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	goTestTestTime  = `\d+(\.\d+)?s`
	goTestMagicTime = `0.042s`
)

var (
	goTestPassRunHeading = regexp.MustCompile(`^( *--- (PASS|FAIL): .+\()` + goTestTestTime + `\)$`)
	goTestFailSummary    = regexp.MustCompile(`^((FAIL|ok  )\t.+\t)` + goTestTestTime + `$`)
)

func sanitiseGoTest(varNames []string, s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = goTestPassRunHeading.ReplaceAllString(lines[i], fmt.Sprintf("${1}%v)", goTestMagicTime))
		lines[i] = goTestFailSummary.ReplaceAllString(lines[i], "${1}"+goTestMagicTime)
	}
	return strings.Join(lines, "\n")
}
