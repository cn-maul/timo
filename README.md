<p align="center">
  <img src="logo.png" alt="Timo" width="120" />
</p>

<h1 align="center">Timo</h1>
<p align="center">桌面端的灵动岛 — 把 AI 编码状态、媒体播放和系统监控浓缩在一个胶囊栏里</p>

<p align="center">
  <img src="https://img.shields.io/badge/platform-Linux%20%7C%20Windows-lightgrey" alt="平台" />
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go" alt="Go" />
  <img src="https://img.shields.io/badge/Vue-3-4FC08D?logo=vue.js" alt="Vue" />
  <img src="https://img.shields.io/badge/Wails-v3-blue" alt="Wails" />
</p>

---

## 什么是 Timo？

Timo 模仿了 iPhone 的「灵动岛」交互，把它搬到了桌面上。屏幕顶部中央悬浮一个无边框的胶囊状态栏，始终置顶、透明背景，根据当前场景自动切换三种显示模式：

| 优先级 | 场景 | 显示内容 |
|:---:|------|---------|
| 1 | **Claude Code 编码中** | Claude Logo、当前工具名、工作目录、已用时长、交通灯状态指示（绿=运行中 / 黄=需关注 / 红=已完成）、绿色进度条动画 |
| 2 | **媒体播放中** | 专辑封面、歌曲名、艺术家、剩余时间、播放/暂停按钮、波形动画、进度条；点击展开完整播放控制面板 |
| 3 | **空闲状态** | CPU 使用率、内存占用、当前时间 |

展开面板在鼠标离开 5 秒后自动收起，悬停时重置计时器。

## 功能亮点

### Claude Code 集成
- 首次启动时自动向 `~/.claude/settings.json` 注入 `PreToolUse`、`Stop`、`Notification` 三个 Hook
- 通过 Unix Socket（`/tmp/timo.sock`）实时接收 Claude Code 生命周期事件
- 后台守护进程监控 `claude` 进程，确保状态及时清除

### 媒体播放控制
- Linux 上通过 **MPRIS / D-Bus** 无缝对接 Spotify、VLC、Firefox 等任何支持 MPRIS 的播放器
- 支持播放/暂停、上一首/下一首
- 专辑封面、进度条、波形动画一应俱全

### 系统监控
- 每 2 秒从 `/proc/stat` 和 `/proc/meminfo` 读取 CPU 和内存数据
- 空闲时实时展示，一目了然

## 技术栈

| 层 | 技术 |
|---|------|
| 后端 | Go 1.25 · Wails v3 · godbus/dbus (MPRIS) |
| 前端 | Vue 3 (Composition API) · Pinia · Vite · TypeScript |
| 构建 | Taskfile · Wails CLI · nfpm (deb/rpm) · NSIS (Windows) |
| 平台 | Linux（完整支持）· Windows（基础支持，媒体功能开发中）|

## 快速开始

### 环境要求

- Go 1.25+
- Node.js / npm
- Wails CLI v3：`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`
- Taskfile：`go install github.com/go-task/task/v3/cmd/task@latest`
- Linux 用户还需要 D-Bus 和 GTK/WebKit 库

### 开发模式

```bash
# 使用 Wails CLI（推荐，支持热重载）
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

### 其他命令

```bash
task run          # 构建并运行
task package      # 打包分发（deb/rpm/AppImage/NSIS）
task build:docker # Docker 构建
```

## 项目结构

```
timo/
├── main.go              # 应用入口，Wails 窗口配置
├── notify.go            # Unix Socket 通知服务、Claude 进程监控、CLI 子命令
├── sysinfo.go           # CPU / 内存数据采集
├── mediaservice.go      # 媒体控制 Wails Service
├── media/               # 平台相关媒体集成（MPRIS 实现）
├── frontend/            # Vue 3 前端
│   └── src/
│       ├── components/  # Island、NotchBar、DropPanel、WaveformBars
│       ├── stores/      # Pinia 状态管理（媒体、通知、系统）
│       ├── composables/ # Wails 事件订阅
│       └── styles/      # 样式与动画
├── build/               # 各平台构建资源与打包配置
└── Taskfile.yml         # 任务编排
```

## CLI 用法

```bash
# 手动配置 Claude Code Hook
timo setup

# 发送通知到运行中的 Timo 实例
timo notify --type claude-start --tool "Edit" --dir "~/project"
```

## 平台支持状态

| 平台 | 状态 | 备注 |
|------|:----:|------|
| Linux | ✅ 完整支持 | MPRIS 媒体控制、系统监控均已就绪 |
| Windows | 🔨 基础支持 | 框架就绪，媒体功能待实现 |
| macOS | 📋 已规划 | 构建资源已存在 |
| iOS / Android | 📋 已规划 | Wails Mobile 脚手架已存在 |

## License

MIT
