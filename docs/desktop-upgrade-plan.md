# 桌面 App 检测与升级方案

- 更新日期：2026-10-15
- 状态：总方案。阶段 0-3 的代码已完成；阶段 4（真机验收）进行中：Windows + WinGet + Antigravity 已验收并默认开启，其他新升级方式在验收前默认关闭。
- 本文合并了原来的《Desktop 升级管理方案》和《Desktop 可触发更新增量方案》，以后只维护这一份。
- 现在已经能用什么，看 [当前支持范围](desktop-support.md)。本文只讲“要做成什么样、按什么顺序做”。

## 0. 先看结论

目标：4 个桌面 App × 3 个平台，一共 12 个格子。每个格子都要做到两件事：

1. **看得准**：装没装、装在哪、什么版本、有没有新版本。查不到就明说查不到。
2. **升得稳**：按安装来源选升级方式。包管理器能升的自动升；有官方安装包的，下载校验后让用户点安装；剩下的给官方更新入口。

做的顺序：

| 阶段 | 做什么 | 会不会改用户的电脑 | 粗估 |
| --- | --- | --- | --- |
| 0 | 合并文档、重做 Web 界面、数据模型预留新字段 | 不会 | 2-3 人日 |
| 1 | 三个平台的检测补全：版本、架构、运行状态、最新版本从哪查 | 不会，只读 | 3-5 人日 |
| 2 | 包管理器升级：macOS Homebrew、Windows WinGet、Linux APT/DNF | 会，需用户确认 | 4-6 人日 |
| 3 | 官方安装包升级：先做 OpenCode Windows，带下载校验和安装回执 | 会，需用户确认并操作安装窗口 | 4-6 人日 |
| 4 | 真机验收，逐个格子打开开关 | 只在测试机上 | 2-3 人日 + 真机时间 |

合计约 15-23 人日，不含等外部条件（厂商接口、测试机）的时间。每个阶段结束后重新估一次。

为什么这么排：

- 先把界面理顺（阶段 0）。后面新增的“等待退出”“下载中”“待验证”这些状态，要有地方显示。
- 先只读（阶段 1）。检测出错的代价最低，而且后面所有升级都依赖检测结果准不准。
- 包管理器（阶段 2）比安装包（阶段 3）风险低。下载、校验、安装都由包管理器负责，AgentUp 只发命令、核对结果。
- 每个格子单独开关（阶段 4）。没验收过的格子一律保持“引导更新”。

### 0.1 当前进度（2026-10-15）

| 阶段 | 状态 | 说明 |
| --- | --- | --- |
| 0 | ✅ 完成 | 文档合并；界面改版；数据模型加了 `interactive`、`privileged`、`no_update`、`cancelled`、版本来源、需单独确认、安装包信息 |
| 1 | ✅ 代码完成 | 三个平台的检测补全（见各阶段的 ✅ 标记）。Windows 本机只读实测通过；macOS、Linux 只有模拟测试 |
| 2 | ✅ 代码完成 | WinGet（Codex、Antigravity、OpenCode 1.x）、APT/DNF 授权命令。默认关闭，等真机验收 |
| 3 | ✅ 代码完成 | OpenCode Windows v2 官方安装包流程。默认关闭，等真机验收 |
| 4 | 进行中 | 已验收：Windows WinGet Antigravity（见附录 C）。其余格子要在真机上做真实升级，通过后加入默认开启名单 |

**升级渠道开关**：开关按“渠道”或“渠道/产品”设置。默认开启的只有 Homebrew Cask 和已验收的 `winget/antigravity.desktop`；其他的 `winget`、`apt`、`dnf`、`installer` 格子默认关闭。关闭时这些格子仍然显示检测结果和最新版本，但升级方式是“引导更新”，原因写“还没完成真机验收”。验收时用环境变量临时打开，例如：

```text
AGENTUP_DESKTOP_CHANNELS=winget,installer agentup ui
AGENTUP_DESKTOP_CHANNELS=apt agentup upgrade codex --kind desktop --yes
AGENTUP_DESKTOP_CHANNELS=winget/codex.desktop agentup upgrade codex --kind desktop --yes
```

某个格子在真机上验收通过后，把它加进代码里的默认开启名单（`internal/desktop/manager.go` 的 `verifiedChannels`）。

## 1. 管理范围

### 1.1 四个 App

| AgentUp 里的 Agent | 要管的桌面 App | 同名 CLI | 备注 |
| --- | --- | --- | --- |
| Codex | ChatGPT 桌面版 | `codex` | 2026-07-09 起 Codex 桌面体验并入 ChatGPT 桌面版 [S1]。升级认的是 ChatGPT 桌面包，不能用 CLI 的包名或版本 |
| Claude Code | Claude 桌面版 | `claude` | Claude Code 集成在 Claude 桌面版里 [S5] |
| OpenCode | OpenCode Desktop | `opencode` | 有 v1、v2 两条版本线，不自动跨版本线升级 |
| Agy | Antigravity app | `agy` | 只管 app。**Antigravity IDE 和 1.x 旧版不管**，只用于排除识别 |

