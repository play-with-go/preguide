package main

import (
	"flag"
	"fmt"
)

type helpCmd struct {
	*runner
	fs           *flag.FlagSet
	flagDefaults string
}

func newHelpCmd(r *runner) *helpCmd {
	res := &helpCmd{
		runner: r,
	}
	res.flagDefaults = newFlagSet("preguide help", func(fs *flag.FlagSet) {
		res.fs = fs
	})
	return res
}

func (hc *helpCmd) usageErr(format string, args ...interface{}) usageErr {
	return hc.rootCmd.usageErr(format, args...)
}

func (hc *helpCmd) run(args []string) error {
	if len(args) != 1 {
		return hc.helpCmd.usageErr("help takes a single command argument")
	}
	var u func() string
	switch args[0] {
	case "gen":
		u = hc.genCmd.usage
	case "init":
		u = hc.initCmd.usage
	case "help":
		u = hc.rootCmd.usage
	default:
		return hc.helpCmd.usageErr("no help available for command %v", args[0])
	}
	fmt.Print(u())
	return nil
}
