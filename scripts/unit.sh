#!/usr/bin/env bash
set -euo pipefail

export ROOT=`dirname $(readlink -f ${BASH_SOURCE%/*})`
if [ ! -f $ROOT/.bin/ginkgo ]; then
  echo "Installing ginkgo"
  (cd $ROOT/src/apt/vendor/github.com/onsi/ginkgo/ginkgo/ && go install)
fi

cd $ROOT/src/apt/
ginkgo -r -skipPackage=integration
