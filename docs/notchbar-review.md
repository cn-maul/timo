# 刘海屏展示逻辑 — 最终审核意见（合并版）

> 合并自两轮审核：my-review（针对外部修改建议的逐条评估）+ ai-review（针对当前实现的全面审计）。
> 基于 commit `9a2e0c5`，审核源码：`notify.go` (559行)、`notification.ts` (157行)、`NotchBar.vue` (143行)

---

## 总结：两份审核的共识与分歧

### 完全共识
| 项目 | 一致判断 |
|------|---------|
| prompt 时清空 tool | P0 bug，必须修 |
| 删除 claude-start/reasonix-start 死代码 | P0，零风险 |
| 不做 store 层 30 字符硬截断 topic | CSS ellipsis + 可选后端软限制 |
| message 不属于 running 态 | 设计正确，不应改动 |
| 信息层次需要改善 | 13px/11px/10px 差距不足 |

### ai-review 新增发现（my-review 遗漏）
| 项目 | 评估 |
|------|------|
| **ProcessMonitor 首次 check 竞态** | ✅ 确认是真 bug，见下方详述 |
| **default 分支静默 attention 误触** | ✅ 合理防御，但严重度低于前者 |
| **Notification struct 注释遗漏** | ✅ 正确，注释只列了 3 种 type，实际有 6 种 |
| **Reasonix 缺少 Subagent hook** | ✅ 应加注释说明 |
| **多实例 topic 覆盖** | ✅ 确认是已知限制，当前不建议修 |
| **DropPanel 历史通知面板** | ⚠️ 新功能范畴，可作为后续需求 |
| **快速工具调用 UI 闪烁** | 保持现状，不做 debounce |

### computed 重构的共识（两份审核均涉及）
| 项目 | 一致判断 |
|------|---------|
| computed 封装方向正确 | 两份审核均认为只重构 running 态，不碰 attention/done |
| 不合并 tool · dir | 破坏三行层次，窄窗口更拥挤 |

---

## 🔴 P0 — 必须修复（2 项）

### 1. prompt 时清空 tool

**文件**: `notification.ts:64`

```ts
// 当前
// Don't clear tool — let PostToolUse update it naturally

// 改为
tool.value = ''
```

**理由**: `UserPromptSubmit` 先于 `PostToolUse` 到达，窗口期内旧 tool 名残留误导用户。清空后 fallback 为"Claude 运行中"，比显示过期工具名更诚实。

### 2. 删除 legacy dead code

**文件**: `notification.ts:91-103`

删除 `claude-start` / `reasonix-start` 整个 case 分支。`buildHooksConfig()` 已不注册 `PreToolUse`，`installHooks()` 在启动时过滤旧 hook。

---

## 🟡 P1 — 应该修复（3 项）

### 3. ProcessMonitor 首次 check 竞态（ai-review 首次发现）

**文件**: `notify.go:108-131`

**问题**: `NewProcessMonitor` 初始化时 `lastCount` 全为 0。如果 timo 启动时 Claude 已在运行，需要等 1 秒后首次 `check()` 才能记录实际计数。若 Claude 恰好在这 1 秒内退出，`prev=0, count=0`，done 事件丢失，刘海栏永久卡在 running。

**修复**（方案 B 更干净）：

```go
// notify.go:108 — NewProcessMonitor 中初始化计数
func NewProcessMonitor(watchNames []string, emitter func(Notification)) *ProcessMonitor {
    pm := &ProcessMonitor{
        emitter:    emitter,
        stopCh:     make(chan struct{}),
        lastCount:  make(map[string]int),
        watchNames: watchNames,
    }
    // 初始化时立即记录当前进程计数，避免首次 check 前的遗漏窗口
    for _, name := range watchNames {
        pm.lastCount[name] = countProcesses(name)
    }
    return pm
}
```

**为什么不选方案 A**（在 `Start()` goroutine 中先调 `m.check()`）：goroutine 启动时序不确定，且 `check()` 内部有 emitter 回调可能在 server 未就绪时触发。方案 B 在构造函数中同步初始化，无副作用。

### 4. default 分支加 warn 日志

**文件**: `notification.ts:124-131`

```ts
default:
    // 当前：任何带 message 的未知 type 都静默进入 attention
    // 改为：
    console.warn(`[notification] unhandled type: ${notif.type}`)
    if (notif.message) {
        // ... 原有逻辑
    }
```

**理由**: 防御性编程。未来 Claude 新增 hook 类型时，不会无声无息地触发 attention。

### 5. Notification struct 注释补充

**文件**: `notify.go:29`

