# 桌面 App：当前支持范围

- 更新日期：2026-10-15。
- 本文只写**已经实现**的能力。要做成什么样、按什么顺序做，见 [桌面 App 检测与升级方案](desktop-upgrade-plan.md)。
- Antigravity 只管 app，不管 IDE，也不用旧版 1.x 的 IDE 仓库升级新 app。

## 1. 怎么用

现有命令默认只管 CLI：

```text
agentup
agentup list
agentup upgrade
```

桌面 App 要单独指定：

```text
agentup list --kind desktop
agentup list --kind desktop --offline --json
agentup list --kind desktop --verbose
agentup list --kind all --offline --json
agentup upgrade codex --kind desktop        # 只预览计划
agentup upgrade codex --kind desktop --yes  # 按计划执行
agentup upgrade --kind desktop --yes        # 批量，只执行可自动升级的项目
```

- 不带 `--yes` 只展示计划，不执行。
- `--yes` 表示接受这次生成的计划；执行前还会重新核对安装、版本、来源和计划。
- “引导更新”的项目只返回官方入口，结果是 `handed_off`（待手动更新），不启动安装器，也不算升级成功。
- 批量只执行能自动升级的项目，其他项目逐个报告跳过原因。
- 同一 App 有多份安装时，用 `--instance <实例 ID>` 指定；实例 ID 从 `list --verbose` 或 JSON 里拿。
- 不支持 `upgrade --kind all`，CLI 和桌面 App 分别确认。
- 离线模式只读，不生成可执行计划。
- 不提供手动输入命令、包名、下载地址或安装路径的入口。

Web 界面：

```text
agentup ui
```

“Agent 概览”按 Agent 分组：每个 Agent 下面一行 CLI（青色 `CLI` 标签）、一行桌面 App（紫色 `App` 标签），可以切换“CLI + App / 只看 CLI / 只看 App”。顶部两张汇总卡分别显示 CLI 和桌面 App 的安装数、更新数，并提供各自的批量升级按钮；确认弹窗里可以取消勾选个别项目。桌面 App 的展开详情里有安装路径、包标识、安装范围、架构、通道、运行状态和升级计划。“升级任务”页把 CLI 任务和桌面 App 任务合在一个列表里。

## 2. 现在每个格子能做什么

升级方式分五级：**自动升级**（AgentUp 执行命令）、**交互安装**（AgentUp 下载校验，你在安装窗口里点完）、**授权命令**（需要 sudo，只在命令行里执行）、**引导更新**（只给官方入口）、**不可用**（显示原因）。

| App | Windows | macOS | Linux |
| --- | --- | --- | --- |
| ChatGPT / Codex | MSIX 按包族识别；WinGet `msstore` 核对；**自动**（`winget upgrade`），需单独确认、不进批量 | Homebrew Cask `chatgpt` 装的：**自动**；直接装的：引导 + 参考版本 | DEB/RPM；APT/DNF 本机缓存查最新版本；**授权命令** |
| Claude | MSIX 按包族、EXE 按发布者识别；引导 | 同上，Cask `claude` | DEB `claude-desktop`；APT 查最新版本；**授权命令** |
| OpenCode | 读主程序架构；v2：官方清单 + **交互安装**；v1：WinGet **自动** | 同上，Cask `opencode-desktop` | DEB/RPM；官方清单查最新版本；引导 |
| Antigravity | 只认 2.x；ProductCode + WinGet 核对；**自动** | 同上，Cask `antigravity`，只认 2.x | 固定目录或 `.desktop` 启动项找解压目录；引导 |

**升级渠道开关**：上表里加粗的升级方式中，默认开启的是 macOS Homebrew Cask 和 Windows WinGet Antigravity（2026-10-15 真机验收通过，2.18.1 → 2.23.1）。其他的 `winget`、`installer`、`apt`、`dnf` 格子在真机验收前默认关闭，关闭时照常显示检测结果和最新版本，但按钮是“去官方更新”，原因写“还没完成真机验收”。验收时用环境变量临时打开：

```text
AGENTUP_DESKTOP_CHANNELS=winget,installer agentup ui
AGENTUP_DESKTOP_CHANNELS=apt agentup upgrade codex --kind desktop --yes
AGENTUP_DESKTOP_CHANNELS=winget/codex.desktop agentup upgrade codex --kind desktop --yes
```

各渠道的关键规则：

- **WinGet**：只用精确 ID + 指定来源 + `--disable-interactivity`，不自动接受协议。必须 ProductCode 或包族对上，且 WinGet 报告的已装版本和本机一致。OpenCode 只在“已装 1.x、可用也是 1.x”时走 WinGet（WinGet 会把 2.x 误对上 1.x 的条目，执行会降级）。结果按 WinGet 官方返回码判断，没有更新时结果是 `no_update`。
- **交互安装（OpenCode Windows v2）**：从 `opencode.ai` 的官方清单按 App 实际架构选安装包，下载后核对大小、SHA-512 和数字签名（发布者必须是 Anomaly Innovations, Inc），然后打开安装窗口，不加静默参数。安装器打开前会写一份“未完成安装回执”，没核对完版本前不会再次打开安装器。
- **授权命令（Linux APT/DNF）**：命令由后端按登记的包名生成，例如 `sudo apt-get install --only-upgrade chatgpt`。命令行里由你自己输入 sudo 密码；Web 界面只提供“复制命令”，接口拒绝执行。
- **参考版本（macOS 直接安装）**：读 formulae.brew.sh 的 Cask 版本，界面写明“Homebrew 参考版本”，不是厂商官方最新版。

