package preguide

import (
	"tool/exec"
	"tool/file"
	"tool/os"
	"strconv"
)

command: embed: {
	gen: exec.Run & {
		cmd: ["go", "run", "cuelang.org/go/cmd/cue", "def"]
		stdout: string
	}

	pkg: os.Getenv & {
		GOPACKAGE: string
	}

	embed: file.Create & {
		filename: "gen_cue.go"
		contents: """
		package \(pkg.GOPACKAGE)

		// CUEDef is the string quoted output of cue def for the current package. This
		// constant exists as a workaround until the full intent and capability of
		// cuelang.org/go/encoding/gocode/... is established.
		const CUEDef = \(strconv.Quote(gen.stdout))

		"""
	}
}
