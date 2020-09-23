#!/usr/bin/env bash

set -eu -o pipefail
shopt -s inherit_errexit

set -x

version="$(basename $GITHUB_REF)"
if [ "$version" == "main" ]
then
	version="$GITHUB_SHA"
fi

docker login -u $DOCKER_HUB_USER -p $DOCKER_HUB_TOKEN

cd $(mktemp -d)
go mod init mod.com
go get -d github.com/play-with-go/preguide@$version

# Re-resolve to a version, ensures we resolve a pseudo-version
# if we previously supplied a commit to go get
version="$(go list -m -f {{.Version}} github.com/play-with-go/preguide)"
dir="$(go list -m -f {{.Dir}} github.com/play-with-go/preguide)"

go build github.com/play-with-go/preguide/cmd/preguide

docker build -f $dir/cmd/preguide/Dockerfile -t playwithgo/preguide:$version .
docker push playwithgo/preguide:$version
