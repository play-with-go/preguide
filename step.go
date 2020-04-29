package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"mvdan.cc/sh/syntax"
)

type langSteps struct {
	Steps      map[string]step
	bashScript string
	Hash       string
	steps      []step
}

type step interface {
	name() string
	order() int
	render(io.Writer)
	renderCompat(io.Writer)
	renderTestLog(io.Writer)
}

type commandStep struct {
	// Extract once we have a solution to cuelang.org/issue/376
	Name  string
	Order int

	Stmts []*commandStmt
}

func (c *commandStep) name() string {
	return c.Name
}

func (c *commandStep) order() int {
	return c.Order
}

type commandStmt struct {
	Negated     bool
	CmdStr      string
	ExitCode    int
	Output      string
	outputFence string
}

// commandStepFromString takes a string value that is a sequence of shell
// statements and returns a commandStep with the individual parsed statements,
// or an error in case s cannot be parsed
func commandStepFromString(name string, order int, s string) (*commandStep, error) {
	r := strings.NewReader(s)
	f, err := syntax.NewParser().Parse(r, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse command string %q: %v", s, err)
	}
	return commadStepFromSyntaxFile(name, order, f)
}

// commandStepFromFile takes a path to a file that contains a sequence of shell
// statements and returns a commandStep with the individual parsed statements,
// or an error in case path cannot be read or parsed
func commandStepFromFile(name string, order int, path string) (*commandStep, error) {
	byts, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %v: %v", path, err)
	}
	r := bytes.NewReader(byts)
	f, err := syntax.NewParser().Parse(r, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse commands from %v: %v", path, err)
	}
	return commadStepFromSyntaxFile(name, order, f)
}

// commadStepFromSyntaxFile takes a *mvdan.cc/sh/syntax.File and returns a
// commandStep with the individual statements, or an error in case any of the
// statements cannot be printed as string values
func commadStepFromSyntaxFile(name string, order int, f *syntax.File) (*commandStep, error) {
	res := &commandStep{}
	res.Name = name
	res.Order = order
	printer := syntax.NewPrinter()
	for i, stmt := range f.Stmts {
		// Capture whether this statement is negated or not
		negated := stmt.Negated
		// Set to not negated because we need to capture the exit code.
		// Handling of the exit code and negated happens in the generated
		// bash script
		stmt.Negated = false
		var sb strings.Builder
		if err := printer.Print(&sb, stmt); err != nil {
			return res, fmt.Errorf("failed to print statement %v: %v", i, err)
		}
		res.Stmts = append(res.Stmts, &commandStmt{
			CmdStr:  sb.String(),
			Negated: negated,
		})
	}
	return res, nil
}

func (c *commandStep) render(w io.Writer) {
	fmt.Fprintf(w, "```.term1\n")
	for _, stmt := range c.Stmts {
		fmt.Fprintf(w, "$ %s\n", stmt.CmdStr)
		fmt.Fprintf(w, "%s", stmt.Output)
	}
	fmt.Fprintf(w, "```\n")
}

func (c *commandStep) renderCompat(w io.Writer) {
	fmt.Fprintf(w, "```.term1\n")
	for _, stmt := range c.Stmts {
		fmt.Fprintf(w, "%s\n", stmt.CmdStr)
	}
	fmt.Fprintf(w, "```\n")
}

func (c *commandStep) renderTestLog(w io.Writer) {
	for _, stmt := range c.Stmts {
		fmt.Fprintf(w, "$ %s\n", stmt.CmdStr)
		fmt.Fprintf(w, "%s", stmt.Output)
	}
}

type uploadStep struct {
	// Extract once we have a solution to cuelang.org/issue/376
	Name  string
	Order int

	Source string
	Target string
}

func (u *uploadStep) name() string {
	return u.Name
}

func (u *uploadStep) order() int {
	return u.Order
}

func uploadStepFromSource(name string, order int, source, target string) *uploadStep {
	return &uploadStep{
		Name:   name,
		Order:  order,
		Source: source,
		Target: target,
	}
}

func uploadStepFromFile(name string, order int, path, target string) (*uploadStep, error) {
	byts, err := ioutil.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("failed to read %v: %v", path, err)
	}
	res := &uploadStep{
		Name:   name,
		Order:  order,
		Source: string(byts),
		Target: target,
	}
	return res, nil
}

func (u *uploadStep) render(w io.Writer) {
	fmt.Fprintf(w, "```.term1\n")
	fmt.Fprintf(w, "%s\n", u.Source)
	fmt.Fprintf(w, "```\n")
}

func (u *uploadStep) renderCompat(w io.Writer) {
	fmt.Fprintf(w, "```.term1\n")
	source := strings.ReplaceAll(u.Source, "\t", "        ")
	fmt.Fprintf(w, "cat <<EOD > %v\n%s\nEOD\n", u.Target, source)
	fmt.Fprintf(w, "```\n")
}

func (u *uploadStep) renderTestLog(w io.Writer) {
	fmt.Fprintf(w, "$ cat <<EOD > %v\n%s\nEOD\n", u.Target, u.Source)
}
