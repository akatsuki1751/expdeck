#!/usr/bin/env bash
# 冒烟测试：用编好的二进制在一个临时仓库里跑一遍最小闭环。
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
bin="$root/build/exp"

if [[ ! -x "$bin" ]]; then
  echo "找不到二进制：$bin（先跑 source/build.sh）" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cd "$tmp"
git init -q
git -c user.email=a@b.c -c user.name=t commit -q --allow-empty -m init

echo "--- version/help ---"
"$bin" >/dev/null 2>&1 || true

echo "--- init ---"
"$bin" init

echo "--- new ---"
"$bin" new demo/smoke --why "验证二进制可用" | head -12

echo "--- ls ---"
"$bin" ls | head -12

echo "--- note ---"
"$bin" note demo/smoke --text "第一段" | head -6
"$bin" note demo/smoke --append "追加一句" | head -6

echo "--- show ---"
"$bin" show demo/smoke | head -12

echo "--- doctor ---"
"$bin" doctor

echo "--- conclude ---"
"$bin" conclude demo/smoke --decision abandon --why "冒烟测试" | head -6

echo "smoke ok: $tmp"