### 1.2 三个平台

- Windows 10/11：x64、arm64。
- macOS 13 及以上：Apple Silicon、Intel（以各 App 实际支持为准）。
- Linux：Debian/Ubuntu 系（DEB/APT）、Fedora/RHEL 系（RPM/DNF），x64、arm64。Arch、Flatpak、Snap、AppImage 不在本轮。

### 1.3 不做的事

- 跨机器远程管理、后台定时升级、无人值守升级。
- 改安装来源（比如把官网装的 App 改成 Homebrew 管）、切换版本线或产品线。
- 关掉厂商自带的自动更新、锁版本、自动回滚。
- 强制结束 App 进程、自动重启 App 或系统。
- 管理账号、API Key、模型、代理等 App 内配置。
- 管理 Antigravity IDE。

CLI 已经在三个平台上支持（npm、Homebrew、Scoop、工具自带升级），本方案不改 CLI 的任何默认行为：不带 `--kind` 的命令仍然只管 CLI。

## 2. 五条基本规则

1. **CLI 和桌面 App 分开管**。分开检测、分开查版本、分开升级，一次任务里不混着执行。
2. **看安装来源，不看产品名**。同一个 App，Homebrew 装的和官网装的升级方式不一样。认不清来源就不自动升级。
3. **先核对，再动手**。用户确认的是后端生成的“升级计划”。真正执行前重新扫描一次，安装位置、版本、来源、架构有任何变化，旧计划作废，请用户重新确认。
4. **不强杀，不偷偷提权**。App 在运行就等用户自己退出；需要管理员权限就明说，让用户自己授权。
5. **没把握就说没把握**。“查不到最新版本”不等于“已是最新”；“安装器打开了”“命令返回成功”也不等于“升级成功”，必须重新读到更高的版本号才算。

## 3. 升级方式分五级

| 等级 | 代号 | 按钮 | 什么时候用 | 怎么算完成 |
| --- | --- | --- | --- | --- |
| 自动升级 | `automatic` | 升级 | 已验证的包管理器能精确对上这份安装 | AgentUp 执行命令后重新读版本，确认变高 |
| 交互安装 | `interactive` | 下载并安装 | 能拿到官方安装包并校验，但安装要用户在窗口里点 | AgentUp 下载并校验；用户装完后 AgentUp 核对版本，核对前显示“待验证” |
| 授权命令 | `privileged` | 复制命令（命令行模式：在终端执行） | 需要 sudo 或管理员权限，例如 Linux APT/DNF | 命令行模式由用户在当前终端输入密码执行；Web 界面只给命令，用户执行后点“重新检测” |
| 引导更新 | `guided` | 去官方更新 | 来源认不清，或没有可靠的外部升级方式 | 只打开官方入口，结果记为“待手动更新”，不算成功 |
| 不可用 | `blocked` | 不显示按钮，只显示原因 | 未安装、App 正在运行、包管理器锁定等 | 不做任何修改 |

现在代码里只有 `automatic`、`guided`、`blocked`。`interactive` 和 `privileged` 在阶段 0 加进数据模型，到阶段 2、3 对应格子验收后才真正启用。

批量升级只包含“自动升级”的项目。其他等级逐个处理，不能在批量里悄悄跳过后宣称全部成功。

## 4. 全景表：12 个格子

图例：✅ 已实现　🟡 部分实现　⬜ 未做。“阶段”指这个格子的目标升级方式在哪个阶段开放。

### 4.1 Windows

图例更新为 2026-10-15 改造后的状态；“已实现”的升级方式在真机验收前默认关闭（见 0.1 节）。

| App | 常见安装来源 | 检测 | 最新版本 | 升级方式 | 阶段 |
| --- | --- | --- | --- | --- | --- |
| ChatGPT / Codex | 微软商店 MSIX，包族 `OpenAI.Codex_2p2nqsd0c76g0` | ✅ 按包族精确识别，WinGet 交叉核对 | ✅ msstore 源不报可用版本，显示“未知” | ✅ **自动**：`winget upgrade --id 9PLM9XGG6VKS --exact --source msstore`；需单独确认，不进批量 | 2 ✅ / 4 待验收 |
| Claude | MSIX（包族 `Claude_pzs8sxrjxfjjc`），或用户级 EXE 安装器 | ✅ MSIX 按包族精确识别；EXE 按名称和发布者 | ⬜ 待验证（本机没装，缺样本） | 引导（App 自带更新）；WinGet 等拿到样本后再定 | 待样本 |
| OpenCode v2 | 官网 NSIS 安装器，用户级 | ✅ 读主程序文件头得到架构 | ✅ 官方更新清单 | ✅ **交互安装**：下载、校验 SHA-512 和签名（Anomaly Innovations, Inc）后打开安装器 | 3 ✅ / 4 待验收 |
| OpenCode v1 | WinGet `SST.OpenCodeDesktop`（NSIS，用户级，只收录 1.x） | ✅ 同上 | ✅ WinGet，只认 1.x→1.x | ✅ **自动**（WinGet） | 2 ✅ / 4 待验收 |
| Antigravity | NSIS 安装器，用户级；WinGet `Google.Antigravity`，ProductCode `121a0be4-63bd-531e-acf8-fc3924c7e984` | ✅ 只认 2.x，排除 IDE 和 1.x；ProductCode 精确核对 | ✅ WinGet | ✅ **自动**（ProductCode 和版本都对上才执行） | ✅ 已验收，默认开启 |

