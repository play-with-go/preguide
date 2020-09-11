#!/usr/bin/env bash

set -eu -o pipefail
shopt -s inherit_errexit

source "${BASH_SOURCE%/*}/env_common.bash"
source "${BASH_SOURCE%/*}/image.bash"

$export PREGUIDE_IMAGE_OVERRIDE "$preguide_image_override"
