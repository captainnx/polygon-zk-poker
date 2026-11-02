#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
version=3.0.0-nightly.20251104
case "$(uname -m)" in arm64|aarch64) arch=arm64;; x86_64) arch=amd64;; *) exit 1;; esac
case "$(uname -s)" in Darwin) platform=darwin;; Linux) platform=linux;; *) exit 1;; esac
mkdir -p .tools
if [[ ! -x .tools/bb ]] || [[ "$(.tools/bb --version)" != "$version" ]]; then
  curl --proto '=https' --tlsv1.2 -fL "https://github.com/AztecProtocol/aztec-packages/releases/download/v${version}/barretenberg-${arch}-${platform}.tar.gz" -o .tools/bb.tar.gz
  if [[ "$arch-$platform" == "arm64-darwin" ]]; then
    echo 'fc30922f8ee2db86a3fb0ddca026f8ab4cf123e6cffbbff17169ede50e513ef4  .tools/bb.tar.gz' | shasum -a 256 -c -
  fi
  tar -xzf .tools/bb.tar.gz -C .tools
fi
.tools/bb --version
