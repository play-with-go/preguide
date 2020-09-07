// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"bytes"
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

// A chunker is an iterator that allows walking over a []byte input, with steps
// for each block found, where a block is identified by a start and end []byte
// sequence. For preguide the start and end blocks are <!-- and -->
// respectively, and the input is the guide prose that contains these
// directives.
type chunker struct {
	b   string
	e   string
	buf []byte
	p   int
	ep  int
	lp  int
}

func newChunker(b []byte, beg, end string) *chunker {
	return &chunker{
		buf: b,
		b:   beg,
		e:   end,
	}
}

func (c *chunker) next() (bool, error) {
	find := func(key string) bool {
		p := bytes.Index(c.buf, []byte(key))
		if p == -1 {
			return false
		}
		c.lp = c.p
		c.p = c.ep + p
		c.ep += p + len(key)
		c.buf = c.buf[p+len(key):]
		return true
	}
	if !find(c.b) {
		return false, nil
	}
	if !find(c.e) {
		return false, fmt.Errorf("failed to find end %q terminator", c.e)
	}
	return true, nil
}

func (c *chunker) pos() int {
	return c.lp
}

func (c *chunker) end() int {
	return c.ep
}