补充说明：

- **Antigravity 改走 WinGet**。原方案打算读 App 自带的更新配置，但直接请求返回 404，原因不明。WinGet 社区源已收录 `Google.Antigravity` 到 2.23.1（2026-10-14 发布），有 x64/arm64 安装包和 SHA256，还写明了 ProductCode，能和注册表卸载记录精确对上，这条路更稳。代价是信任依据变成“WinGet 社区清单审核 + 安装包 SHA256”，不是厂商签名（本机主程序签名检查结果是 NotSigned）。这一点需要拍板，见第 10 节。
- **Claude 先不定**。WinGet 社区源里 `Anthropic.Claude` 最新是 2.19675.1（10-05 发布），Homebrew 已经是 2.42304.0，差距很大。要先确认是 WinGet 落后，还是两边发布线不同。WinGet 落后时执行升级等于什么都没做，所以阶段 1 先查清楚。
- **OpenCode v2 不走 WinGet**。WinGet 里的 `SST.OpenCodeDesktop` 只有 1.x，没有 v2。v2 只能用官方更新清单 + 安装包。
- WinGet 能“列出”一个 App，只说明能对上，不代表升级一定成功。退出码的含义要在真机上收集后才能开放。
- WinGet 里的 `OpenAI.Codex` 是 Codex CLI，不是桌面版，不能混用。

### 4.2 macOS

四个 App 都有 Homebrew Cask（Homebrew 用来装图形 App 的包）。

| App | Cask 名 | 检测（现在） | 最新版本 | Homebrew 装的 | 官网直接装的 | 阶段 |
| --- | --- | --- | --- | --- | --- | --- |
| ChatGPT / Codex | `chatgpt` | ✅ 应用目录 + Bundle ID + 实际版本 + 架构 + 运行状态 | ✅ Homebrew 装的读 Cask 版本；✅ 直接装的显示“参考版本” | ✅ 自动（默认开启的唯一渠道，待真机验收） | 引导（App 自带更新） | 1 ✅ / 4 待验收 |
| Claude | `claude` | ✅ 同上 | 同上 | 同上 | 同上 | 1 ✅ / 4 待验收 |
| OpenCode | `opencode-desktop` | ✅ 同上 | 同上 | 同上 | 同上 | 1 ✅ / 4 待验收 |
| Antigravity | `antigravity` | ✅ 同上，只认 2.x | 同上 | 同上 | 同上 | 1 ✅ / 4 待验收 |

补充说明：

- 四个 Cask 都标了 `auto_updates`，意思是 App 会自己更新。这带来三件事：
  1. `brew outdated` 默认不列出它们。AgentUp 不依赖它，而是自己比较“App 实际版本”和“Cask 版本”。
  2. App 自己更新后，Homebrew 的安装记录会落后。只有 App 实际版本低于 Cask 版本时才生成升级计划，避免拿旧包覆盖新版本 [S10]。
  3. 指名升级 `brew upgrade --cask <名字>` 时，Homebrew 会处理这类 Cask，但要在真机上确认实际行为。不强制加 `--greedy`。
- 认领规则保持现状：Cask 必须来自 `homebrew/cask`、确有安装记录、安装目标和发现的 App 对得上。官网直接装的同名 App 不会被当成 Homebrew 管理。
- 直接装的 App 在阶段 1 加“参考版本”：读 formulae.brew.sh 上的 Cask 数据（只读），界面写明“参考版本（来自 Homebrew）”。不当成厂商官方最新版，也不据此显示“已是最新”。
- 架构用 Mach-O 文件头读取；读不到或和当前机器不符，就转为引导。App 在运行或无法确认运行状态时不执行。

### 4.3 Linux

| App | 官方分发方式 | 检测 | 最新版本 | 升级方式 | 阶段 |
| --- | --- | --- | --- | --- | --- |
| ChatGPT / Codex | DEB/RPM，官方 APT/DNF 仓库（Preview），另有 Arch 方式 [S2] | ✅ DEB 和 RPM 都查；✅ 运行状态 | ✅ APT 候选；✅ DNF 本地缓存 | ✅ **授权命令** | 2 ✅ / 4 待验收 |
| Claude | DEB + 官方 APT 仓库（Beta，Ubuntu/Debian）[S6] | ✅ DEB `claude-desktop`；✅ 运行状态 | ✅ APT 候选 | ✅ **授权命令** | 2 ✅ / 4 待验收 |
| OpenCode | DEB/RPM 安装包 [S7] | ✅ DEB 和 RPM `opencode-desktop` | ✅ 官方更新清单（`latest-linux.yml` / `latest-linux-arm64.yml`） | 引导；“下载校验 + 授权安装”以后再评估 | 1 ✅ |
| Antigravity | 只有 tar.gz 压缩包（x64/arm64）[S13] | ✅ 固定目录 + `.desktop` 启动项找解压目录；✅ 运行状态 | ⬜ 没有可靠来源 | 引导（保持） | 1 ✅ |

