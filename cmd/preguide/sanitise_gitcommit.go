package main

import (
	"regexp"
	"strings"
)

var (
	gitCommitFirstLine = regexp.MustCompile(`^(\[master \(root-commit\) )[0-9a-f]+(\] .*)$`)
)

func sanitiseGitCommit(s string) string {
	lines := strings.SplitN(s, "\n", 2)
	lines[0] = gitCommitFirstLine.ReplaceAllString(lines[0], "${1}abcd123${2}")
	return strings.Join(lines, "\n")
}
