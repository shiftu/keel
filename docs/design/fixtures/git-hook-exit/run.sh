#!/bin/sh
# 复现并锁定 P1-01。失败则非零退出。
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

chmod +x "$root/keel-fail.sh" "$root/hook-old.sh" "$root/hook-new.sh"

# --- keel 已安装且失败 ---
PATH="$root:$PATH"
cp "$root/keel-fail.sh" "$tmp/keel"
chmod +x "$tmp/keel"
PATH="$tmp:$PATH"

set +e
sh "$root/hook-old.sh"
old_fail=$?
sh "$root/hook-new.sh"
new_fail=$?
set -e

[ "$old_fail" -eq 0 ] || fail "old hook should swallow failure, got $old_fail"
[ "$new_fail" -eq 1 ] || fail "new hook should propagate exit 1, got $new_fail"

# --- 未安装 keel：两者都放行 ---
PATH=/usr/bin:/bin
set +e
sh "$root/hook-old.sh"
old_missing=$?
sh "$root/hook-new.sh"
new_missing=$?
set -e

[ "$old_missing" -eq 0 ] || fail "old hook without keel should pass, got $old_missing"
[ "$new_missing" -eq 0 ] || fail "new hook without keel should pass, got $new_missing"

echo "ok git-hook-exit"
