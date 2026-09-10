#!/bin/sh
# keel 生成的 git hook 必须把 keel check 的退出码原样传出去。
# 见决策 D-aad57f96：`|| true` 会把门禁变成永远放行。
#
# 只看真正会被写进 hook 脚本的那几行：
#   - 排除 .keel/（反例夹具里放的就是坏版本）与 docs/（审查留痕里也有坏版本）
#   - 排除 _test.go 与注释行——解释「为什么不能这么写」时必须能把它写出来
# 选项要写在 -- 之前：-- 之后的 --exclude-dir 会被当成文件名，grep 直接报错退 2。
hits=$(grep -rn \
  --exclude-dir=.keel --exclude-dir=docs --exclude-dir=.git \
  --exclude-dir=dist --exclude-dir=scripts \
  --exclude='*_test.go' \
  -e '|| true' . 2>/dev/null)
status=$?
[ "$status" -gt 1 ] && { echo "grep 出错"; exit 2; }

# 去掉注释行：Go 的 //、shell 的 #，以及 Markdown 引用。
out=$(printf '%s\n' "$hits" | grep -v -E ':[[:space:]]*(//|#|>)' | grep -v '^$')
[ -z "$out" ] && exit 0

echo "$out"
echo "不要用 || true 吞掉退出码；正确写法是 || exit \$?"
exit 1
