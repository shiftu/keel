---
schema: 1
id: D-febf95c3-800c-4302-a6d9-4aeb7b2c2c7c
title: 用 GitHub Release 发预编译单二进制，安装脚本从 Releases 拉
status: accepted
date: "2026-09-09"
by: agent:claude
tags:
  - build
scope:
  - docs/release.md
supersedes: []
superseded_by: []
rule_migration: []
confidence: null
review_after: "2026-12-08"
evidence: []
---

## 背景

keel 是给别人仓库装底座的工具，装它本身不该先装一套工具链。此前只有 `make build`，
要求使用者有 Go 1.27+ 并自己 clone 源码。同时 keel 装的 git hook 用 `command -v keel`
定位二进制，所以「装到 PATH 上」不是可选项，是 hook 生效的前提。

## 决定

发版产物是 GitHub Release 上的六个预编译静态二进制（darwin/linux 各 amd64+arm64，
windows amd64+arm64）加一份 SHA256SUMS，文件名固定为 `keel_<os>_<arch>`。

`install.sh` / `install.ps1` 按这个文件名拼下载地址，默认装到 `~/.local/bin`
（Windows 是 `%LOCALAPPDATA%\keel` 并写用户 PATH），可用 `KEEL_INSTALL_DIR`
和 `KEEL_VERSION` 覆盖。版本号唯一来源是 git tag，`make release` 经
`-ldflags -X internal/cli.Version` 打进二进制，源码里的默认值只是无 tag 时的兜底。

## 备选与理由

- **只留 `go install`**：不用维护发布流水线，但把 Go 工具链变成使用者的前置条件，
  和「零依赖单二进制」的目标冲突。保留为可选路径，不作为主路径。
- **Homebrew tap / npm 包**：分发体验更好，但要额外维护 formula 或 package 仓库，
  且每个平台一套。当前用户量不值这个维护成本，等有人提再说。
- **打成 tar.gz/zip 归档**：省带宽，但多一步解压，且 `install.sh` 要处理两种压缩格式。
  单文件二进制让安装脚本保持在 30 行以内。

## 后果与验证方式

改二进制文件名等于改安装脚本 —— 两者靠命名约定耦合，`docs/release.md` 已写明这一点。
必须先推 tag 再 `make release`，否则 `git describe` 会打出 `-dirty` 版本号。

验证：发布后用**从 GitHub 下回来的**产物验证，不用本地 dist/ 那份 ——
`KEEL_INSTALL_DIR=$(mktemp -d) bash <(curl -fsSL .../install.sh)`，
确认 `keel version` 与 tag 完全一致，再在一个空 git 仓库里跑通 `keel init`，
最后 `shasum -a 256 -c SHA256SUMS` 对一遍校验和。