授权命令只针对已登记的包名，由后端生成，例如：

```text
sudo apt-get install --only-upgrade chatgpt
sudo dnf upgrade chatgpt
```

APT 候选版本来自本机缓存。如果要先刷新，命令里明确写出 `sudo apt-get update`，由用户决定是否执行；AgentUp 检测时不会偷偷刷新。

补充说明：

- 运行状态：读 `/proc/*/exe`，和包里的文件（或解压目录）比较；读不了进程表就显示“未知”并禁止执行。
- DEB 和 RPM 两种包数据库都查。
- 不自动申请 sudo，不做完整系统升级（比如 Arch 的 `pacman -Syu`）。
- Antigravity 只有压缩包：阶段 1 改为从 `.desktop` 启动项（`~/.local/share/applications`、`/usr/share/applications`）找解压目录，仍要求 `resources/app/package.json` 的名称是 `antigravity`、版本 ≥ 2。升级保持引导；“AgentUp 自己托管目录并整体替换”不在本轮。
- 没有图形界面的机器只报告检测结果，不靠启动 App 来查版本。

## 5. 实施路线

### 阶段 0：整理基础，重做界面（✅ 完成）

1. ✅ 文档：合并为本文 + [当前支持范围](desktop-support.md)。
2. ✅ Web 界面改版，设计见 8.2 节。
3. ✅ 数据模型预留：
   - 升级等级加 `interactive`、`privileged`。
   - 计划动作在 `command`、`guide` 之外加 `installer`（交互安装）和 `privileged_command`（授权命令）。
   - 结果加 `no_update`（渠道明确说没有更新）和 `cancelled`（开始安装前取消）。
   - 安装记录加“最新版本来源”字段，例如 `winget`、`brew-cask`、`apt`、`vendor-manifest`、`reference`，界面据此写明版本从哪来。

完成标准：现有 Go 测试和浏览器测试全部通过；没有新增任何会修改系统的能力；界面上 CLI 和桌面 App 一眼能区分。

### 阶段 1：三平台检测补全（只读，✅ 代码完成）

Windows：

- ✅ Claude MSIX 按包族 `Claude_pzs8sxrjxfjjc` 精确识别；EXE 安装按发布者 + 安装路径识别。
- ✅ 安装器装的 App（OpenCode、Antigravity、Claude EXE）读主程序文件头，得到 x64/arm64 架构。
- ✅ 用 WinGet 只读交叉核对（`winget list --id <ID> --exact --source <源>`）：Codex 按包族、Antigravity 按 ProductCode、OpenCode v1 按卸载记录。对不上的不进入阶段 2。WinGet 报告的已装版本也必须和本机检测到的一致。
- ✅ 最新版本：Codex、Antigravity、OpenCode v1 从 WinGet 读；OpenCode v2 从官方更新清单读（限定域名、大小、超时）。
- ⬜ 查清 Claude：WinGet 是否落后；MSIX 和 EXE 两种装法各自怎么更新（本机没装，缺样本）。
- ✅ WinGet 的文字输出会随系统语言变化，只按精确 ID 找到那一行，读后面的版本号；结果只看退出码。

macOS：

- ✅ 直接装的 App 显示 Homebrew Cask 参考版本，离线时不查。

Linux：

- ✅ 运行状态检测。
- ✅ DEB 和 RPM 都查；DNF 候选版本只读本地缓存（`--cacheonly`），不隐式刷新。
- ✅ Antigravity 通过 `.desktop` 启动项找解压目录。
- ✅ OpenCode 在 Linux 上有官方更新清单，已接入最新版本查询。

完成标准：12 个格子都能显示“装没装、当前版本、最新或参考版本、查不到的原因”；离线模式不发任何网络请求；检测失败显示“检测未完成”，不当成“未安装”。

### 阶段 2：包管理器升级（✅ 代码完成，默认关闭等验收）

按这个顺序开放：

1. **macOS Homebrew**：代码已有。补四个 Cask 的真机验收：App 运行中、App 已自己更新、架构不符、Cask 被固定。通过后正式标为可用。
2. **Windows WinGet**：Codex → Antigravity → OpenCode v1。Claude 看阶段 1 的结论。
3. **Linux APT/DNF**：ChatGPT、Claude 的授权命令。

WinGet 执行规则：

