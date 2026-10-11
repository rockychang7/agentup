# AgentUp

跨平台 CLI 工具，用于统一检测和升级本机安装的 AI Coding Agent CLI 工具。

## 功能特性

- **状态概览**：直接运行 `agentup`，以彩色紧凑列表查看版本、安装来源和更新状态；详细路径按需展开
- **自动识别安装方式**：识别 npm、pnpm、yarn、Homebrew、winget、scoop 等安装方式
- **升级进度**：显示检测、等待、升级、验证阶段及耗时；同一包管理器串行升级，不同来源并行执行
- **指定升级**：单独升级某个工具
- **自身升级**：从 GitHub Releases 下载并校验最新版，直接升级 agentup
- **环境诊断**：检查当前环境是否具备升级能力
- **脚本友好**：提供离线检测、JSON 输出、颜色控制及可靠的失败退出码
- **跨平台**：支持 macOS、Linux 和 Windows

## 支持的平台

| 平台 | 状态 |
|------|------|
| macOS | ✅ 已支持 |
| Windows | ✅ 已支持 |
| Linux | ✅ 已支持 |

## 支持的工具

| 工具 | 二进制名 | npm 包名 | Homebrew | Scoop | Winget | 自动升级 |
|------|---------|----------|----------|-------|--------|---------|
| Codex CLI | `codex` | `@openai/codex` | `codex` (cask) | - | - | ✅ |
| Claude Code CLI | `claude` | `@anthropic-ai/claude-code` | `claude-code` (cask) | - | - | ✅ |
| OpenCode CLI | `opencode` | `opencode-ai` | `opencode` | `opencode` | - | ✅ |
| Agy CLI | `agy` | - | - | - | - | ✅ (self-update) |

## 安装

### 方式一：一键安装脚本（推荐）

#### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/rockychang7/agentup/main/install.ps1 | iex
```

#### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/rockychang7/agentup/main/install.sh | bash
```

安装脚本会自动下载最新版本的预编译二进制文件并配置 PATH，无需安装 Go 环境。

### 方式二：下载预编译二进制

从 [Releases](../../releases) 页面下载对应平台的压缩包，解压后将二进制文件放入 PATH 中。

| 平台 | 文件 |
|------|------|
| Windows | `agentup_<version>_windows_amd64.zip` |
| macOS (Intel) | `agentup_<version>_darwin_amd64.tar.gz` |
| macOS (Apple Silicon) | `agentup_<version>_darwin_arm64.tar.gz` |
| Linux (x86_64) | `agentup_<version>_linux_amd64.tar.gz` |
| Linux (ARM64) | `agentup_<version>_linux_arm64.tar.gz` |

### 方式三：从源码编译

```bash
git clone <repo-url>
cd agentup
go build -o agentup .
```

### 方式四：go install（需要 Go 环境）

```bash
go install agentup@latest
```

## 使用方式

### 查看安装状态

```bash
agentup
agentup list
agentup list --verbose
agentup list --offline
```

不带子命令时显示只读状态概览，不会升级或安装任何工具。普通终端显示颜色：青色标题、绿色成功、黄色提醒、红色失败。默认隐藏完整路径，窄终端自动切换为逐项展示。

先显示本机检测进度和已完成的结果，再查询各安装来源的最新版本。`--offline` 跳过最新版本查询，但仍读取本地版本和安装方式。

示例输出（版本仅为演示）：

```text
AgentUp
dev | Agent status

Agent        Version           Source       Status
-------------------------------------------------------------------
codex        0.12.3 -> 0.13.0   npm          Update available
claude-code  1.0.25             npm          Up to date
opencode     -                  -            Not installed
agy          0.3.5              self-update  Latest unavailable

3 installed / 4 supported | 1 updates | 0 checks failed

Next: agentup upgrade
```

`Latest unavailable` 表示该安装来源没有可用的最新版查询，`Check failed` 表示查询执行失败，`Offline / not checked` 表示未查询。它们都不等同于“已是最新”。`--verbose` 展示完整路径、自动升级能力及查询失败原因。

### 输出选项

| 选项 | 行为 |
|------|------|
| `--color=auto` | 默认，仅在真实终端启用颜色 |
| `--color=always` | 强制颜色，便于支持 ANSI 的展示工具使用 |
| `--color=never` | 关闭颜色 |
| `--verbose`, `-v` | 展开路径、检查详情和升级命令输出 |
| `--offline` | 只读模式，跳过 registry 查询；不允许搭配升级命令 |
| `--json` | 输出结构化 JSON，关闭进度和样式 |

