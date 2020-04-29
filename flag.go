package main

import (
	"flag"
	"fmt"
	"os"
)

type usageErr string

func (u usageErr) Error() string { return string(u) }

type flagErr string

func (f flagErr) Error() string { return string(f) }

func main() { os.Exit(main1()) }

func main1() int {
	r := &runner{}

	fs := flag.NewFlagSet("preguide", flag.ContinueOnError)
	r.flagSet = fs
	fs.Usage = r.usage

	r.fDir = fs.String("dir", ".", "the directory in which to run preguide")
	r.fOutput = fs.String("out", "", "the target directory for generation")
	r.fDebug = fs.Bool("debug", false, "include debug output")
	r.fSkipCache = fs.Bool("skipcache", os.Getenv("PREGUIDE_SKIP_CACHE") == "true", "whether to skip any output cache checking")
	r.fImageOverride = fs.String("image", os.Getenv("PREGUIDE_IMAGE_OVERRIDE"), "the image to use instead of the guide-specified image")
	r.fCompat = fs.Bool("compat", false, "render old-style PWD code blocks")

	err := r.mainerr()
	if err == nil {
		return 0
	}
	switch err.(type) {
	case usageErr:
		fmt.Fprintln(os.Stderr, err)
		r.flagSet.Usage()
		return 2
	case flagErr:
		return 2
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func (r *runner) usage() {
	fmt.Fprintf(os.Stderr, `
Usage of preguide:

`[1:])
	r.flagSet.PrintDefaults()
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
