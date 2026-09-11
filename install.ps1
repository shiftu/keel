# keel 安装脚本（Windows PowerShell）：下载单个 exe 到 %LOCALAPPDATA%\keel 并加入用户 PATH。
#   irm https://raw.githubusercontent.com/shiftu/keel/main/install.ps1 | iex
# 可选环境变量：$env:KEEL_VERSION = "v0.1.0"
$ErrorActionPreference = "Stop"
$repo = "shiftu/keel"
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$ver = $env:KEEL_VERSION
if (-not $ver) {
  $ver = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
}
$dir = Join-Path $env:LOCALAPPDATA "keel"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$url = "https://github.com/$repo/releases/download/$ver/keel_windows_$arch.exe"
Write-Host "下载 $url"
Invoke-WebRequest -Uri $url -OutFile (Join-Path $dir "keel.exe")
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
  $env:Path = "$env:Path;$dir"
  Write-Host "已把 $dir 加入用户 PATH（新开的终端生效）"
}
& (Join-Path $dir "keel.exe") version

# 顺手把 PowerShell 补全装上：问不出 $PROFILE 就跳过，不会让安装失败。
Write-Host ""
& (Join-Path $dir "keel.exe") completion --install powershell --quiet

Write-Host ""
Write-Host "下一步：cd <你的仓库>; keel init"
