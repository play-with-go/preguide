package main

//go:generate go run cuelang.org/go/cmd/cue cmd -t workflowsDir=./.github/workflows gengithub ./internal/ci
//go:generate go run cuelang.org/go/cmd/cue cmd -t scriptsDir=./_scripts genenv ./internal/ci
