#!/bin/sh
# 修订前：|| true 吞掉检查失败。
command -v keel >/dev/null 2>&1 && keel check --quiet || true