- 只用精确 ID + 指定来源。不用 `--all`，不按名字模糊匹配。
- 加 `--disable-interactivity`；不自动接受新来源或许可协议。首次使用 msstore 源需要同意协议时，提示用户先自己运行一次 winget。
- 执行前要求用户完全退出 App。App 在运行时不生成计划，用户退出后刷新再升级，所以等待期间不占用全局修改锁；执行时加锁并重新核对计划。
- 退出码按 WinGet 官方返回码表映射（真机上再确认）：明确没有更新 → `no_update`；权限不足、网络失败、App 被占用、安装失败分别说明。
- 执行后重新读同一份安装的版本。MSIX 的安装路径里带版本号，所以按包族认实例，不按路径。
- Codex 桌面版可能就是用户正在用的会话，要单独提醒，不放进批量，不强杀。升级任务由独立的 AgentUp 进程执行，不依赖 Codex 会话还在。

Linux 授权命令规则：

- 命令只由后端按已登记的包名生成。界面和 HTTP 接口都不接受用户输入的包名或命令。
- 命令行模式：`agentup upgrade codex --kind desktop --yes` 在当前终端里执行 sudo 命令，密码由用户自己输入，AgentUp 不保存、不转发。
- Web 界面：只显示命令和“复制”按钮。用户在终端执行后点“重新检测”。Web 服务始终以普通用户身份运行。

完成标准：每个开放的格子都有真机验收记录（见第 9 节）；没验收的格子保持引导。

### 阶段 3：官方安装包升级（✅ 代码完成，默认关闭等验收）

先做 Windows OpenCode v2；Linux OpenCode 只做评估。

```text
读取官方更新清单
  -> 按 App 实际架构选安装包（不默认取第一个）
  -> 用户确认计划
  -> 下载（限定域名和跳转、大小、超时，可以取消）
  -> 校验大小、SHA-512、数字签名的发布者
  -> 等用户完全退出 OpenCode
  -> 加锁，再次核对计划
  -> 打开安装器（交互模式，不猜静默参数）
  -> 等安装结束，重新读同一份安装的版本
  -> 报告结果；拿不到证据就标“待验证”，可以手动“重新核对”
```

要点：

- SHA-512 只证明“下载的和清单写的一样”，不证明发布者可信。签名发布者按预先登记的规则核对，并允许正常的证书更换。
- 安装器可能再拉起子进程，父进程退出不代表装完。
- 安装开始后不提供“强制取消”。
- 写一份最小的“未完成安装回执”：任务 ID、产品、安装身份、前后版本、安装包摘要、当前阶段。UI 重启后据此重新核对，**不会**重复启动安装器。回执不存任何凭证。
- 需要 UAC 时由用户确认；Web 服务不以管理员身份常驻。
- 下载阶段发现候选版本变了，要重新确认，不悄悄换成另一个版本安装。

完成标准：哈希、签名、来源校验失败一律拒绝；安装器提前退出、超时、UI 重启都显示“待验证”；同版本不重装。

### 阶段 4：真机验收，逐格开放

- 按“App × 平台 × 安装来源 × 架构”登记验收表。每个格子单独一个能力开关。
- 至少覆盖：Windows 11 x64、macOS Apple Silicon、Ubuntu 24.04、Fedora 当前版。Windows arm64、macOS Intel、Ubuntu 22.04 视测试机情况补充。
- 在用户自己电脑上做真实升级（尤其可能中断 Codex 会话的）要单独确认后再做。

### 本轮不做，以后再评估

- Antigravity Linux 压缩包版的自动替换。
- 把 macOS 官网直装的 App 改由 Homebrew 管理。
- 安装器静默参数（`/S`、`--silent` 等）。
- Arch、Flatpak、Snap、AppImage。
- 完整的持久化任务历史（阶段 3 只做最小回执）。

## 6. 升级流程、状态和结果

### 6.1 流程

```text
发现安装 -> 读实际版本和来源 -> 查最新版本 -> 判断升级等级
  -> 生成计划 -> 用户确认 -> 重新扫描并核对计划
  -> 等用户退出 App / 等授权 -> 执行，或交给用户
  -> 重新读同一份安装的版本 -> 报告结果
```

### 6.2 计划里要有什么

- 目标安装（实例 ID）、当前版本、目标版本、平台、架构、来源、通道。
- 动作：`command`（包管理器命令）、`installer`（下载安装包）、`privileged_command`（授权命令）、`guide`（官方入口）。
- 动作对应的内容：命令；或安装包地址、大小、摘要和签名规则；或官方入口地址。
- 要不要退出 App、要不要授权、可能的影响。
- 计划 ID：对以上内容做摘要。确认和执行都绑定这个 ID。

包管理器执行时可能装上比预览时更新的版本，确认页要写明“不保证锁定到预览版本”。

### 6.3 任务阶段（阶段 2、3 启用）

| 阶段 | 用户看到的 | 能不能取消 |
| --- | --- | --- |
| `checking` | 正在查询 | 能 |
| `waiting_for_exit` | 请完全退出 App | 能，不会强杀 |
| `needs_elevation` | 等待授权 | 开始安装前能 |
| `downloading` | 正在下载 | 能，并清理临时文件 |
| `verifying_package` | 正在校验安装包 | 能 |
| `installing` | 正在安装 | 不能保证停止 |
| `pending_verification` | 等待确认安装结果 | 可以停止等待，但不代表安装停了 |
| `restart_required` | 需要重启 App 或系统 | 不会自动重启 |
| `verifying` | 正在核对版本 | 可以停止等待 |

