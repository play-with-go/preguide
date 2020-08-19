package ci

import "github.com/SchemaStore/schemastore/src/schemas/json"

test: json.#Workflow & {
	name: "Test"
	env: {
		PREGUIDE_IMAGE_OVERRIDE: "playwithgo/go1.15@sha256:7bf7788bb2291af0830a8c62cf38e3bfbd8b0d53e5435531458c8f77bd3efaeb"
		PREGUIDE_PULL_IMAGE:     "missing"
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
				go_version: ["1.15"]
			}
		}
		"runs-on": "${{ matrix.os }}"
		steps: [{
			name: "Checkout code"
			uses: "actions/checkout@v2"
		}, {
			name: "Install Go"
			uses: "actions/setup-go@v2"
			with: "go-version": "${{ matrix.go_version }}"
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
			name: "staticcheck"
			run:  "go run honnef.co/go/tools/cmd/staticcheck ./..."
		}, {
			name: "Tidy"
			run:  "go mod tidy"
		}, {
			name: "Verify commit is clean"
			run:  #"test -z "$(git status --porcelain)" || (git status; git diff; false)"#
		}]
	}
}
