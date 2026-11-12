#!/usr/bin/env bash
# 把 source/ 编译成单个二进制，输出到仓库根的 build/。
# 用法：bash source/build.sh
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/.." && pwd)"
out="$repo/build"

mkdir -p "$out"

commit="$(git -C "$repo" rev-parse --short HEAD 2>/dev/null || echo unknown)"
echo "ExpDeck build"
echo "  source  $here"
echo "  commit  $commit"
echo "  output  $out/exp"

cd "$here"
CGO_ENABLED=0 go build -trimpath \
  -ldflags="-s -w -X expdeck/internal/cli.version=${commit}" \
  -o "$out/exp" ./cmd/exp

size="$(du -h "$out/exp" | cut -f1)"
sum="$(sha256sum "$out/exp" | cut -d' ' -f1)"
echo "  size    $size"
echo "  sha256  $sum"
echo "done."
