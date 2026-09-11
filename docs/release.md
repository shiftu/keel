# 发版流程

版本号只有一个来源：**git tag**。`Makefile` 用 `git describe --tags --always --dirty` 取值，
通过 `-ldflags -X github.com/shiftu/keel/internal/cli.Version` 打进二进制。
源码里的 `internal/cli.Version = "0.1.0-dev"` 只是没有 tag 时的兜底，发版时不要手改它。

版本号用语义化版本 `vX.Y.Z`。M0–M4 里程碑没走完之前主版本停在 0：
新命令或新字段进 minor，只修 bug 进 patch。破坏 `.keel/` 文件格式要同时提 `schema` 并写迁移说明。

## 发版前

```sh
make check                    # gofmt + go vet + go test ./...
git status --short            # 必须干净
```

`make check` 里的 `fmt` 会改文件，所以先跑它、再看 `git status`。要发版的提交必须已经在 `main` 上。

再核对三件事：

- `README.md` 的状态行和 `docs/usage.md` 的「当前实现到 …」与真实进度一致。
- 这一版改了 `.keel/` 里任何文件格式的话，`docs/design/formats.md` 已经跟着改。
- 改了 hook 或适配器输出的话，`.keel/generated.yaml` 里的 `adapter_schema` 已经跟着提。

`keel update` 依赖发布产物的两件事，改 `Makefile` 的 `TARGETS` 或产物命名时要一起想到：

- **资产名必须是 `keel_<goos>_<goarch>`（Windows 加 `.exe`）**。对不上的表现是用户那边 404，
  不是「装错了」。`internal/selfupdate` 有个测试直接读这个 Makefile 钉住它。
- **`SHA256SUMS` 必须跟二进制一起上传**。`keel update` 下完先对校验和，对不上就删掉不装；
  发布里没有这个文件，它会直接拒绝安装而不是「将就装上」。

## 打 tag 并发布

```sh
# 1. 打 tag 并推
git tag -a v0.1.0 -m "keel v0.1.0：M0 协议 + M1 统一底座"
git push origin main
git push origin v0.1.0

# 2. 交叉编译六个平台 + 校验和（release 目标自己会先跑 vet 和 test）
make release VERSION=v0.1.0

# 3. 建 GitHub Release 并上传产物
gh release create v0.1.0 dist/keel_* dist/SHA256SUMS \
  --title "keel v0.1.0" \
  --notes "……"
```

`make release` 产出 `dist/` 下六个二进制（darwin/linux 各 amd64+arm64，windows amd64+arm64）
和一份 `SHA256SUMS`。文件名格式是 `keel_<os>_<arch>`（Windows 带 `.exe`），
`install.sh` / `install.ps1` 按这个格式拼下载地址，**改名字就等于改安装脚本**。

必须先推 tag 再 `make release`：`git describe` 取的是当前提交上的 tag，
顺序反了会打出 `v0.1.0-1-gxxxx-dirty` 这种版本号。

## 发布后验证

用**从 GitHub 下回来的**产物验证，不是本地 `dist/` 里那份：

```sh
D=$(mktemp -d)
KEEL_INSTALL_DIR="$D" bash <(curl -fsSL https://raw.githubusercontent.com/shiftu/keel/main/install.sh)
"$D/keel" version                              # 版本号要和 tag 完全一致

cd "$(mktemp -d)" && git init -q . && "$D/keel" init      # 冒烟：能建 .keel/、装 hook
```

校验和对一遍：

```sh
cd dist && shasum -a 256 -c SHA256SUMS
```

确认无误后再更新自己机器上的那份：`make install`（装到 `$KEEL_INSTALL_DIR`，默认 `~/.local/bin`）。

## 撤回一个版本

产物有问题就删 release 和 tag，别原地覆盖 —— 已经装过的人不会重新下载：

```sh
gh release delete v0.1.0 --yes
git push origin :refs/tags/v0.1.0
git tag -d v0.1.0
```

修好之后发 `v0.1.1`。