设置 `NO_COLOR` 环境变量（即使值为空）也会关闭颜色，包括 `--color=always`。重定向输出默认不带颜色或光标控制符；非交互模式的检测/升级事件写到 stderr，最终结果写到 stdout。

```bash
agentup list --offline --json
agentup list --json > agents.json
agentup upgrade codex --json > result.json
agentup doctor --verbose
agentup version --json
```

JSON 支持状态概览、`list`、`upgrade`、`update`、`doctor` 和 `version`，不支持 `uninstall`。列表结果包含 `latest_check`，升级结果包含 `outcome`、`duration_ms`、执行命令及日志。

### 一键升级所有已安装工具

```bash
agentup upgrade
```

交互式终端以固定任务行显示 `detecting`、`waiting`、`upgrading`、`verifying` 阶段及耗时，不刷出大量重复日志。同一包管理器的升级及后续验证排队执行，独立的包管理器或自升级工具可并行执行。一个工具失败不会阻止其他工具，最终汇总仍按配置顺序展示。

示例汇总：

```text
Upgrade results

Agent        Version           Time  Result
------------------------------------------------
codex        0.12.3 -> 0.13.0   5s    updated
claude-code  1.0.25             8s    unchanged
opencode     -                  <1s   skipped
agy          0.3.5 -> 0.3.6     3s    updated

2 updated | 1 unchanged | 0 unverified | 0 failed | 1 skipped

claude-code: version unchanged; this does not confirm it is the latest

opencode: not installed
```

升级结果含义：

| `outcome` | 含义 |
|-----------|------|
| `updated` | 命令成功，且检测到有效的新版本；版本降低会判为失败 |
| `unchanged` | 命令成功，但版本未变；不能据此证明已是最新 |
| `unverified` | 命令成功，但无法读取或比较升级前后的版本 |
| `failed` | 升级命令或检测失败 |
| `skipped` | 未安装或不支持自动升级 |

失败或无法验证的结果附带重试命令。使用 `--verbose` 查看执行命令及捕获的 stdout/stderr。单次本地检测命令最多等待 15 秒，包管理器诊断命令最多等待 10 秒，单次升级命令最多等待 10 分钟；Ctrl+C 可取消正在执行的子进程。

退出码 `0` 表示命令完成；`1` 表示参数错误、执行失败、最新版检查失败或升级结果无法验证。未安装、不支持查询、离线检查、版本未变和正常跳过不会单独导致失败。`doctor` 的诊断提醒以结果为准，缺少可选包管理器不会导致失败。

### 指定升级某一个工具

```bash
agentup upgrade codex
agentup upgrade claude-code
agentup upgrade claude          # claude-code 的简写
agentup upgrade opencode
agentup upgrade agy
```

### 环境诊断

```bash
agentup doctor
```

优先显示操作系统、已安装 Agent 数量、可用包管理器和需要处理的问题。未安装的可选包管理器不视为错误。`agentup doctor --verbose` 展开包管理器版本和全部路径；诊断本身不查询最新版。

### 查看版本

```bash
agentup version
```

### 升级 agentup

```bash
agentup update
```

也可以使用与其他工具一致的命令：

```bash
agentup upgrade agentup
```

agentup 会下载当前系统和架构对应的最新 Release，并使用发布的 `checksums.txt` 校验 SHA-256 后替换当前可执行文件。Windows 会在命令退出后完成替换。

`v0.1.0` 尚未内置自身升级能力，需要重新运行一次安装脚本升级到 `v0.2.0`；从 `v0.2.0` 开始可直接使用上述命令升级后续版本。

### 卸载 agentup

#### 方式一：使用内置命令

```bash
agentup uninstall
```

自动删除二进制文件并清理 PATH（Windows）。

#### 方式二：使用卸载脚本

Windows (PowerShell)：

