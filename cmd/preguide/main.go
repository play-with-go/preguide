// preguide is a pre-processor for Play With Docker-based guides
package main

// A note on Go types, CUE definitions and code generation
// =======================================================
// Ideally we would have Go types be the source of truth for this entire
// program. The Go package github.com/play-with-go/preguide/internal/types
// would be the source of truth for the github.com/play-with-go/preguide CUE
// definitions, and the Go types defined in github.com/play-with-go/preguide
// would be the source of truth for the types defined in
// github.com/play-with-go/preguide/out CUE definitions.
//
// However, as github.com/cuelang/cue/discussions/462 concludes, there isn't currently
// a good story on how to handle converting Go interface types to CUE definitions.
// So for now we manually define the two.
//
// Theoretically we could code generate some of these types

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/encoding/gocode/gocodec"
	"github.com/play-with-go/preguide/internal/embed"
)

func main() { os.Exit(main1()) }

func main1() int {
	r := newRunner()

	r.rootCmd = newRootCmd(r)
	r.genCmd = newGenCmd(r)
	r.initCmd = newInitCmd(r)
	r.helpCmd = newHelpCmd(r)
	r.dockerCmd = newDockerCmd(r)

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

type runner struct {
	*rootCmd
	genCmd    *genCmd
	initCmd   *initCmd
	helpCmd   *helpCmd
	dockerCmd *dockerCmd

	// runtime is the cue.Runtime used for all CUE operations
	runtime cue.Runtime

	// codec is the *gocodec.Codec based on runtime
	codec *gocodec.Codec

	// buildInfo is the Go runrimte/debug.BuildInfo associated with the running
	// binary. This information is hashed as part of the calculation to
	// determine whether re-running preguide for a given guide is necessary
	// (because a change in the preguide binary should result in a cache miss)
	buildInfo string

	// guides is the set of guides that we successfully processed, gathered as
	// part of processDir
	guides []*guide

	// Definitions used in the course of validating config, input and
	// re-reading output
	confDef        cue.Value
	guideDef       cue.Value
	commandDef     cue.Value
	commandFileDef cue.Value
	uploadDef      cue.Value
	uploadFileDef  cue.Value
	guideOutDef    cue.Value
	commandStep    cue.Value
	uploadStep     cue.Value

	// seenPrestepPkgs is a cache of the presteps we have seen and resolved
	// to a version in a given run of preguide
	seenPrestepPkgs map[string]string
}

func newRunner() *runner {
	res := &runner{
		seenPrestepPkgs: make(map[string]string),
	}
	res.codec = gocodec.New(&res.runtime, nil)
	return res
}

func (r *runner) mainerr() (err error) {
	defer handleKnown(&err)

	r.readBuildInfo()

	if err := r.rootCmd.fs.Parse(os.Args[1:]); err != nil {
		return usageErr{err, r.rootCmd}
	}

	args := r.rootCmd.fs.Args()
	if len(args) == 0 {
		return r.rootCmd.usageErr("missing command")
	}
	cmd := args[0]
	switch cmd {
	case "gen":
		return r.genCmd.run(args[1:])
	case "init":
		return r.initCmd.run(args[1:])
	case "docker":
		return r.dockerCmd.run(args[1:])
	case "help":
		return r.helpCmd.run(args[1:])
	default:
		return r.rootCmd.usageErr("unknown command: " + cmd)
	}
}

func (r *runner) readBuildInfo() {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		raise("failed to read build info")
	}
	if bi.Main.Replace != nil {
		bi.Main = *bi.Main.Replace
	}
	if bi.Main.Sum == "" {
		// Local development. Use the export information if it is available
		export := exec.Command("go", "list", "-mod=readonly", "-export", "-f={{.Export}}", "github.com/play-with-go/preguide")
		out, err := export.CombinedOutput()
		if err == nil {
			r.buildInfo = string(out)
		} else {
			// The only really conceivable case where this should happen is development
			// of preguide itself. In that case, we will be running testscript tests
			// that start from a clean slate.
			r.buildInfo = "devel"
		}
	} else {
		r.buildInfo = bi.Main.Version + " " + bi.Main.Sum
	}
}

// loadSchemas loads the CUE schemas (definitions) we require to validate the
// configuration, input and output to preguide gen. The load process uses a CUE
// overlay wrapped around the the go-bindata generated embedding.
func (r *runner) loadSchemas() {
	overlay := make(map[string]load.Source)
	for _, asset := range embed.AssetNames() {
		contents, err := embed.Asset(asset)
		if err != nil {
			panic(err)
		}
		overlay[filepath.Join("/", asset)] = load.FromBytes(contents)
	}
	conf := &load.Config{
		Dir:     "/",
		Overlay: overlay,
	}
	bps := load.Instances([]string{".", "./out"}, conf)
	preguide, err := r.runtime.Build(bps[0])
	check(err, "failed to compile github.com/play-with-go/preguide package: %v", err)
	preguideOut, err := r.runtime.Build(bps[1])
	check(err, "failed to compile github.com/play-with-go/preguide/out package: %v", err)

	mustFind := func(v cue.Value) cue.Value {
		check(v.Err(), "failed to find definition: %v", v)
		return v
	}

	r.confDef = mustFind(preguide.LookupDef("#PrestepServiceConfig"))
	r.guideDef = mustFind(preguide.LookupDef("#Guide"))
	r.commandDef = mustFind(preguide.LookupDef("#Command"))
	r.commandFileDef = mustFind(preguide.LookupDef("#CommandFile"))
	r.uploadDef = mustFind(preguide.LookupDef("#Upload"))
	r.uploadFileDef = mustFind(preguide.LookupDef("#UploadFile"))
	r.guideOutDef = mustFind(preguideOut.LookupDef("#GuideOutput"))
	r.commandStep = mustFind(preguideOut.LookupDef("#CommandStep"))
	r.uploadStep = mustFind(preguideOut.LookupDef("#UploadStep"))
}

func (r *runner) debugf(format string, args ...interface{}) {
	if *r.fDebug {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}
