# AgentUp

跨平台本地工具，用于统一检测和升级本机安装的 AI Coding Agent CLI 与 Desktop 应用，提供命令行与本地 Web UI；默认命令仍只管理 CLI。

## 功能特性

- **状态概览**：直接运行 `agentup`，以彩色紧凑列表查看版本、安装来源和更新状态；详细路径按需展开
- **本地 Web UI**：`agentup ui` 打开 Agent 概览、升级任务和环境诊断；完整中英文切换，支持深色与浅色主题
- **Desktop 分级管理**：三平台检测安装实例和最新版本、预览升级计划；按安装来源走 Homebrew Cask、WinGet、官方安装包（下载并校验签名）或 Linux 授权命令，其余给官方入口；Antigravity 仅管理 app，不管理 IDE
- **确认与实时日志**：Web UI 单项或批量升级前展示实际命令；执行中实时推送日志，刷新页面后恢复当前会话任务
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
| OpenCode CLI v1 | `opencode` | `opencode-ai` | `opencode` | `opencode` | - | ✅ |
| OpenCode CLI v2 | `opencode`（回退 `opencode2`） | `@opencode/cli` | `anomalyco/tap/opencode-v2` | - | - | ✅ |
| Agy CLI | `agy` | - | - | - | - | ✅ (self-update) |

OpenCode 仍作为一个工具管理，优先检测 PATH 中的 `opencode`，不存在时才检测 `opencode2`；不会枚举或分别升级所有共存安装。根据本机版本选择 v1/v2 对应的包名、最新版查询和升级命令，普通 `agentup upgrade opencode` 只更新当前版本线，不执行 v1→v2 迁移。

v2 的 pnpm 升级带 `--allow-build=@opencode/cli`，允许官方必需的 postinstall 脚本。官方 `~/.opencode/bin/opencode`（Windows 为 `.exe`）安装支持 `opencode upgrade --method curl` 自升级，最新版通过对应版本线的 npm registry 元数据查询，不要求安装 Node.js。其他独立二进制、无法识别的版本和预发布版本不自动升级。

跨大版本迁移需要另外处理插件、服务端 API 和终端配置，AgentUp 不自动卸载旧包、迁移配置或切换版本线。调研依据、各来源的命令和已知边界见 [OpenCode v1/v2 兼容说明](docs/opencode-versions.md)。

## Desktop 管理

Desktop 与 CLI 分别管理，不共享版本、包名或默认升级目标：

```bash
agentup list --kind desktop
agentup list --kind desktop --offline --json
agentup list --kind all --offline --json
agentup upgrade codex --kind desktop        # 仅预览
agentup upgrade codex --kind desktop --yes  # 确认生成的计划
agentup upgrade --kind desktop --yes        # 仅批量执行可自动升级项
```

支持检测 ChatGPT/Codex Desktop、Claude Desktop、OpenCode Desktop、Antigravity app。**不管理 Antigravity IDE**，不接管旧版 1.x IDE，不切换安装来源或执行产品迁移。

默认对身份、来源、路径、版本线、架构和退出状态均可确认的 macOS Homebrew Cask，以及已真机验收的 Windows WinGet Antigravity 自动执行升级。其他 WinGet 应用、OpenCode 官方安装包、Linux APT/DNF 授权命令已实现，但在真机验收前默认关闭，可用 `AGENTUP_DESKTOP_CHANNELS=winget,installer,apt,dnf` 临时开启；关闭时提供官方入口引导，结果为 `handed_off`，不是升级成功。详见 [当前支持范围](docs/desktop-support.md)。

多份桌面安装需要通过 `--instance <ID>` 明确目标。Desktop 命令不带 `--yes` 只展示计划，离线模式不执行操作。Web UI 按 Agent 分组，同时列出 CLI 与桌面 App，并用不同颜色的类型标签区分；CLI 与桌面 App 分别确认和执行。三平台的完整方案与实施顺序见 [桌面 App 检测与升级方案](docs/desktop-upgrade-plan.md)。

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

