#!/usr/bin/env bash

source "${BASH_SOURCE%/*}/common.bash"
source "${BASH_SOURCE%/*}/image.bash"

export="$2"
alias="$3"

if [ "$export" = "" ]
then
	export="export"
fi

if [ "$alias" = "" ]
then
	alias="alias"
fi

echo $export PREGUIDE_IMAGE_OVERRIDE="$preguide_image_override"
echo $export PREGUIDE_PRESTEP_DOCKEXEC="$preguide_prestep_dockexec"
