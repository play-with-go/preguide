package main

import (
	"flag"
	"fmt"
)

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

func (r *runner) runInit(args []string) error {
	return nil
}
