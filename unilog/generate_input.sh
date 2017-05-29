#! /bin/bash
# This script will generate input at different clevels
# to help test behavior locally.

set -e


function dumpText(){
  echo "this is a sheddable clevel=sheddable"
  echo "this is a sheddableplus clevel=sheddableplus"
  echo "this is a critical clevel=critical"
  echo "this is a criticalplus clevel=criticalplus"
  echo "this is an ERROR clevel=sleddable"
  echo "this is a default (sheddableplus)"
}



while true; do
  dumpText
  sleep 2
done

