# AgentUp

跨平台 CLI 工具，用于统一检测和升级本机安装的 AI Coding Agent CLI 工具。

## 功能特性

- **一键查看**：列出所有支持的 AI Coding Agent CLI 工具的安装状态、版本、安装方式和路径
- **自动识别安装方式**：识别 npm、pnpm、yarn、Homebrew、winget、scoop 等安装方式
- **一键升级**：自动使用对应的包管理器升级所有已安装的工具
- **指定升级**：单独升级某个工具
- **环境诊断**：检查当前环境是否具备升级能力
- **跨平台**：支持 macOS 和 Windows

## 支持的平台

| 平台 | 状态 |
|------|------|
| macOS | ✅ 已支持 |
| Windows | ✅ 已支持 |
| Linux | ⚠️ 代码结构已预留，未完整测试 |

## 支持的工具

| 工具 | 二进制名 | npm 包名 | Homebrew | Scoop | Winget | 自动升级 |
|------|---------|----------|----------|-------|--------|---------|
| Codex CLI | `codex` | `@openai/codex` | `codex` (cask) | - | - | ✅ |
| Claude Code CLI | `claude` | `@anthropic-ai/claude-code` | `claude-code` (cask) | - | - | ✅ |
| OpenCode CLI | `opencode` | `opencode-ai` | `opencode` | `opencode` | - | ✅ |
| Agy CLI | `agy` | - | - | - | - | ❌ (仅支持手动安装) |

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
| Linux | `agentup_<version>_linux_amd64.tar.gz` |

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
agentup list
```

示例输出：

```text
Name          Installed   Version   Install Method   Path                         Upgrade Supported
----          ---------   -------   --------------   ----                         -----------------
codex         yes         0.12.3    npm              /usr/local/bin/codex         yes
claude-code   yes         1.0.21    npm              /usr/local/bin/claude        yes
opencode      no          -         -                -                            -
agy           yes         0.3.5     binary           /opt/homebrew/bin/agy        no
```

### 一键升级所有已安装工具

```bash
agentup upgrade
```

示例输出：

```text
Upgrading installed agents...

codex         success   0.12.3 -> 0.13.0
claude-code   success   1.0.21 -> 1.0.25
opencode      skipped   not installed
agy           skipped   automatic upgrade not supported, please upgrade manually: https://antigravity.google/docs/cli-overview

Done.
```

### 指定升级某一个工具

```bash
agentup upgrade codex
agentup upgrade claude-code
agentup upgrade opencode
agentup upgrade agy
```

### 环境诊断

```bash
agentup doctor
```

检查当前操作系统的包管理器可用性和工具二进制是否在 PATH 中。

### 查看版本

```bash
agentup version
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
│   ├── doctor.go                   # agentup doctor
│   └── version.go                  # agentup version
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
│   └── upgrader/
│       └── upgrader.go             # 升级逻辑 + 命令构建 + 错误分类
├── pkg/
│   └── table/
│       └── table.go                # 表格输出格式化
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
| `pkg/table` | 表格输出格式化 |

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
# macOS
GOOS=darwin GOARCH=amd64 go build -o agentup-darwin .

# Windows
GOOS=windows GOARCH=amd64 go build -o agentup.exe .

# Linux
GOOS=linux GOARCH=amd64 go build -o agentup-linux .
```

## 已知限制

- **不支持桌面 GUI**：仅提供命令行界面
- **不支持自动定时升级**：需要手动执行
- **不管理 API Key**：不涉及账号登录和 API Key 管理
- **不管理模型配置**：不涉及模型切换和代理配置
- **Linux 支持有限**：代码结构已预留，但未完整测试
- **Agy CLI 不支持自动升级**：Agy 仅支持通过安装脚本安装，无法通过包管理器升级

## 许可证

MIT