### 6.4 结果

| 结果 | 意思 |
| --- | --- |
| `updated` | 同一份安装的版本确实变高了，且没跨版本线 |
| `no_update` | 渠道明确表示没有可用更新（新增） |
| `unchanged` | 命令成功但版本没变，不能据此说已是最新 |
| `unverified` | 缺少证据，无法确认成功 |
| `failed` | 执行失败、校验失败、计划过期、版本降低或跨版本线 |
| `handed_off` | 已打开官方入口或给出命令，需要用户完成后重新检测 |
| `skipped` | 未安装、批量里需要手动处理、条件不满足 |
| `cancelled` | 开始安装前用户取消（新增） |

新增的结果和阶段不改变现有 CLI 的默认目标和旧结果的含义。

## 7. 安全边界

可信来源：

- 只用官方或明确认可的分发来源：厂商下载站、微软商店、Homebrew 官方 Cask、厂商 APT/DNF 仓库；WinGet 社区源待拍板。
- App 本地更新配置里的地址只当线索，不能直接拿来下载执行。
- 网络请求都有超时、大小上限和取消。

接口安全：

- 浏览器只提交“计划 ID + 状态版本号”。后端重新生成计划并核对，不接受任何命令、包名、下载地址或路径。
- 只监听本机地址，带会话校验。

保护用户工作：

- 不强杀 App，不删数据，不改配置，不自动重启。
- 不假设能判断 App 里有没有任务在跑，一律要求用户自己退出。

并发：

- CLI 和 Web 界面共用跨进程修改锁，同一时间只有一个修改任务。
- 同一个包管理器串行执行。
- AgentUp 管不了厂商自带的更新器，所以执行前后都重新核对。发现 App 已被自带更新器升级，就放弃旧计划。

和厂商自动更新的关系：AgentUp 只做“按需补充升级”，不关闭厂商的自动更新。OpenAI 官方也说明，关闭内置更新不能阻止商店或设备管理工具更新 [S4]。以后如果要做“集中接管更新”，要作为单独功能设计：明确授权、只用厂商支持的管理策略、验证策略生效、承担失败处理和审计。

## 8. 命令行和 Web 界面

### 8.1 命令行

现有命令不变：

```text
agentup list --kind desktop                # 只看桌面 App
agentup list --kind all                    # CLI 和桌面 App 一起看
agentup upgrade codex --kind desktop       # 只预览计划
agentup upgrade codex --kind desktop --yes # 按计划执行
agentup upgrade --kind desktop --yes       # 批量，只执行“自动升级”的项目
```

- 不带 `--kind` 只管 CLI。
- 不带 `--yes` 只预览；带 `--yes` 也不能跳过退出 App、授权和校验。
- 同一 App 有多份安装时，用 `--instance <实例 ID>` 指定。
- 不支持 `upgrade --kind all`，CLI 和桌面 App 分别确认。
- 离线模式只读。
- 阶段 2 起：Linux 授权命令在终端里由用户输入 sudo 密码执行。阶段 3 起：交互安装在终端里显示下载和校验进度，并等待用户完成安装窗口。

### 8.2 Web 界面改版（阶段 0）

旧界面的问题：

- 顶部“CLI / Desktop / 全部”和下面的“全部 / 有更新 / 已安装”长得一样，两排同样的下划线标签，“全部”出现两次、意思不同。
- “全部”视图里，顶部统计只算 CLI；桌面部分又有自己的一套筛选和“选择可用更新”，重复了。
- 同一个 Agent 的 CLI 和桌面 App 分在两张表里，离得很远。
- 桌面行的来源显示 `app-bundle` 这类内部代码；状态列叠着两个没有样式的标签；“去官方更新”只有一个外链小图标，看不出是干什么的。

新布局：

```text
Agent 概览                                         [离线只读] [刷新状态]
┌ >_ 命令行 CLI ─────────────────┐ ┌ ▭ 桌面 App ─────────────────┐
│ 已安装 3/4 · 有更新 2           │ │ 已安装 3/4 · 有更新 1         │
│ [升级全部 CLI 更新 (2)]         │ │ [升级可自动升级的 App (1)]    │
└────────────────────────────────┘ └──────────────────────────────┘
显示：[CLI + App] [只看 CLI] [只看 App]      状态：全部 · 有更新 · 已安装
┌ Codex ─────────────────────────────────────────────────────────┐
│ >_ CLI  codex                1.2.0 → 1.3.0  npm      有新版本  [升级] ⌄ │
│ ▭ App   ChatGPT / Codex 桌面版 26.1007.9446  MSIX     版本未知  [去官方更新] ⌄ │
└────────────────────────────────────────────────────────────────┘
┌ Claude Code … ┐
```

