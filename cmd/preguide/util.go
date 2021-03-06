// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/play-with-go/preguide/internal/util"
)

// check verifies that err is nil, else it parnics wrapping err in a knownErr
// (which is recovered by my mainerr). This allows clean, fluent code without
// lots of error handling, where that error handling would otherwise simply
// bubble an error up the stack.
func check(err error, format string, args ...interface{}) {
	if err != nil {
		if format != "" {
			err = fmt.Errorf(format, args...)
		}
		panic(util.KnownErr{Err: err})
	}
}

// raise raises a knownErr, wrapping a fmt.Errorf generated error using the
// provided format and args. See the documentation for check on why these
// functions exist.
func raise(format string, args ...interface{}) {
	panic(util.KnownErr{Err: fmt.Errorf(format, args...)})
}

// stringFlagList is a supporting type for generating a string flag that can
// appear multiple times.
type stringFlagList struct {
	vals *[]string
}

func (s stringFlagList) String() string {
	if s.vals == nil {
		return ""
	}
	return strings.Join(*s.vals, " ")
}

func (s stringFlagList) Set(v string) error {
	*s.vals = append(*s.vals, v)
	return nil
}

var markdownFile = regexp.MustCompile(`.(md|mkdn?|mdown|markdown)$`)

// isMarkdown determines whether name is a markdown file name
func isMarkdown(name string) (string, bool) {
	ext := markdownFile.FindString(name)
	return ext, ext != ""
}

type position struct {
	offset int
	line   int
	col    int
}

// String returns the human readable line:col representation
func (p position) String() string {
	return fmt.Sprintf("%v:%v", p.line, p.col)
}
