<p align="center">
  <img src="logo.png" alt="Timo" width="120" />
</p>

<h1 align="center">Timo</h1>
<p align="center">桌面端的「灵动岛」— 把 AI 编码状态、媒体播放和系统监控浓缩在一个胶囊栏里</p>

<p align="center">
  <img src="https://img.shields.io/badge/platform-Linux%20%7C%20Windows-lightgrey" alt="平台" />
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go" alt="Go" />
  <img src="https://img.shields.io/badge/Vue-3-4FC08D?logo=vue.js" alt="Vue" />
  <img src="https://img.shields.io/badge/Wails-v3-blue" alt="Wails" />
</p>

---

## 目录

- [什么是 Timo？](#什么是-timo)
- [场景与优先级](#场景与优先级)
- [功能亮点](#功能亮点)
- [技术栈](#技术栈)
- [快速开始](#快速开始)
- [CLI 用法](#cli-用法)
- [Unix Socket API](#unix-socket-api)
- [项目结构](#项目结构)
- [架构概览](#架构概览)
- [平台支持状态](#平台支持状态)
- [License](#license)

---

## 什么是 Timo？

Timo 模仿了 iPhone 的「灵动岛」交互，把它搬到了桌面上。屏幕顶部中央悬浮一个**无边框的胶囊状态栏**，始终置顶、透明背景，根据当前场景自动切换显示模式。

点击胶囊展开完整的控制面板，鼠标离开 5 秒后自动收起（悬停时重置计时器）。

---

## 场景与优先级

Timo 按优先级在三种场景间自动切换，**同时只显示一个场景**：

| 优先级 | 场景 | 显示内容 |
|:---:|------|---------|
| 1 | **AI 编码进行中**（Claude Code / Reasonix） | AI Logo、当前工具名、工作目录、已用时长、交通灯状态指示（🟢 运行中 / 🟡 需关注 / 🔴 已完成）、绿色进度条动画、subagent 标识 |
| 2 | **媒体播放中**（MPRIS 兼容播放器） | 专辑封面、歌名与艺术家（滚动字幕）、剩余时间、播放/暂停按钮、波形动画、进度条；点击展开完整播放控制面板 |
| 3 | **空闲状态** | CPU 使用率、内存占用、当前时间 |

---

## 功能亮点

### 🤖 AI 编码集成 — Claude Code & Reasonix

- **双 AI 支持**：同时集成 [Claude Code](https://claude.ai) 和 [Reasonix](https://reasonix.ai) 的编码状态
- **自动 Hook 注入**：首次启动时自动将 Hook 配置写入 `~/.claude/settings.json` 和 `~/.reasonix/settings.json`，无需手动操作
- **Unix Socket 实时通信**：通过 `/tmp/timo.sock` 接收 AI 编码生命周期事件
- **三种状态指示**：`running`（绿色呼吸灯 + 进度条 + 计时器）、`attention`（黄色闪烁）、`done`（红色闪烁后自动清除）
- **子代理（Subagent）感知**：当 AI 派遣子代理时显示标识，完成后自动清除
- **后台进程守护**：持续监控 `claude` 和 `reasonix` 进程，多会话结束时自动清除状态

### 🎵 媒体播放控制

- Linux 上通过 **MPRIS / D-Bus** 无缝对接 Spotify、VLC、Firefox 等任何支持 MPRIS 的播放器
- 播放/暂停、上一首/下一首完整控制
- 专辑封面、进度条、波形动画一应俱全
- 展开面板提供更完整的媒体操控体验

### 📊 系统监控

- 每 2 秒从 `/proc/stat` 和 `/proc/meminfo` 读取 CPU 和内存数据
- 空闲时实时展示，一目了然

### 🧩 其他特性

- **Dock 模式窗口**：使用 X11 `_NET_WM_WINDOW_TYPE_DOCK` 隐藏任务栏条目，始终置顶
- **无边框透明背景**：60px 高度胶囊栏，融合桌面环境
- **多会话管理**：同时处理多个 Claude/Reasonix 会话，等待所有会话结束后再清除状态
- **服务器模式**：可构建为轻量 HTTP 服务器（支持 Docker），通过浏览器访问 Timo 界面

---

## 技术栈

| 层 | 技术 |
|---|------|
| 后端 | Go 1.25 · Wails v3 · godbus/dbus (MPRIS) |
| 前端 | Vue 3 (Composition API) · Pinia · Vite · TypeScript |
| 构建 | Taskfile · Wails CLI · nfpm (deb/rpm) · NSIS (Windows) · Docker |
| 平台 | Linux（完整支持）· Windows（基础支持）|

---

## 快速开始

### 环境要求

- Go 1.25+
- Node.js / npm
- Wails CLI v3：`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`
- Taskfile：`go install github.com/go-task/task/v3/cmd/task@latest`
- Linux 用户还需要 D-Bus 和 GTK/WebKit 库

### 开发模式（支持热重载）

```bash
# 使用 Wails CLI（推荐）
wails3 dev

# 或使用 Taskfile
task dev
```

### 构建生产版本

```bash
wails3 build
# 或
task build
```

构建产物位于 `bin/` 目录。

### 使用独立构建脚本

如果不想安装 Taskfile，可以使用项目自带的构建脚本：

```bash
# 构建到 bin/timo
./build.sh

# 清理后构建
./build.sh --clean
```

### 其他构建命令

```bash
task package              # 打包分发（deb/rpm/AppImage/NSIS）
task build:docker         # Docker 构建
task run:docker           # 构建并运行 Docker 容器（端口 8080）
task build:server         # 构建 HTTP 服务器模式
task run:server           # 运行 HTTP 服务器
task generate:bindings    # 重新生成前端 TypeScript 类型绑定
task generate:icons       # 生成应用图标（.ico）
```

---

## CLI 用法

Timo 内置了一套命令行子命令，用于管理 AI Hook 配置和发送通知：

### 配置 AI Hook

```bash
# 手动配置 Claude Code Hook（覆盖 ~/.claude/settings.json）
timo setup

# 手动配置 Reasonix Hook（覆盖 ~/.reasonix/settings.json）
timo setup-reasonix
```

> 这些命令通常在首次启动 GUI 时**自动执行**，无需手动运行。Hook 的自动注入会保留已有的非 Timo 配置项。

### 发送通知

```bash
# 发送 Claude Code 事件
timo notify --type claude-start --tool "Edit" --dir "~/project" --topic "Refactor"
timo notify --type claude-done

# 发送 Reasonix 事件
timo notify --type reasonix-start --tool "research" --dir "/tmp/analysis" --topic "Audit"
timo notify --type reasonix-attention --msg "Need your input"
timo notify --type reasonix-done
```

支持的事件类型前缀：`claude-` 和 `reasonix-`

| 事件 | 含义 |
|------|------|
| `*-start` / `*-prompt` | AI 开始工作（启动计时器） |
| `*-tool` | 更新当前使用的工具名 |
| `*-attention` / `*-notify` | 需要用户关注（黄色，8 秒自动清除） |
| `*-done` | AI 完成工作（红色，5 秒自动清除） |
| `*-subagent` | 子代理开始（显示标识） |
| `*-subagent-done` | 子代理结束 |

### 从 Hook 输入接收数据

`timo notify` 也可以从标准输入读取 JSON 数据，兼容 Claude Code 和 Reasonix 的 Hook 调用格式：

```bash
# 从管道输入（Claude Code 格式 — snake_case）
echo '{"type":"claude-start","tool":"Edit","work_dir":"/home/user/proj"}' | timo notify

# 从管道输入（Reasonix 格式 — camelCase）
echo '{"type":"reasonix-start","tool":"research","workDir":"/tmp/analysis"}' | timo notify
```

---

## Unix Socket API

Timo 在 `/tmp/timo.sock` 监听一个 Unix Domain Socket，任何程序都可以通过该 Socket 向 Timo 发送通知。

### 通知格式

```json
{
  "type":     "claude-start",     // 必填，事件类型
  "message":  "Edit file",        // 可选，显示的消息文本
  "tool":     "Edit",             // 可选，当前使用的工具名
  "work_dir": "/home/user/proj",  // 可选，工作目录
  "topic":    "Refactor API"      // 可选，任务主题
}
```

`type` 支持的事件类型同上表（`claude-start`、`claude-done`、`reasonix-start` 等）。

---

## 项目结构

```
timo/
├── main.go                 # 应用入口，Wails 窗口配置与事件注册
├── notify.go               # 📡 Unix Socket 通知服务、AI 进程监控、CLI 子命令、Hook 安装
├── sysinfo.go              # 📊 CPU / 内存数据采集（/proc/stat, /proc/meminfo）
├── mediaservice.go         # 🎵 媒体控制 Wails Service（Play/Pause/Next/Previous）
├── linux_window.go         # 🪟 Linux 窗口配置 — X11 DOCK 模式、置顶、隐藏任务栏
├── linux_window_other.go   # 非 Linux 平台的 stub
│
├── media/                  # 🎧 平台相关媒体集成
│   ├── provider.go         # MediaProvider 接口 + MediaInfo 结构体
│   ├── poller.go           # 通用轮询循环（按变化过滤发射）
│   ├── linux.go            # Linux MPRIS D-Bus 实现
│   ├── provider_linux.go   # Linux 工厂方法
│   ├── windows.go          # Windows 占位（待实现）
│   └── provider_windows.go # Windows 工厂方法（返回错误）
│
├── frontend/               # 🌐 Vue 3 前端
│   ├── src/
│   │   ├── main.ts         # 应用引导
│   │   ├── App.vue         # 根组件：<Island />
│   │   ├── components/     # UI 组件
│   │   │   ├── Island.vue        # 🏝️ 主控制器、展开/收起管理
│   │   │   ├── NotchBar.vue      # 💊 胶囊栏（三种显示模式）
│   │   │   ├── DropPanel.vue     # 📋 展开的媒体控制面板
│   │   │   └── WaveformBars.vue  # 🌊 音频波形动画
│   │   ├── stores/         # 📦 Pinia 状态管理
│   │   │   ├── media.ts          # 媒体播放状态
│   │   │   ├── notification.ts   # AI 通知状态机
│   │   │   └── system.ts         # 系统监控数据
│   │   ├── composables/    # 🔌 Wails 事件订阅（useMediaEvents、useNotificationEvents、useSystemEvents）
│   │   ├── types/          # TypeScript 类型定义
│   │   └── styles/         # 🎨 样式与动画（island.css, animations.css）
│   ├── bindings/           # 自动生成的 Wails TypeScript 绑定
│   └── public/             # 静态资源（claude.png, reasonix.png）
│
├── build/                  # 🔧 各平台构建资源与打包配置
│   ├── config.yml          # Wails 项目元数据
│   ├── Taskfile.yml        # 通用构建任务
│   ├── linux/              # Linux：nfpm 配置、AppImage 脚本、.desktop 文件
│   ├── windows/            # Windows：NSIS 脚本、MSIX 清单
│   └── docker/             # Dockerfile（跨编译 + 服务器模式）
│
├── docs/                   # 📖 开发文档
├── Taskfile.yml            # 📋 任务编排入口
└── build.sh                # 🚀 独立构建脚本（无需 Taskfile）
```

---

## 架构概览

Timo 采用 **Go 后端 + Vue 前端** 的 Wails 应用架构：

```
┌────────────────────────────────────────────────────────────┐
│                     Timo (Wails App)                        │
│                                                            │
│  ┌──────────────┐    ┌──────────────────────┐              │
│  │   Go 后端      │    │  Wails Events Bus     │              │
│  │              │    │                      │              │
│  │  notify.go ──┼───→│  "notification"      │              │
│  │  sysinfo.go ─┼───→│  "sys-stats"         │──────────────┼──→ Vue 前端
│  │  poller.go ──┼───→│  "media-update"      │  (Events.On) │   (Pinia Stores)
│  │              │    │                      │              │
│  │  MediaService│    │  ← Wails Service ────│──────────────┼──→ Vue (Play/Pause...)
│  └──────────────┘    └──────────────────────┘              │
│                                                            │
│  ┌─────────────────────────────────────────────────┐       │
│  │  Unix Socket Server (/tmp/timo.sock) ←── notify │       │
│  │  CLI / Claude Hooks / Reasonix Hooks            │       │
│  └─────────────────────────────────────────────────┘       │
└────────────────────────────────────────────────────────────┘
```

**数据流**：Go 后端通过轮询（系统状态、媒体播放）或 Socket 事件（AI 通知）收集数据，通过 Wails Events Bus 推送到 Vue 前端。前端通过 Pinia 状态管理响应式更新 UI。用户操作（如点击播放按钮）通过 Wails Service RPC 回调用后端。

---

## 平台支持状态

| 平台 | 状态 | 备注 |
|------|:----:|------|
| Linux (X11) | ✅ 完整支持 | MPRIS 媒体控制、系统监控、Dock 窗口均已就绪 |
| Linux (Wayland) | ⚠️ 基础支持 | 功能可用，但 Dock 窗口模式仅在 X11 下生效 |
| Windows | ✅ 支持 | 框架就绪（媒体功能需 MPRIS 兼容播放器支持）|
| macOS | ❌ 不支持 | Timo 不支持 macOS，请使用 Linux 或 Windows |

> **关于 macOS**：Timo 基于 Wails v3 构建，但 macOS 平台尚未实现全局热键、系统监控和 Dock 窗口等核心功能。如果希望支持 macOS，欢迎提交 PR。注意 `build/darwin/` 和 `build/ios/` 目录已移除。

---

## License

MIT