- **按 Agent 分组**：每组里 CLI 一行、桌面 App 一行（多份安装就多行）。同一个 Agent 的两种形态放在一起看。
- **类型有固定颜色**：CLI 用青色 `>_ CLI`，桌面 App 用紫色 `▭ App`。顶部两张汇总卡也用同样的颜色。
- **两层筛选长得不一样**：类型用实心分段按钮，状态用小号文字标签，不再出现两个“全部”。
- **按钮带文字**：CLI 显示“升级”；桌面 App 按等级显示“升级”“下载并安装”“复制命令”“去官方更新”；不能操作时，状态下面写原因（比如“请先退出 App”）。
- **来源说人话**：npm、Homebrew、MSIX、安装程序、直接安装、APT、RPM、解压版；内部代码（如 `app-bundle`）只在展开详情里显示。
- **批量升级放到汇总卡上**：CLI 和 App 各一个按钮。确认弹窗里可以取消勾选个别项目。行上不再放复选框。
- **任务页合并**：CLI 任务和桌面 App 任务在同一个列表里，按时间排，每条带 CLI / App 标签。
- 后续阶段新增的状态（等待退出、下载中、待验证……）显示在对应行的状态列和任务详情里。

## 9. 验收清单

| 场景 | 应该怎样 |
| --- | --- |
| 未安装 | 显示未安装，不下载、不安装 |
| CLI 和 App 都装了 | 分开显示，不混用版本和命令 |
| 同一 App 有多份安装 | 分别显示；身份冲突时不自动执行 |
| 安装来源不明 | 显示已知信息，不猜升级命令 |
| 查不到最新版本，或离线 | 显示对应状态，不显示“已是最新” |
| App 正在运行 | 等用户退出，不强杀 |
| 权限被拒 | 明确失败或等待授权，不在后台反复重试 |
| 确认后安装、版本或来源变了 | 旧计划作废，要求重新确认 |
| App 已被自带更新器升级 | 重新判断，不用旧包覆盖 |
| WinGet 只匹配到相似名称或其他来源 | 不执行 |
| WinGet 明确说没有更新 | 结果为 `no_update`，不算失败 |
| 安装包哈希、签名、来源或架构不符 | 拒绝执行 |
| 更新清单里有多个架构 | 按 App 实际架构选 |
| 安装器父进程提前退出、超时、UI 重启 | 显示“待验证”，不重复安装 |
| 只打开了官方入口或给出了命令 | 记为“待手动更新”，不算成功 |
| 命令成功但版本没变 | 显示“版本未变”，不说已是最新 |
| 版本降低或跨版本线 | 报告失败 |
| 单项失败 | 其他独立项目继续，最后逐项报告 |
| 另一个 UI 或 CLI 同时发起升级 | 互斥，后来的提示“忙” |
| 原有 CLI 功能、macOS Homebrew、离线模式 | 行为不倒退 |

测试方法：单元测试和模拟接口验证流程；签名、权限、安装窗口、真实升级必须在对应平台的测试机上验收，不能用跨平台编译或模拟测试代替。

## 10. 已拍板的决定和待验证

已拍板（2026-10-15）：

1. **接受 WinGet 社区源作为可信来源**。只对 ProductCode 或包族精确对上、且 WinGet 报告的已装版本和 AgentUp 检测到的一致的安装执行；确认页写明来源是 WinGet。
2. **Linux 授权命令在命令行模式下直接执行**。用户在当前终端自己输入 sudo 密码；Web 界面只显示命令供复制，HTTP 接口拒绝执行这类计划。
3. **批量升级不包含 Codex 桌面版**。它的计划标记为“需单独确认”，批量里跳过并说明原因。

实现中又确认的事（2026-10-15，本机只读）：

- Antigravity 注册表卸载记录的键名就是 `121a0be4-63bd-531e-acf8-fc3924c7e984`，和 WinGet 的 ProductCode 一致；`winget list` 能精确对上，报告已装 2.18.1、可用 2.23.1。
- **OpenCode 有降级风险**：本机 v2（2.0.29）会被 WinGet 对上只收录 1.x 的 `SST.OpenCodeDesktop`。所以 OpenCode 只有“已装 1.x 且可用版本也是 1.x”时才走 WinGet，v2 一律走官方安装包。
- Codex 的 msstore 源能对上已装版本，但 `winget list` 不显示可用版本。所以 Codex 的最新版本显示“未知”，仍可发起升级，没有更新时结果为 `no_update`。
- OpenCode 官方更新清单 Windows（`latest.yml`）、Linux（`latest-linux.yml`、`latest-linux-arm64.yml`）都能读到，含版本、安装包地址、大小和 SHA-512。Linux 的 OpenCode 因此也能查最新版本。
- OpenCode 主程序和卸载程序的签名有效，发布者是 `Anomaly Innovations, Inc`；Antigravity 主程序未签名。
- WinGet 的退出码按官方返回码表映射（无适用更新、App 正在运行、需要重启、协议未接受、被策略阻止等）。

还需要在真机上验证：

