package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

const (
	pullImageMissing = "missing"
)

type usageErr struct {
	err error
	u   cmd
}

func (u usageErr) Error() string { return u.err.Error() }

func main() { os.Exit(main1()) }

func main1() int {
	r := newRunner()

	r.rootCmd = newRootCmd()
	r.genCmd = newGenCmd()
	r.initCmd = newInitCmd()
	r.helpCmd = newHelpCmd(r)

	err := r.mainerr()
	if err == nil {
		return 0
	}
	switch err := err.(type) {
	case usageErr:
		if err.err != flag.ErrHelp {
			fmt.Fprintln(os.Stderr, err.err)
		}
		fmt.Fprint(os.Stderr, err.u.usage())
		return 2
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}

type cmd interface {
	usage() string
	usageErr(format string, args ...interface{}) usageErr
}

type rootCmd struct {
	fs           *flag.FlagSet
	flagDefaults string
	fDebug       *bool
}

func newFlagSet(name string, setupFlags func(*flag.FlagSet)) string {
	res := flag.NewFlagSet(name, flag.ContinueOnError)
	var b bytes.Buffer
	res.SetOutput(&b)
	setupFlags(res)
	res.PrintDefaults()
	res.SetOutput(ioutil.Discard)
	s := b.String()
	const indent = "\t"
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			lines[i] = ""
		} else {
			lines[i] = indent + strings.Replace(l, "\t", "    ", 1)
		}
	}
	return strings.Join(lines, "\n")
}

func newRootCmd() *rootCmd {
	res := &rootCmd{}
	res.flagDefaults = newFlagSet("preguide", func(fs *flag.FlagSet) {
		res.fs = fs
		res.fDebug = fs.Bool("debug", false, "include debug output")
	})
	return res
}

func (r *rootCmd) usage() string {
	return fmt.Sprintf(`
Usage of preguide:

    preguide <command>

The commands are:

    init
    gen

Use "preguide help <command>" for more information about a command.

preguide defines the following flags:

%s`[1:], r.flagDefaults)
}

func (r *rootCmd) usageErr(format string, args ...interface{}) usageErr {
	return usageErr{fmt.Errorf(format, args...), r}
}

type genCmd struct {
	fs               *flag.FlagSet
	flagDefaults     string
	fOutput          *string
	fSkipCache       *bool
	fImageOverride   *string
	fCompat          *bool
	fPullImage       *string
	fPrestepDockExec *string
	fRaw             *bool
}

func newGenCmd() *genCmd {
	res := &genCmd{}
	res.flagDefaults = newFlagSet("preguide gen", func(fs *flag.FlagSet) {
		res.fs = fs
		res.fOutput = fs.String("out", "", "the target directory for generation")
		res.fSkipCache = fs.Bool("skipcache", os.Getenv("PREGUIDE_SKIP_CACHE") == "true", "whether to skip any output cache checking")
		res.fImageOverride = fs.String("image", os.Getenv("PREGUIDE_IMAGE_OVERRIDE"), "the image to use instead of the guide-specified image")
		res.fCompat = fs.Bool("compat", false, "render old-style PWD code blocks")
		res.fPullImage = fs.String("pull", os.Getenv("PREGUIDE_PULL_IMAGE"), "try and docker pull image if missing")
		res.fPrestepDockExec = fs.String("prestep", os.Getenv("PREGUIDE_PRESTEP_DOCKEXEC"), "the image and docker flags passed to dockexec when running the pre-step (if there is one)")
		res.fRaw = fs.Bool("raw", false, "generate raw output for steps")
	})
	return res
}

func (g *genCmd) usage() string {
	return fmt.Sprintf(`
usage: preguide gen

%s`[1:], g.flagDefaults)
}

func (g *genCmd) usageErr(format string, args ...interface{}) usageErr {
	return usageErr{fmt.Errorf(format, args...), g}
}

type initCmd struct {
	fs           *flag.FlagSet
	flagDefaults string
}

func newInitCmd() *initCmd {
	res := &initCmd{}
	res.flagDefaults = newFlagSet("preguide init", func(fs *flag.FlagSet) {
		res.fs = fs
	})
	return res
}

func (i *initCmd) usage() string {
	return fmt.Sprintf(`
usage: preguide init

%s`[1:], i.flagDefaults)
}

func (i *initCmd) usageErr(format string, args ...interface{}) usageErr {
	return usageErr{fmt.Errorf(format, args...), i}
}

type helpCmd struct {
	fs           *flag.FlagSet
	flagDefaults string
	r            *runner
}

func newHelpCmd(r *runner) *helpCmd {
	res := &helpCmd{}
	res.flagDefaults = newFlagSet("preguide help", func(fs *flag.FlagSet) {
		res.fs = fs
	})
	return res
}

func (h *helpCmd) usage() {
	h.r.rootCmd.usage()
}

func (h *helpCmd) usageErr(format string, args ...interface{}) usageErr {
	return h.r.rootCmd.usageErr(format, args...)
}

func check(err error, format string, args ...interface{}) {
	if err != nil {
		if format != "" {
			err = fmt.Errorf(format, args...)
		}
		panic(knownErr{err})
	}
}

func raise(format string, args ...interface{}) {
	panic(knownErr{fmt.Errorf(format, args...)})
}

type knownErr struct{ error }

func handleKnown(err *error) {
	switch r := recover().(type) {
	case nil:
	case knownErr:
		*err = r
	default:
		panic(r)
	}
}
