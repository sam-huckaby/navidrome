#!/bin/sh
set -eu

if command -v fpcalc >/dev/null 2>&1; then
  fpcalc -version
  exit 0
fi

case "$(uname -s)" in
  Darwin)
    if ! command -v brew >/dev/null 2>&1; then
      >&2 printf 'Homebrew is required: https://brew.sh\n'
      exit 1
    fi
    brew install chromaprint
    ;;
  Linux)
    if ! command -v apt-get >/dev/null 2>&1; then
      >&2 printf 'Install the Chromaprint fpcalc package for your Linux distribution.\n'
      exit 1
    fi
    sudo apt-get update
    sudo apt-get install -y libchromaprint-tools
    ;;
  *)
    >&2 printf 'Install fpcalc from Chromaprint for this operating system.\n'
    exit 1
    ;;
esac

fpcalc -version