- Codex：真正执行 msstore 升级时的行为；退出 Codex 后 AgentUp 任务能否独立跑完。
- Claude Windows：WinGet 版本是否落后；MSIX 和 EXE 两种装法的更新方式。
- Antigravity Windows：真正执行 WinGet 升级（参数 `--updated`）后，App 和版本记录是否正常。
- OpenCode v2：安装器的交互流程；怎么判断安装真的完成。
- macOS：四个 Cask 指名升级时的实际行为；Intel 机器上 ChatGPT 能不能用。
- Linux：ChatGPT DNF 仓库的包名；Claude APT 仓库在 arm64 上是否可用；没有图形界面时怎么检测运行状态。

## 附录 A：已有的本机只读观察（2026-10-15，Windows）

这些只是当天的观察，不是永久配置，也不证明升级能成功。期间没有运行任何安装或升级命令。

| 对象 | 观察到的 | 能说明 | 不能说明 |
| --- | --- | --- | --- |
| ChatGPT / Codex | WinGet 能用 `9PLM9XGG6VKS` + `msstore` 精确对上，安装版本 `26.1007.9446.0` | 有一条能精确对上的包管理器路径 | 当前有没有更新、需不需要提权、执行能否成功 |
| OpenCode | 本机 `resources/app-update.yml` 指向的更新清单返回 200，含版本、x64/arm64 安装包、大小、SHA-512；当时版本 `2.0.29` | 可以继续做候选版本和安装包获取 | 安装器签名、参数、完成判断；当时和本机同版本，不是“有更新”的样本 |
| Antigravity | 本机 `resources/app-update.yml` 指向的地址直接请求返回 404 | 不能靠这份配置开放执行 | 不代表没有更新，也不代表 App 自带更新失效 |
| Antigravity 签名 | 本机主程序签名检查为 NotSigned | 不能拿主程序签名当安装包的信任依据 | 不代表所有官方安装包都没签名 |
| Claude | 本机没装 | 缺少本机样本 | 不能照搬其他 Electron App 的更新方式 |

## 附录 C：真机验收记录

| 日期 | 格子 | 环境 | 过程和结果 |
| --- | --- | --- | --- |
| 2026-10-15 | Windows + WinGet + Antigravity | Windows 10 x64，用户级安装，WinGet v1.29.380 | App 未运行时执行 `agentup upgrade antigravity --kind desktop --yes`，结果 `updated`，2.18.1 → 2.23.1，用时 86 秒。独立复查：卸载记录和主程序版本都是 2.23.1，安装位置不变，App 没被自动打开，`winget list` 不再显示可用版本；再执行一次 `winget upgrade` 返回 `0x8A15002B`（无适用更新），和 `no_update` 映射一致。已加入默认开启名单 |

## 附录 B：参考资料

这些资料用于确认产品和分发方式，不代表外部升级流程已经验证。页面和渠道可能变化，实施时要重新核对。

- [S1] [OpenAI: What's new（含 Codex 并入 ChatGPT 桌面版）](https://learn.chatgpt.com/docs/whats-new)
- [S2] [OpenAI: ChatGPT desktop app for Linux](https://learn.chatgpt.com/docs/linux/linux-app)
- [S3] [OpenAI: Deploy the Windows app](https://learn.chatgpt.com/docs/enterprise/windows-deployment)
- [S4] [OpenAI: Manage app updates](https://learn.chatgpt.com/docs/enterprise/manage-app-updates)
- [S5] [Claude: Claude Code Desktop](https://code.claude.com/docs/en/desktop)
- [S6] [Claude: Desktop on Linux](https://code.claude.com/docs/en/desktop-linux)
- [S7] [OpenCode: Download](https://opencode.ai/download)
- [S8] [Antigravity: Download](https://antigravity.google/download)
- [S9] [Antigravity: Releases](https://antigravity.google/releases)
- [S10] [Homebrew: FAQ（含自更新 App 与实际版本检查）](https://docs.brew.sh/FAQ)
- [S11] Homebrew Cask 数据（2026-10-15 查询）：[chatgpt](https://formulae.brew.sh/api/cask/chatgpt.json)、[claude](https://formulae.brew.sh/api/cask/claude.json)、[opencode-desktop](https://formulae.brew.sh/api/cask/opencode-desktop.json)、[antigravity](https://formulae.brew.sh/api/cask/antigravity.json)。四个都标了 `auto_updates`。
- [S12] [WinGet 社区源 winget-pkgs](https://github.com/microsoft/winget-pkgs)（2026-10-15 查询）：`Google.Antigravity` 2.23.1（NSIS，用户级，带 ProductCode）；`Anthropic.Claude` 2.19675.1（MSIX + EXE）；`SST.OpenCodeDesktop` 只有 1.x；`OpenAI.Codex` 是 CLI。
- [S13] [Antigravity 下载页（Linux）](https://antigravity.google/download?os=linux)：app 只提供 tar.gz。
- [S14] [Microsoft: winget upgrade](https://learn.microsoft.com/en-us/windows/package-manager/winget/upgrade)
- [S15] [Electron autoUpdater](https://www.electronjs.org/docs/latest/api/auto-updater)：App 内更新接口的背景，不代表存在通用的外部更新命令。
