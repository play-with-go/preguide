package ci

import "github.com/SchemaStore/schemastore/src/schemas/json"

test: json.#Workflow & {
	name: "Test"
	env: {
		PREGUIDE_IMAGE_OVERRIDE:   "playwithgo/go1.14.4@sha256:94e0f5cdc221034048679cfd00b29d2410a55696d933c309718530c2bb0f06ea"
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
			run:  #"test -z "$(git status --porcelain)" || (git status; git diff; false)"#
		}]
	}
}
