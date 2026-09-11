#!/usr/bin/env bash
# keel 安装脚本（macOS / Linux）：下载对应平台的单个二进制到 ~/.local/bin。
#   curl -fsSL https://raw.githubusercontent.com/shiftu/keel/main/install.sh | bash
# 可选环境变量：KEEL_VERSION=v0.1.0  KEEL_INSTALL_DIR=/usr/local/bin
set -euo pipefail

REPO="shiftu/keel"
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in darwin|linux) ;; *) echo "不支持的系统：$os（Windows 请用 install.ps1）"; exit 1;; esac
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "不支持的架构：$arch"; exit 1 ;;
esac

ver="${KEEL_VERSION:-}"
if [ -z "$ver" ]; then
  ver="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
  [ -n "$ver" ] || { echo "拿不到最新版本号；可以手动指定 KEEL_VERSION=vX.Y.Z"; exit 1; }
fi

dir="${KEEL_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$dir"
url="https://github.com/${REPO}/releases/download/${ver}/keel_${os}_${arch}"
echo "下载 $url"
tmp="$(mktemp)"
curl -fsSL -o "$tmp" "$url"
chmod +x "$tmp"
mv "$tmp" "$dir/keel"
echo "已安装到 $dir/keel"

case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo
     echo "⚠ $dir 不在 PATH 里。加一行到 ~/.zshrc 或 ~/.bashrc："
     echo "    export PATH=\"$dir:\$PATH\""
     echo "  keel 装的 git hook 用 command -v keel 找它；不在 PATH 里，hook 会静默放行。" ;;
esac
echo
"$dir/keel" version

# 顺手把 shell 补全装上。它自己认当前 shell，认不出就跳过。
# 不写成 `… || true`：这个脚本开了 set -e，装不上要说一声，而不是假装没发生。
echo
if ! "$dir/keel" completion --install --quiet; then
  echo "补全没装上（不影响 keel 本身）。想自己装：keel completion"
fi

echo
echo "下一步：cd <你的仓库> && keel init"
