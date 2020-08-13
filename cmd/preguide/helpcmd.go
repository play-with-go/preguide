package main

import (
	"flag"
	"fmt"
)

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

func (h *helpCmd) usageErr(format string, args ...interface{}) usageErr {
	return h.r.rootCmd.usageErr(format, args...)
}

func (r *runner) runHelp(args []string) error {
	if len(args) != 1 {
		return r.helpCmd.usageErr("help takes a single command argument")
	}
	var u func() string
	switch args[0] {
	case "gen":
		u = r.genCmd.usage
	case "init":
		u = r.initCmd.usage
	case "help":
		u = r.rootCmd.usage
	default:
		return r.helpCmd.usageErr("no help available for command %v", args[0])
	}
	fmt.Print(u())
	return nil
}
