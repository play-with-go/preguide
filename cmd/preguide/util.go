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
	// beginSeq is the sequence of bytes that start a chunk
	beginSeq []byte

	// endSeq is the sequence of bytes that end a chunk
	endSeq []byte

	// buf is the buffer we are scanning
	buf []byte

	// currPoint is the offset of start of the beingSeq of
	// the current chunk. Hence currPoint != lastPoint is
	// the invariant that holds whilst we are
	currPoint position

	// endPoint is the offset of the most last sequence found using find() as defined
	// in next(). Because next() makes two calls
	// to find (the begin and end sequences), this ensures that after the second
	// call, endP refers to the start of the begin sequence.
	// after the end of the last endSeq
	endPoint position

	// prevSeqStart is the byte offset of the start of the last but one sequence
	// found using find() as defined in next(). Because next() makes two calls
	// to find (the begin and end sequences), this ensures that after the second
	// call, prevSeqStart refers to the start of the begin sequence.
	prevSeqStart position
}

func newChunker(b []byte, beg, end string, startLine int) *chunker {
	if strings.Contains(beg, "\n") || strings.Contains(end, "\n") {
		panic(fmt.Errorf("cannot have a begin or end sequence that contains newline"))
	}
	res := &chunker{
		buf:      b,
		beginSeq: []byte(beg),
		endSeq:   []byte(end),
	}
	// 1-initialise line values
	res.currPoint.line = startLine
	res.endPoint.line = startLine
	res.prevSeqStart.line = startLine
	return res
}

func (c *chunker) next() (bool, error) {
	find := func(key []byte) bool {
		p := bytes.Index(c.buf, key)
		if p == -1 {
			return false
		}
		c.prevSeqStart = c.currPoint

		c.currPoint = c.endPoint
		c.currPoint.offset += p
		numLines := bytes.Count(c.buf[:p], []byte("\n"))
		c.currPoint.line += numLines
		col := p
		if numLines > 0 {
			col -= bytes.LastIndex(c.buf[:p], []byte("\n"))
		}
		c.currPoint.col = col

		c.endPoint = c.currPoint
		c.endPoint.offset += len(key)
		c.endPoint.col += len(key)

		c.buf = c.buf[p+len(key):]
		return true
	}
	if !find(c.beginSeq) {
		return false, nil
	}
	if !find(c.endSeq) {
		return false, fmt.Errorf("failed to find end %q terminator", c.endSeq)
	}
	return true, nil
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

func (c *chunker) pos() position {
	return c.prevSeqStart
}

func (c *chunker) end() position {
	return c.endPoint
}
