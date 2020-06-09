package ci

import "github.com/SchemaStore/schemastore/src/schemas/json"

scriptsDir: *"./" | string @tag(scriptsDir)

test: json.#Workflow & {
	name: "Go"
	env: {
		PREGUIDE_IMAGE_OVERRIDE:   "playwithgo/go1.14.4@sha256:a4bd24f1f831b0ff9674810b258f547961491282da5e339dd1af36f71bc336f8"
		PREGUIDE_PRESTEP_DOCKEXEC: "buildpack-deps@sha256:ec0e9539673254d0cb1db0de347905cdb5d5091df95330f650be071a7e939420"
		PREGUIDE_PULL_IMAGE:       "missing"
	}
	on: {
		push: branches: ["master"]
		pull_request: branches: ["**"]
	}
	jobs: test: {
		strategy: {
			"fail-fast": false
			matrix: {
				os: ["ubuntu-latest"]
				go_version: ["go1.14.4"]
			}
		}
		"runs-on": "${{ matrix.os }}"
		steps: [{
			name: "Checkout code"
			uses: "actions/checkout@v2"
		}, {
			name: "Install Go"
			uses: "actions/setup-go@v2"
			with: "go-version": "${{ matrix.go-version }}"
		}, {
			name: "Verify"
			run:  "go mod verify"
		}, {
			name: "Generate"
			run:  "go generate ./..."
		}, {
			name: "Test"
			run:  "go test ./..."
		}, {
			name: "Tidy"
			run:  "go mod tidy"
		}, {
			name: "Verify commit is clean"
			run:  "test -z \"$(git status --porcelain)\" || (git status; git diff; false)"
		}]
	}
}
