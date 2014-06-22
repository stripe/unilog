#!/bin/sh

set -eu

cleanup() {
    rm -f unilog
    [ "${version-}" ] && rm -f "unilog-${version}"
}

trap cleanup exit

godep go build
verstr=$(./unilog -V)
version=${verstr#This is unilog v}

echo "Built unilog v${version}."
mv unilog "unilog-${version}"

$HOME/stripe/puppet-config/bin/store-blob "unilog-${version}"

echo "Uploaded to s3."
echo " version: $version"
echo " sha1sum: $(sha1sum "unilog-${version}" | cut -f 1 -d' ')"