Homebrew 的认领规则：

- Cask 必须来自 `homebrew/cask`，且确有安装记录。
- Cask 的安装目标或 Caskroom 里的 App 链接要对得上发现的 App。
- 不接管官网直接装的同名 App，不能只凭“Brew 里有这个包”就认领。
- 比较用 App 实际版本，不只看 Brew 的记录。
- 用 Mach-O 文件头确认架构；读不到或和当前机器不符时转为引导。
- 无法确认 App 已退出时不执行，不自动结束进程。
- Cask 被固定或禁用时不执行。
- OpenCode 和 Antigravity 不自动跨版本线升级。
- 不强制 `--greedy`，不卸载，不迁移配置。

## 3. 检测边界

- Windows 用系统自带的 Windows PowerShell，不依赖 PowerShell 7 的 Appx 模块。
- Windows 只查当前用户的 MSIX；卸载记录查当前用户和系统两处。
- macOS 只查系统和当前用户的 Applications 目录，不扫整个磁盘或外接盘。
- Linux 通过 `/proc/*/exe` 判断 App 是否在运行；读不了进程表就显示“未知”，不执行升级。DEB 和 RPM 都查。
- Linux 的 Antigravity 解压版只认 `~/.local/share/antigravity`、`/opt/Antigravity` 和 `.desktop` 启动项指向的目录；ASAR 打包、AppImage、Flatpak、Snap 和 Arch 安装暂不识别。
- 检测失败会报告问题并显示“检测未完成”，不当成“未安装”。
- “最新版本未知”“没检查”“查询失败”和“已是最新”严格分开。
- 候选版本来自包管理器的本机缓存，可能落后。检测时不会偷偷运行 `brew update`、`apt update` 或系统升级。
- Debian 包版本保留 epoch 和修订号，用 `dpkg --compare-versions` 比较，不截断。
- 界面上写“渠道候选版本”，不把包管理器缓存等同于厂商的全球最新版本。

## 4. 安全与结果

- 浏览器只提交后端生成的计划 ID 和状态版本号。
- 后端重新扫描并重新生成计划，身份、版本、来源、架构、通道或命令变了就拒绝。
- 桌面 App 和 CLI 共用跨进程修改锁；Web 界面同一时间只允许一个修改任务。
- 批量任务串行执行，同一个包管理器不并发。
- 离线、计划无效、计划重复、状态过期、来源不明时都不执行。
- 不关厂商自带更新，不锁版本，不自动提权、强杀、重启或回滚。
- 只从 `opencode.ai` 和 `formulae.brew.sh` 读元数据和下载；安装包必须通过大小、SHA-512 和签名发布者核对才会打开。
- 批量升级只包含“自动升级”且不需要单独确认的项目；Codex 桌面版需要单独确认。
- 包管理器执行时可能选择更新的版本，计划不承诺锁定版本。

| 结果 | 意思 |
| --- | --- |
| `updated` | 命令成功，同一份安装的版本确实变高，且没跨版本线 |
| `no_update` | 渠道明确表示没有可用更新 |
| `unchanged` | 命令成功但版本没变，不能据此说已是最新 |
| `unverified` | 命令成功但无法可靠验证，或检测不完整 |
| `failed` | 执行失败、计划过期、版本降低或跨版本线 |
| `skipped` | 未安装、批量里需要手动处理、条件不满足 |
| `handed_off` | 已给出官方入口，需要用户完成后重新检测 |
| `cancelled` | 开始安装前停止了（比如下载时 App 被重新打开），没有修改任何东西 |

任务和日志只保存在当前 UI 进程里：刷新页面不丢，服务停止后清空。桌面任务最多保留 20 条，每条只留日志末尾一段。

## 5. 怎么验证的

- Go 测试覆盖：安装身份、多份安装、排除 IDE、离线、版本比较（含 RPM 规则）、架构和进程保护、计划变化拒绝、升级后验证、锁和接口安全；WinGet 精确匹配、本地化输出解析和返回码；OpenCode 2.x 不走 WinGet；安装包架构选择、校验和签名、未完成安装回执；APT/DNF 授权命令只在终端执行；渠道开关默认关闭。
- 浏览器测试用纯模拟接口，不运行包管理器或安装器；覆盖 Codex 不进批量、安装包确认信息、授权命令只复制不提交。
- Windows 本机做了只读实测：WinGet 精确对上 Codex（msstore）和 Antigravity（2.18.1，可用 2.23.1），OpenCode 从官方清单读到 2.0.29，主程序架构读出 amd64。Antigravity 用 WinGet 做了真实升级验收（2.18.1 → 2.23.1，结果核对通过），其他格子还没做真实升级。
- macOS / Linux 的真实升级还需要对应真机验收，不能用跨平台编译或模拟测试代替。