```go
// 当前
Type string `json:"type"` // "claude-prompt"/"reasonix-prompt", "claude-done"/"reasonix-done", "claude-notify"/"reasonix-notify"

// 改为
Type string `json:"type"` // "claude-prompt"/"reasonix-prompt", "claude-tool"/"reasonix-tool", "claude-subagent"/"reasonix-subagent", "claude-subagent-done", "claude-notify"/"reasonix-notify", "claude-done"/"reasonix-done"
```

---

## 🟢 P2 — 改善项（3 项）

### 6. 信息层次字号/颜色微调

**文件**: `island.css`

```css
/* 当前: gap: 1px, 三行颜色层次不明显 */
/* 建议: */
.claude-tool { color: #888; }   /* 原 #999，更灰 */
.claude-dir  { color: #555; }   /* 原 #666，更暗 */
/* gap: 1px → 2px, 增加行间呼吸感 */
```

### 7. topic 软限制（后端，非显示截断）

**文件**: `notify.go:260`

```go
// 当前
notif.Topic = prompt

// 改为（内存保护，不是显示截断）
if len(prompt) > 500 {
    notif.Topic = prompt[:500]
} else {
    notif.Topic = prompt
}
```

**理由**: 500 字符足够保留完整语义，同时防止极端长文本占用内存。CSS ellipsis 继续负责显示截断。

### 8. running 态主文本 computed 封装

**文件**: `NotchBar.vue`

```ts
// 只封装 running 态的主文本选择逻辑，不碰 attention/done
const primaryText = computed(() => {
    if (notif.state !== 'running') return ''
    return notif.topic || notif.tool || (notif.source === 'reasonix' ? 'Reasonix 运行中' : 'Claude 运行中')
})

const showToolLine = computed(() =>
    notif.state === 'running' && notif.topic && notif.tool
)
```

**注意**: 不采用原建议的 `secondaryText = parts.join(' · ')` 方案——把 tool 和 dir 合并为一行会破坏现有的三行层次结构。dir 保持独立第三行。

---

## ⏸️ 暂缓 / 不修复（4 项）

| 项目 | 理由 |
|------|------|
| running 态显示 message | 设计正确，message 语义是 attention/done |
| store 层 30 字符硬截断 topic | 中英混排场景丢关键信息，CSS + 后端 500 软限制已足够 |
| secondaryText 合并 tool · dir | 窄窗口更拥挤，破坏视觉层次 |
| 多实例 topic 覆盖 | 需要后端实例 ID 方案，复杂度过高 |
| DropPanel 历史通知 | 新功能，单独评估 |

---

## 架构讨论：设计权衡与边界分析

### 快速连续工具调用的 UI 闪烁

`PostToolUse` 在每次工具调用后触发。Claude 执行 `Read → Think → Edit → Read` 时，tool 行高频闪烁。

**这是设计权衡，不是 bug**。实时更新比延迟更新更准确。debounce（如 300ms）的副作用是快速小工具（如 50ms 的 Read）可能从未展示就被覆盖。

**结论**: 保持现状。闪烁是同一位置文字变化，非布局变动。

### attention/done 态的 8 秒消息丢失窗口

`ATTENTION_CLEAR_DELAY = 8000`，`DONE_CLEAR_DELAY = 5000`。如果用户在这段时间内没注意到刘海栏，信息就永久丢失了（`reset()` 清空所有字段）。

**可讨论的方向**: DropPanel 扩展显示历史通知，让用户点开刘海栏查看。当前 DropPanel 只有媒体信息。这是新功能范畴，不在本次修复范围内。

### 多实例 topic 覆盖

Claude A 处理"分析整个代码库"时启动 Claude B（"帮我写个函数"），B 的 `claude-prompt` 会覆盖 `topic` 字段。刘海栏显示 B 的 topic，但 A 仍在 running。

**已知限制**。解决需要后端为每个实例分配 ID 并跟踪各自的 topic，复杂度显著增加，当前阶段不建议实施。

---

## 改动清单（按文件）

### `notification.ts`
1. L64: `tool.value = ''` (P0)
2. L91-103: 删除 `claude-start`/`reasonix-start` 分支 (P0)
3. L124: 加 `console.warn` (P1)

### `notify.go`
4. L29: 注释补充完整 event types (P1)
5. L108-114: `NewProcessMonitor` 中初始化 `lastCount` (P1)
6. L260: topic 500 字符软限制 (P2)

### `NotchBar.vue`
7. `primaryText` / `showToolLine` computed (P2)

### `island.css`
8. `.claude-tool` 颜色 `#999` → `#888` (P2)
9. `.claude-dir` 颜色 `#666` → `#555` (P2)
10. `.claude-info` gap `1px` → `2px` (P2)
