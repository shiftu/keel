---
schema: 1
id: D-d123fcee-1824-40fb-b5f6-15fe91d6ee8f
title: 补全与自更新：spec 一处来源 + 校验和必对
status: accepted
date: "2026-09-10"
by: agent:claude
tags:
  - cli
  - ux
  - release
scope:
  - internal/cli/**
  - internal/selfupdate/**
  - internal/ui/progress.go
  - install.sh
  - install.ps1
  - Makefile
supersedes: []
superseded_by: []
rule_migration: []
confidence: null
review_after: "2026-12-09"
evidence:
  - E-bab35e7a-7b7f-4644-b6ec-ad6e3adf90de
  - E-3f3c16d0-6a4f-4cf4-b8a3-6803812d0d60
---

## 背景

keel 的命令、选项、以及仓库里规则和记忆的 ID，此前只能靠手敲。ID 是 `R-<uuid>` 形状，
人记不住，`keel promote` / `keel verify` 这类必须给 ID 的命令因此很难用。
升级也只能重跑安装脚本，没有「问一句有没有新版」的入口。

## 决定

加两个命令：`keel completion`（bash / zsh / fish / powershell）和 `keel update`。

**补全的候选一律现算，不写进 shell 脚本。** 四家脚本都只是十行胶水，每按一次 Tab 就调一次
`keel __complete`。理由是候选有一半是活的——规则和记忆的 ID 由当前仓库的 `.keel/` 决定，
写死在脚本里会随仓库内容过期，而且过期了不报错，只是补错。路径也由 keel 自己补：
四家 shell 里只有 bash 能在「没给候选」时干净地退回文件名补全。

**说明书（`internal/cli/spec.go`）与真正注册选项的代码是两处，靠测试盯着。**
`spec_test.go` 用 go/ast 读源码里的 `newFlagSet` + `fs.Xxx(...)`，逐命令比对选项名。
不选「把 spec 做成唯一来源、命令从 spec 取 flag 指针」那条路：那要改写全部 16 个命令函数体，
收益只是省掉一个测试。

**自更新必须对校验和。** 下完先比发布时一起传的 `SHA256SUMS`，对不上当场删掉、不安装；
发布里没有 `SHA256SUMS` 就直接拒绝。这是在往用户机器上放一个会被直接执行的文件，
跳过校验的自更新等于给自己开一条后门。资产名 `keel_<goos>_<goarch>` 由 Makefile 的 TARGETS
决定，`internal/selfupdate` 有测试直接读 Makefile 钉住它——对不上的表现是 404，不是「装错了」。

## 备选与理由

- **用 cobra 之类的框架换来免费的补全**：要换掉整个命令层，并且引入一个大依赖。
  keel 现在零运行时依赖、命令层一百多行，不值得。
- **补全脚本里写死候选**：省掉一次进程调用，代价是候选会过期且不报错。补全撒谎比补全慢糟得多。
- **自更新只比版本号不校验哈希**：少一次请求，但把「下到了什么」这件事交给运气。不接受。
- **进度条引 `golang.org/x/term` 判断 TTY**：为一条进度条加一个依赖。改用
  `os.File.Stat()` 的字符设备位，够用。

## 后果与验证方式

- 新增 `internal/selfupdate`（无第三方依赖）与 `internal/ui/progress.go`。
- `install.sh` / `install.ps1` 装完顺手跑 `keel completion --install`，认不出 shell 就跳过，
  不会让安装失败。
- 发版时 `SHA256SUMS` 从「顺手传的」变成「必须传的」，已写进 `docs/release.md`。
- 验证：`go test ./...`。其中
  `TestSpecFlagsMatchSource` 盯住说明书与实现不许对不上，
  `TestUpdateAbortsOnChecksumMismatch` 盯住校验失败时目标文件不被动，
  `TestAssetNameMatchesMakefile` 盯住资产命名与 Makefile 一致，
  `testdata/script/completion.txtar` 盯住 `--install` 反复跑不会越装越多。
