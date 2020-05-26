#!/usr/bin/env bash

source "${BASH_SOURCE%/*}/common.bash"

cd $(git rev-parse --show-toplevel)

targetDir=cue.mod/pkg/github.com/SchemaStore/schemastore/src/schemas/json
targetFile=$targetDir/workflow.cue

mkdir -p $targetDir

rm -f $targetFile
curl -s https://raw.githubusercontent.com/SchemaStore/schemastore/master/src/schemas/json/github-workflow.json | \
	go run cuelang.org/go/cmd/cue import -f -p schemas -l ''#Schema: jsonschema: - --outfile $targetFile