需要 Go（版本见 `go.mod`）和 Node.js 22.12+；使用发布包运行时不需要 Node.js。

```bash
git clone <repo-url>
cd agentup
npm ci --prefix web
npm run build --prefix web
go build -o agentup .
```

Windows 使用 `go build -o agentup.exe .`。前端构建产物嵌入 Go 二进制，发布包不需要单独部署网页或安装前端依赖。仅运行 `go build` 仍可使用 CLI，但未生成前端资源时 `agentup ui` 会提示构建步骤。

## 使用方式

### 本地 Web UI

```bash
agentup ui
agentup ui --port 8765
agentup ui --no-open
agentup ui --offline
agentup ui status
agentup ui stop
agentup ui stop --port 8765
agentup ui stop --all
```

默认选择空闲端口并打开浏览器。每次启动都是独立实例，可在多个终端同时运行；`--port` 指定端口，重复占用同一端口会提示选择其他端口。`--no-open` 只输出启动地址；终端保持运行，Ctrl+C 停止该实例。

`agentup ui status` 显示当前用户的运行实例、端口、PID、版本和完整启动地址；`--json` 输出不包含会话凭证。只有一个实例时，`agentup ui stop` 自动停止它；多个实例时需用 `--port` 指定或用 `--all` 全部停止。网页右上角的电源按钮在确认后停止当前实例，其他实例不受影响；同一实例的所有已连接页面会显示“服务已停止”并停止自动重连。关闭浏览器不会停止服务。

停止命令默认拒绝中断正在执行的升级；检测和诊断可以取消。确实需要中断升级时，可执行 `agentup ui stop --port 8765 --force` 或 `agentup ui stop --all --force`。强制停止会取消子进程，但不回滚已发生的包管理器修改。命令等待实例清理完成后返回，随后可运行自身更新或卸载。

- **Agent 概览**：查看版本和安装来源，筛选有更新或已安装项，展开路径与升级命令，选择单项或批量升级
- **升级任务**：先确认命令再执行；显示阶段、耗时、版本验证结果和实时输出，支持复制日志
- **环境诊断**：检查包管理器、Agent 路径和自动升级能力，支持复制诊断报告
- **中英文与主题**：界面语言、日期和业务状态随语言切换，记住语言与深浅主题；第三方命令的原始输出不翻译
- **离线只读**：跳过最新版查询，允许本机检测和诊断；前后端均拒绝升级操作

只监听 `127.0.0.1`，不支持远程访问。请使用终端显示的完整启动地址建立本地会话；启动凭证只在 URL fragment 中传递，不进入 HTTP 请求地址。API 校验会话、Host 和 Origin，升级接口不接受任意命令。打开页面只检测，不会自动升级或提权。

任务记录保存在各自服务进程内，最多保留 20 条及每条任务最近的日志；同一实例刷新或重连不丢失，停止服务后不保留，不同实例不共享任务历史。CLI 与所有 Web UI 实例的升级互斥；AgentUp 自身更新或卸载前需执行 `agentup ui stop --all` 关闭全部实例。若确认后的检测结果或执行命令已变化，升级停止并要求重新确认。

运行实例通过当前用户缓存目录中的记录和操作系统锁登记；进程异常退出后，后续状态或停止命令会清理失效记录，不会仅按 PID 强杀进程。`v0.5.0` 的旧版 UI 没有实例登记与停止接口，首次切换时需在原终端 Ctrl+C 停止；新版管理命令只控制新版启动的实例。

### 查看安装状态

```bash
agentup
agentup list
agentup list --verbose
agentup list --offline
```

不带子命令时显示只读状态概览，不会升级或安装任何工具。普通终端显示颜色：青色标题、绿色成功、黄色提醒、红色失败。默认隐藏完整路径，窄终端自动切换为逐项展示。

先显示本机检测进度和已完成的结果，再查询各安装来源的最新版本。`--offline` 跳过最新版本查询，但仍读取本地版本和安装方式。

