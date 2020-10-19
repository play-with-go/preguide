package main

import (
	"sync"

	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/load"
)

var cueLoadMutxex sync.Mutex

func cueLoadInstances(args []string, c *load.Config) []*build.Instance {
	cueLoadMutxex.Lock()
	defer cueLoadMutxex.Unlock()
	return load.Instances(args, c)
}
