package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/load"
	"github.com/rogpeppe/go-internal/gotooltest"
	"github.com/rogpeppe/go-internal/testscript"
)

var (
	fUpdateScripts = flag.Bool("update", false, "whether to update testscript scripts that use cmp with (stdout|stderr)")
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"preguide": main1,
	}))
}

func TestScripts(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required to run preguide tests")
	}

	p := testscript.Params{
		UpdateScripts: *fUpdateScripts,
		Dir:           filepath.Join("testdata"),
		Setup: func(env *testscript.Env) (err error) {
			defer handleKnown(&err)

			// Remove pre-existing temp dir
			currTmp := filepath.Join(env.WorkDir, "tmp")
			err = os.RemoveAll(currTmp)
			check(err, "failed to remove pre-existing tmp dir %v: %v", currTmp, err)

			newTmp := filepath.Join(env.WorkDir, ".tmp")
			env.Vars = append(env.Vars,
				"TMPDIR="+newTmp,
				"PREGUIDE_IMAGE_OVERRIDE="+os.Getenv("PREGUIDE_IMAGE_OVERRIDE"),
				"PREGUIDE_PULL_IMAGE=missing",
				"PREGUIDE_PRESTEP_DOCKEXEC="+os.Getenv("PREGUIDE_PRESTEP_DOCKEXEC"),
			)
			err = os.Mkdir(newTmp, 0777)
			check(err, "failed to create %v: %v", newTmp, err)

			// Despite the fact that preguide embeds the definitions it needs,
			// it's more convenient to write guides' CUE packages and have them
			// import the preguide packages, for validation etc. That is to say,
			// if we _didn't_ import the preguide packages as part of a guide's
			// CUE package we would not be able to validate, code complete etc
			// independent of running preguide itself (which isn't ideal)
			err = modInit(env.WorkDir)
			check(err, "failed to mod init: %v", err)

			// Always generate an output directory to save typing in each script
			outDir := filepath.Join(env.WorkDir, "_output")
			err = os.Mkdir(outDir, 0777)
			check(err, "failed to create %v: %v", outDir, err)

			return nil
		},
	}
	if err := gotooltest.Setup(&p); err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, p)
}

func modInit(dir string) (err error) {
	defer handleKnown(&err)
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return fmt.Errorf("%v is not a directory: %v", dir, err)
	}
	modDir := filepath.Join(dir, "cue.mod")
	err = os.Mkdir(modDir, 0777)
	check(err, "failed to mkdir %v: %v", modDir, err)

	modFile := filepath.Join(modDir, "module.cue")
	err = ioutil.WriteFile(modFile, []byte("module: \"mod.com\"\n"), 0666)
	check(err, "failed to write module file %v: %v", modFile, err)

	bps := load.Instances([]string{".", "./out"}, nil)
	for _, bp := range bps {
		pkgRootBits := []string{modDir, "pkg"}
		pkgRootBits = append(pkgRootBits, strings.Split(bp.Module, "/")...)
		pkgRoot := filepath.Join(pkgRootBits...)

		for _, f := range bp.CUEFiles {
			sfp := filepath.Join(bp.Root, f)
			tfp := filepath.Join(pkgRoot, f)
			td := filepath.Dir(tfp)

			err = os.MkdirAll(td, 0777)
			check(err, "failed to create directory %v: %v", td, err)

			tf, err := os.Create(tfp)
			check(err, "failed to create %v: %v", tfp, err)

			sf, err := os.Open(sfp)
			check(err, "failed to open %v: %v", sfp, err)

			_, err = io.Copy(tf, sf)
			check(err, "failed to copy from %v to %v: %v", sfp, tfp, err)

			err = tf.Close()
			check(err, "failed to close %v: %v", tfp, err)
		}
	}
	return nil
}
