#!/usr/bin/env bash
# 用编好的二进制起一个临时仓库与服务，抓两份页面并把 CSS 内联，
# 产出可以直接双击打开的静态预览。
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
bin="$root/build/exp"
out="/mnt/e/myResearch/Lab/expdeck/preview"
port=8799

mkdir -p "$out"
tmp="$(mktemp -d)"
server=""
cleanup() {
  if [ -n "$server" ]; then kill "$server" 2>/dev/null || true; fi
  rm -rf "$tmp"
}
trap cleanup EXIT

cd "$tmp"
git init -q
git -c user.email=a@b.c -c user.name=t commit -q --allow-empty -m init
"$bin" init > /dev/null
"$bin" new sensitive/lr_1e4 --why "lr 降到 1e-4 能提升 r2" > /dev/null
"$bin" new ablation/sigreg --why "去掉 SIGReg 看消融效果" > /dev/null
"$bin" note sensitive/lr_1e4 --text "第一轮跑完 r2 = 0.8712，比 baseline 低 0.0044。" > /dev/null
"$bin" note sensitive/lr_1e4 --append "第 3 个 epoch 起 train/val gap 从 0.02 涨到 0.09。" > /dev/null
git branch exp/legacy/todo > /dev/null 2>&1 || true

"$bin" serve --addr "127.0.0.1:${port}" --json > serve.json 2>&1 &
server=$!
sleep 1

token="$(sed -n 's/.*"token": *"\([^"]*\)".*/\1/p' serve.json | head -1)"
if [ -z "$token" ]; then
  echo "未能拿到令牌：" >&2
  cat serve.json >&2
  exit 1
fi

cookie="$tmp/cookie.txt"
curl -s -c "$cookie" "http://127.0.0.1:${port}/?token=${token}" -o /dev/null
curl -s -b "$cookie" "http://127.0.0.1:${port}/" -o "$out/index.raw.html"
curl -s -b "$cookie" "http://127.0.0.1:${port}/e/sensitive/lr_1e4" -o "$out/detail.raw.html"
curl -s -b "$cookie" "http://127.0.0.1:${port}/e/sensitive/lr_1e4/edit" -o "$out/edit.raw.html"

for page in index detail edit; do
  python3 - "$out/${page}.raw.html" "$out/${page}.html" "$root/source/internal/web/static/style.css" <<'PY'
import sys
src, dst, css_path = sys.argv[1:4]
html = open(src, encoding="utf-8").read()
css = open(css_path, encoding="utf-8").read()
html = html.replace(
    '<link rel="stylesheet" href="/static/style.css">',
    "<style>\n" + css + "\n</style>",
)
open(dst, "w", encoding="utf-8").write(html)
PY
  rm -f "$out/${page}.raw.html"
done

echo "预览已生成："
ls -l "$out"