```powershell
irm https://raw.githubusercontent.com/rockychang7/agentup/main/uninstall.ps1 | iex
```

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/rockychang7/agentup/main/uninstall.sh | bash
```

## 升级策略

根据检测到的安装方式自动选择升级命令：

| 安装方式 | 升级命令 |
|---------|---------|
| npm | `npm install -g <package>@latest` |
| pnpm | `pnpm add -g <package>` |
| yarn | `yarn global upgrade <package>` |
| Homebrew (formula) | `brew upgrade <formula>` |
| Homebrew (cask) | `brew upgrade --cask <formula>` |
| winget | `winget upgrade <id>` |
| scoop | `scoop update <package>` |
| binary / unknown | 不自动升级，提示用户手动升级 |

## 项目结构

```
agentup/
├── main.go                          # 程序入口
├── cmd/                             # 命令层
│   ├── root.go                     # 根命令 + 全局初始化
│   ├── list.go                     # agentup list
│   ├── upgrade.go                  # agentup upgrade [tool]
│   ├── update.go                   # agentup 自升级
│   ├── doctor.go                   # agentup doctor
│   ├── version.go                  # agentup version
│   └── uninstall.go                # agentup uninstall
├── internal/
│   ├── model/
│   │   └── model.go                # 核心数据结构 (ToolInfo, UpgradeResult 等)
│   ├── config/
│   │   └── config.go               # 工具配置集中管理 (包名/formula 等)
│   ├── runner/
│   │   └── runner.go               # 命令执行封装 (Runner 接口 + MockRunner)
│   ├── platform/
│   │   ├── platform.go             # 平台接口 + 工厂函数
│   │   ├── darwin.go               # macOS 实现
│   │   ├── windows.go              # Windows 实现
│   │   └── other.go                # Linux 预留实现
│   ├── detector/
│   │   ├── detector.go             # 检测编排逻辑 + 版本解析
│   │   ├── npm.go                  # npm/pnpm/yarn 安装检测
│   │   ├── brew.go                 # Homebrew 安装检测
│   │   ├── winget.go               # winget 安装检测
│   │   ├── scoop.go                # scoop 安装检测
│   │   └── binary.go               # 手动 binary 安装检测 (fallback)
│   ├── upgrader/
│   │   └── upgrader.go             # 升级逻辑 + 命令构建 + 错误分类
│   └── selfupdate/
│       └── selfupdate.go           # agentup Release 下载、校验和自身升级
├── pkg/
│   └── table/
│       ├── table.go                # 统一终端样式与结果展示
│       └── progress.go             # 实时检测与升级进度
├── install.ps1                     # Windows 一键安装脚本
├── install.sh                      # macOS/Linux 一键安装脚本
├── uninstall.ps1                   # Windows 卸载脚本
├── uninstall.sh                    # macOS/Linux 卸载脚本
├── .goreleaser.yml                 # GoReleaser 跨平台编译配置
└── go.mod
```

### 模块说明

| 模块 | 职责 |
|------|------|
| `cmd` | 处理 CLI 命令解析和执行 |
| `internal/model` | 定义核心数据结构 |
| `internal/config` | 集中配置工具的包名、formula 等 |
| `internal/runner` | 封装外部命令执行，提供 MockRunner 用于测试 |
| `internal/platform` | 封装不同操作系统的差异 |
| `internal/detector` | 检测工具是否安装、版本、路径、安装方式 |
| `internal/upgrader` | 根据安装方式执行升级 |
| `pkg/table` | 统一终端样式、宽度适配、检测与升级进度 |

## 开发指南

### 添加新的 Agent CLI 工具

1. 在 `internal/model/model.go` 中添加新的 `ToolName` 常量
2. 在 `internal/model/model.go` 的 `SupportedTools()` 中添加新工具
3. 在 `internal/config/config.go` 的 `DefaultTools()` 中添加工具配置
4. 如果工具使用新的包管理器，在 `internal/detector/` 添加检测逻辑
5. 在 `internal/upgrader/upgrader.go` 的 `buildUpgradeCommand()` 添加升级命令

### 运行测试

```bash
go test ./... -v
```

### 跨平台编译

```bash
# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o agentup-darwin-amd64 .

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o agentup-darwin-arm64 .

# Windows
GOOS=windows GOARCH=amd64 go build -o agentup.exe .

# Linux (amd64)
GOOS=linux GOARCH=amd64 go build -o agentup-linux-amd64 .

# Linux (arm64)
GOOS=linux GOARCH=arm64 go build -o agentup-linux-arm64 .
```

## 已知限制

- **不支持桌面 GUI**：仅提供命令行界面
- **不支持自动定时升级**：需要手动执行
- **不管理 API Key**：不涉及账号登录和 API Key 管理
- **不管理模型配置**：不涉及模型切换和代理配置

## 许可证

MIT