Agy 通过官方安装器使用的发布清单查询当前操作系统和架构的最新版本，无需 npm，也不会执行 `agy update`。查询失败会显示 `Check failed`，实际升级仍使用 Agy 自更新命令。

示例输出（版本仅为演示）：

```text
AgentUp
dev | Agent status

Agent        Version           Source       Status
-------------------------------------------------------------------
codex        0.12.3 -> 0.13.0   npm          Update available
claude-code  1.0.25             npm          Up to date
opencode     -                  -            Not installed
agy          0.3.5 -> 0.3.6     self-update  Update available

3 installed / 4 supported | 2 updates | 0 checks failed

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

JSON 支持状态概览、`list`、`upgrade`、`update`、`doctor`、`version`、`ui status` 和 `ui stop`，不支持 `uninstall` 和启动 `ui`。列表结果包含 `latest_check`，升级结果包含 `outcome`、`duration_ms`、执行命令及日志。`ui stop --json` 包含成功停止的端口和各失败实例的错误，部分停止失败时退出码仍为 `1`。

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

agentup 会下载当前系统和架构对应的最新 Release，并使用发布的 `checksums.txt` 校验 SHA-256 后替换当前可执行文件。Windows 也会在命令返回前完成替换，安装失败会尝试恢复原程序并立即返回错误，不再依赖退出后运行的后台脚本。

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
| OpenCode v2 (pnpm) | `pnpm add -g --allow-build=@opencode/cli @opencode/cli@latest` |
| OpenCode v2 (yarn) | `yarn global add @opencode/cli@latest` |
| OpenCode 官方目录自升级 | `<opencode 完整路径> upgrade --method curl` |
| Agy 自升级 | `<agy 完整路径> update` |
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
│   ├── ui.go                       # agentup ui 本地服务与浏览器启动
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
│   ├── diagnostic/                 # CLI / UI 共用环境诊断
│   ├── operation/                  # 跨进程操作锁
│   ├── service/                    # 检测状态、升级任务和有界日志
│   ├── uisession/                  # UI 实例登记、存活检查和认证停止
│   ├── webui/                      # 本地 HTTP / SSE API 与嵌入资源
│   └── selfupdate/
│       └── selfupdate.go           # agentup Release 下载、校验和自身升级
├── pkg/
│   └── table/
│       ├── table.go                # 统一终端样式与结果展示
│       └── progress.go             # 实时检测与升级进度
├── web/                            # React / TypeScript 中英文 Web UI
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
| `internal/diagnostic` | CLI 与 Web UI 共用的环境诊断 |
| `internal/operation` | 防止多个进程同时修改安装 |
| `internal/service` | 管理本地状态、任务与日志，推送状态变化 |
| `internal/uisession` | 登记多实例、清理失效记录，通过认证请求停止服务 |
| `internal/webui` | 受本地会话保护的 HTTP API、SSE 和嵌入页面 |
| `web` | 中英文界面、主题与响应式布局 |
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
npm ci --prefix web
npm run build --prefix web
go test ./... -v
go vet ./...

# 浏览器测试使用模拟工具，不升级本机 Agent
cd web
npx playwright install chromium
npm test
```

Windows 浏览器测试默认使用已安装的 Microsoft Edge；Linux / macOS 需先安装 Playwright Chromium。CI 在三个操作系统运行 Go 测试，在 Linux 验证桌面和移动布局。修改页面后重新构建前端，再启动 `go run . ui` 验证嵌入资源。

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

- **不提供原生桌面安装器或托盘**：图形界面通过本地浏览器打开，不提供公网部署或远程管理
- **任务历史不跨会话保留**：停止 `agentup ui` 后清空；不提供安装、卸载 Agent 或升级回滚
- **不支持自动定时升级**：需要手动执行
- **不管理 API Key**：不涉及账号登录和 API Key 管理
- **不管理模型配置**：不涉及模型切换和代理配置

## 许可证

MIT
