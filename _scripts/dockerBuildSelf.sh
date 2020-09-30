#!/usr/bin/env bash

set -eu -o pipefail
shopt -s inherit_errexit

export DOCKER_BUILDKIT=1

if [ "${GITHUB_WORKFLOW:-}" == "Docker self" ]
then
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
elif [ "$(go list -m)" == "github.com/play-with-go/preguide" ]
then
	version="devel"
else
	# We are calling this script from elsewhere with a viewing simply
	# building the preguide image
	version="$(go list -m -f {{.Version}} github.com/play-with-go/preguide)"
fi

dir="$(go list -m -f {{.Dir}} github.com/play-with-go/preguide)"

docker build -f $dir/cmd/preguide/Dockerfile -t playwithgo/preguide:$version $dir

if [ "${GITHUB_WORKFLOW:-}" == "Docker self" ]
then
	docker push playwithgo/preguide:$version
fi
