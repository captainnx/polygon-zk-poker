#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
version=1.0.0-beta.18
case "$(uname -m)" in arm64|aarch64) arch=aarch64;; x86_64) arch=x86_64;; *) exit 1;; esac
case "$(uname -s)" in Darwin) platform=apple-darwin;; Linux) platform=unknown-linux-gnu;; *) exit 1;; esac
mkdir -p .tools
curl --proto '=https' --tlsv1.2 -fL "https://github.com/noir-lang/noir/releases/download/v${version}/nargo-${arch}-${platform}.tar.gz" -o .tools/nargo.tar.gz
if [[ "$arch-$platform" == "aarch64-apple-darwin" ]]; then
  echo 'ed67d9f3490a391f9ae2b454a7f6433df5abfe37e0bbdb0e4e28208da1ef9d41  .tools/nargo.tar.gz' | shasum -a 256 -c -
fi
tar -xzf .tools/nargo.tar.gz -C .tools
.tools/nargo --version
