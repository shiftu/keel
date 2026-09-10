#!/bin/sh
# 修订后：keel 已安装则传播其退出码；未安装不阻塞。
if command -v keel >/dev/null 2>&1; then
  keel check --quiet --target index || exit $?
fi
