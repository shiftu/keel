#!/bin/sh
# 复现并锁定 P1-07：目录不存在时 ! grep 不得算规则通过。
set -eu
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cd "$tmp"

# 旧写法：路径不存在 → grep 报错 → ! 变成成功
set +e
sh -c '! grep -rlnE "pgx|lib/pq" internal/store' >/dev/null 2>&1
old=$?
set -e
[ "$old" -eq 0 ] || fail "expected old ! grep to succeed (wrongly) when dir missing, got $old"

# 修正写法：路径必须存在，否则 error
check() {
  if [ ! -d internal/store ]; then
    return 2
  fi
  grep -rlnE 'pgx|lib/pq' internal/store >/dev/null 2>&1
  st=$?
  case $st in
    0) return 1 ;;  # 找到禁用导入 → 规则失败
    1) return 0 ;;  # 无匹配 → 规则通过
    *) return 2 ;;  # 用法/IO 错误
  esac
}

set +e
check
new_missing=$?
set -e
[ "$new_missing" -eq 2 ] || fail "missing dir should be error/2, got $new_missing"

mkdir -p internal/store
printf 'package store\n' > internal/store/db.go
set +e
check
new_clean=$?
set -e
[ "$new_clean" -eq 0 ] || fail "clean tree should pass, got $new_clean"

printf 'import "github.com/jackc/pgx/v5"\n' > internal/store/bad.go
set +e
check
new_hit=$?
set -e
[ "$new_hit" -eq 1 ] || fail "pgx import should fail the rule, got $new_hit"

echo "ok shell-grep-negation"
