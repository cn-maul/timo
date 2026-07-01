<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'
import { useSettingsStore } from '../stores/settings'

const props = defineProps<{
  standalone?: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

const settings = useSettingsStore()

// Sidebar navigation
type PageId = 'display' | 'network' | 'hooks'
const activePage = ref<PageId>('display')

const pages = [
  { id: 'display' as const, label: '显示', icon: '🖥️' },
  { id: 'network' as const, label: '网络', icon: '🌐' },
  { id: 'hooks' as const, label: 'Hooks', icon: '🔗' },
]

// Display settings
const localPriority = ref<string[]>(['ai', 'media'])
const localIdleDisplay = ref<'all' | 'cpu' | 'mem' | 'net' | 'none'>('all')
const localTheme = ref<'dark' | 'light' | 'frosted'>('dark')

// New display options
const localShowToolContext = ref(true)
const localShowToolProgress = ref(true)
const localShowSubagentDetails = ref(true)

// Network settings
const localNetUnit = ref<'kb' | 'mb' | 'auto'>('auto')

// Hooks settings
const hooksStatus = ref({
  claude: { installed: false, path: '', pathMismatch: false, currentPath: '' },
  reasonix: { installed: false, path: '', pathMismatch: false, currentPath: '' },
})
const hooksLoading = ref(false)
const hooksFeedback = ref('')

// Track active setTimeout IDs for cleanup on unmount
const pendingTimers: ReturnType<typeof setTimeout>[] = []

function scheduleTimer(fn: () => void, ms: number): ReturnType<typeof setTimeout> {
  const id = setTimeout(() => {
    pendingTimers.splice(pendingTimers.indexOf(id), 1)
    fn()
  }, ms)
  pendingTimers.push(id)
  return id
}

onMounted(() => {
  if (settings.loaded) {
    localPriority.value = [...settings.displayPriority]
    localIdleDisplay.value = settings.idleDisplay as any || 'all'
    localTheme.value = settings.theme as any || 'dark'
    localShowToolContext.value = settings.showToolContext
    localShowToolProgress.value = settings.showToolProgress
    localShowSubagentDetails.value = settings.showSubagentDetails
    localNetUnit.value = settings.netUnit as any || 'auto'
  }
  Events.Emit('get-hooks-status')
})

const unsubLoaded = settings.$subscribe(() => {
  if (settings.loaded) {
    localPriority.value = [...settings.displayPriority]
    localIdleDisplay.value = settings.idleDisplay as any || 'all'
    localTheme.value = settings.theme as any || 'dark'
    localShowToolContext.value = settings.showToolContext
    localShowToolProgress.value = settings.showToolProgress
    localShowSubagentDetails.value = settings.showSubagentDetails
    localNetUnit.value = settings.netUnit as any || 'auto'
  }
})

const unsubHooksStatus = Events.On('hooks-status', (event: { data: typeof hooksStatus.value }) => {
  if (event.data) {
    hooksStatus.value = event.data
  }
})

const unsubHooksFeedback = Events.On('hooks-feedback', (event: { data: string }) => {
  if (event.data) {
    hooksFeedback.value = event.data
    scheduleTimer(() => { hooksFeedback.value = '' }, 3000)
  }
})

onUnmounted(() => {
  unsubLoaded()
  unsubHooksStatus()
  unsubHooksFeedback()
  // Clear all pending timers to prevent callbacks after unmount
  pendingTimers.forEach(id => clearTimeout(id))
  pendingTimers.length = 0
})

function togglePriority(mode: string) {
  const idx = localPriority.value.indexOf(mode)
  if (idx === 0) return
  if (idx > 0) {
    const arr = [...localPriority.value]
    ;[arr[idx - 1], arr[idx]] = [arr[idx], arr[idx - 1]]
    localPriority.value = arr
  }
}

function saveSettings() {
  settings.displayPriority = [...localPriority.value]
  settings.idleDisplay = localIdleDisplay.value
  settings.theme = localTheme.value
  settings.showToolContext = localShowToolContext.value
  settings.showToolProgress = localShowToolProgress.value
  settings.showSubagentDetails = localShowSubagentDetails.value
  settings.netUnit = localNetUnit.value
  settings.save()
  showSavedFeedback()
}

const savedFeedback = ref(false)
function showSavedFeedback() {
  savedFeedback.value = true
  setTimeout(() => { savedFeedback.value = false }, 2000)
}

function resetDefaults() {
  localPriority.value = ['ai', 'media']
  localIdleDisplay.value = 'all'
  localTheme.value = 'dark'
  localNetUnit.value = 'auto'
  localShowToolContext.value = true
  localShowToolProgress.value = true
  localShowSubagentDetails.value = true
}

function injectHook(tool: 'claude' | 'reasonix') {
  hooksLoading.value = true
  Events.Emit('inject-hook', tool)
  scheduleTimer(() => { hooksLoading.value = false }, 1000)
}

function injectAllHooks() {
  hooksLoading.value = true
  Events.Emit('inject-hook', 'all')
  scheduleTimer(() => { hooksLoading.value = false }, 1000)
}

function removeHook(tool: 'claude' | 'reasonix') {
  hooksLoading.value = true
  Events.Emit('remove-hook', tool)
  scheduleTimer(() => { hooksLoading.value = false }, 1000)
}

function closeSettings() {
  if (props.standalone) {
    Events.Emit('close-settings')
  } else {
    emit('close')
  }
}

function onDragStart(e: MouseEvent) {
  if (!props.standalone) return
  let dragging = true
  const dragOffsetX = e.screenX - window.screenX
  const dragOffsetY = e.screenY - window.screenY

  function onDragMove(e: MouseEvent) {
    if (!dragging) return
    window.moveTo(e.screenX - dragOffsetX, e.screenY - dragOffsetY)
  }

  function onDragEnd() {
    dragging = false
    document.removeEventListener('mousemove', onDragMove)
    document.removeEventListener('mouseup', onDragEnd)
  }

  document.addEventListener('mousemove', onDragMove)
  document.addEventListener('mouseup', onDragEnd)
}
</script>

<template>
  <div :class="standalone ? 'settings-standalone' : 'settings-overlay'" @click.self="closeSettings">
    <div :class="standalone ? 'settings-panel-full' : 'settings-panel'">
      <!-- Header -->
      <div class="settings-header" @mousedown="onDragStart">
        <h2>设置</h2>
        <button class="close-btn" aria-label="关闭设置" @click.stop="closeSettings">✕</button>
      </div>

      <div class="settings-body">
        <!-- Sidebar -->
        <nav class="sidebar">
          <button
            v-for="page in pages"
            :key="page.id"
            class="sidebar-item"
            :class="{ active: activePage === page.id }"
            :aria-label="page.label"
            @click="activePage = page.id"
          >
            <span class="sidebar-icon">{{ page.icon }}</span>
            <span class="sidebar-label">{{ page.label }}</span>
          </button>
        </nav>

        <!-- Content -->
        <div class="settings-content">
          <!-- Display Page -->
          <template v-if="activePage === 'display'">
            <div class="settings-section">
              <h3>显示优先级</h3>
              <p class="section-desc">当多个活动同时存在时，谁优先显示</p>
              <div class="priority-list">
                <div v-for="(mode, idx) in localPriority" :key="mode" class="priority-item">
                  <span class="priority-label">
                    {{ mode === 'ai' ? '🤖 AI 编码状态' : '🎵 媒体播放' }}
                  </span>
                  <span class="priority-badge">{{ idx === 0 ? '最高优先' : idx === localPriority.length - 1 ? '最低优先' : '' }}</span>
                  <button v-if="idx > 0" class="move-btn" @click="togglePriority(mode)" :aria-label="`提高 ${mode === 'ai' ? 'AI 编码状态' : '媒体播放'} 优先级`" title="提高优先级">↑</button>
                </div>
              </div>
            </div>

            <div class="settings-section">
              <h3>空闲显示</h3>
              <p class="section-desc">没有活动时刘海屏显示什么</p>
              <div class="radio-grid">
                <label class="radio-item" :class="{ active: localIdleDisplay === 'all' }">
                  <input type="radio" v-model="localIdleDisplay" value="all" />
                  全部
                </label>
                <label class="radio-item" :class="{ active: localIdleDisplay === 'cpu' }">
                  <input type="radio" v-model="localIdleDisplay" value="cpu" />
                  仅 CPU
                </label>
                <label class="radio-item" :class="{ active: localIdleDisplay === 'mem' }">
                  <input type="radio" v-model="localIdleDisplay" value="mem" />
                  仅内存
                </label>
                <label class="radio-item" :class="{ active: localIdleDisplay === 'net' }">
                  <input type="radio" v-model="localIdleDisplay" value="net" />
                  仅网络
                </label>
                <label class="radio-item" :class="{ active: localIdleDisplay === 'none' }">
                  <input type="radio" v-model="localIdleDisplay" value="none" />
                  不显示
                </label>
              </div>
            </div>

            <div class="settings-section">
              <h3>主题样式</h3>
              <p class="section-desc">切换界面配色方案</p>
              <div class="theme-grid">
                <div class="theme-card" :class="{ active: localTheme === 'dark' }" tabindex="0" role="radio" :aria-checked="localTheme === 'dark'" aria-label="深色主题" @click="localTheme = 'dark'" @keydown.enter="localTheme = 'dark'" @keydown.space.prevent="localTheme = 'dark'">
                  <div class="theme-preview theme-preview-dark"></div>
                  <span>深色</span>
                </div>
                <div class="theme-card" :class="{ active: localTheme === 'light' }" tabindex="0" role="radio" :aria-checked="localTheme === 'light'" aria-label="浅色主题" @click="localTheme = 'light'" @keydown.enter="localTheme = 'light'" @keydown.space.prevent="localTheme = 'light'">
                  <div class="theme-preview theme-preview-light"></div>
                  <span>浅色</span>
                </div>
                <div class="theme-card" :class="{ active: localTheme === 'frosted' }" tabindex="0" role="radio" :aria-checked="localTheme === 'frosted'" aria-label="磨砂主题" @click="localTheme = 'frosted'" @keydown.enter="localTheme = 'frosted'" @keydown.space.prevent="localTheme = 'frosted'">
                  <div class="theme-preview theme-preview-frosted"></div>
                  <span>磨砂</span>
                </div>
              </div>
            </div>

            <div class="settings-section">
              <h3>AI 显示增强</h3>
              <p class="section-desc">控制 Claude/Reasonix 运行时的显示细节</p>
              <div class="toggle-list">
                <label class="toggle-item">
                  <input type="checkbox" v-model="localShowToolContext" />
                  <span class="toggle-label">显示工具上下文</span>
                  <span class="toggle-desc">文件路径、命令摘要等</span>
                </label>
                <label class="toggle-item">
                  <input type="checkbox" v-model="localShowToolProgress" />
                  <span class="toggle-label">显示进度计数</span>
                  <span class="toggle-desc">工具调用次数和进度条</span>
                </label>
                <label class="toggle-item">
                  <input type="checkbox" v-model="localShowSubagentDetails" />
                  <span class="toggle-label">显示子代理详情</span>
                  <span class="toggle-desc">代理类型和任务描述</span>
                </label>
              </div>
            </div>
          </template>

          <!-- Network Page -->
          <template v-else-if="activePage === 'network'">
            <div class="settings-section">
              <h3>网络状态</h3>
              <p class="section-desc">网络信息将在刘海屏空闲时显示</p>
              <div class="info-box">
                <p>以下信息由系统实时检测：</p>
                <ul>
                  <li>下行速度</li>
                  <li>上行速度</li>
                  <li>本机 IP 地址</li>
                </ul>
              </div>
            </div>

            <div class="settings-section">
              <h3>速度单位</h3>
              <p class="section-desc">选择网速显示的单位格式</p>
              <div class="radio-grid">
                <label class="radio-item" :class="{ active: localNetUnit === 'auto' }">
                  <input type="radio" v-model="localNetUnit" value="auto" />
                  自动 (K/M)
                </label>
                <label class="radio-item" :class="{ active: localNetUnit === 'kb' }">
                  <input type="radio" v-model="localNetUnit" value="kb" />
                  KB/s
                </label>
                <label class="radio-item" :class="{ active: localNetUnit === 'mb' }">
                  <input type="radio" v-model="localNetUnit" value="mb" />
                  MB/s
                </label>
              </div>
            </div>

            <div class="settings-section">
              <h3>显示选项</h3>
              <p class="section-desc">在"显示"页面的"空闲显示"中选择"全部"或"仅网络"以显示网络状态</p>
            </div>
          </template>

          <!-- Hooks Page -->
          <template v-else-if="activePage === 'hooks'">
            <div class="settings-section">
              <h3>Hook 注入管理</h3>
              <p class="section-desc">配置 AI 工具的事件通知 Hook，让 Timo 能够显示编码状态</p>

              <div class="hooks-grid">
                <!-- Claude Code -->
                <div class="hook-card">
                  <div class="hook-header">
                    <span class="hook-icon">🤖</span>
                    <span class="hook-name">Claude Code</span>
                    <span class="hook-status" :class="{ installed: hooksStatus.claude.installed && !hooksStatus.claude.pathMismatch, warning: hooksStatus.claude.pathMismatch }">
                      {{ hooksStatus.claude.pathMismatch ? '⚠️ 路径变更' : (hooksStatus.claude.installed ? '✓ 已安装' : '未安装') }}
                    </span>
                  </div>
                  <div class="hook-path" v-if="hooksStatus.claude.path">{{ hooksStatus.claude.path }}</div>
                  <div class="hook-warning" v-if="hooksStatus.claude.pathMismatch">
                    Hooks 指向旧路径，建议重新注入以更新到当前路径
                  </div>
                  <div class="hook-actions">
                    <button class="btn btn-small" @click="injectHook('claude')" :disabled="hooksLoading" :aria-label="hooksStatus.claude.installed ? '重新注入 Claude Hook' : '注入 Claude Hook'">
                      {{ hooksStatus.claude.installed ? '重新注入' : '注入' }}
                    </button>
                    <button v-if="hooksStatus.claude.installed" class="btn btn-small btn-danger" @click="removeHook('claude')" :disabled="hooksLoading" aria-label="移除 Claude Hook">
                      移除
                    </button>
                  </div>
                </div>

                <!-- Reasonix -->
                <div class="hook-card">
                  <div class="hook-header">
                    <span class="hook-icon">🧠</span>
                    <span class="hook-name">Reasonix</span>
                    <span class="hook-status" :class="{ installed: hooksStatus.reasonix.installed && !hooksStatus.reasonix.pathMismatch, warning: hooksStatus.reasonix.pathMismatch }">
                      {{ hooksStatus.reasonix.pathMismatch ? '⚠️ 路径变更' : (hooksStatus.reasonix.installed ? '✓ 已安装' : '未安装') }}
                    </span>
                  </div>
                  <div class="hook-path" v-if="hooksStatus.reasonix.path">{{ hooksStatus.reasonix.path }}</div>
                  <div class="hook-warning" v-if="hooksStatus.reasonix.pathMismatch">
                    Hooks 指向旧路径，建议重新注入以更新到当前路径
                  </div>
                  <div class="hook-actions">
                    <button class="btn btn-small" @click="injectHook('reasonix')" :disabled="hooksLoading" :aria-label="hooksStatus.reasonix.installed ? '重新注入 Reasonix Hook' : '注入 Reasonix Hook'">
                      {{ hooksStatus.reasonix.installed ? '重新注入' : '注入' }}
                    </button>
                    <button v-if="hooksStatus.reasonix.installed" class="btn btn-small btn-danger" @click="removeHook('reasonix')" :disabled="hooksLoading" aria-label="移除 Reasonix Hook">
                      移除
                    </button>
                  </div>
                </div>
              </div>

              <div class="hooks-batch">
                <button class="btn btn-primary" @click="injectAllHooks" :disabled="hooksLoading" aria-label="全部注入 Hooks">
                  全部注入
                </button>
              </div>

              <div v-if="hooksFeedback" class="hooks-feedback">
                {{ hooksFeedback }}
              </div>
            </div>
          </template>
        </div>
      </div>

      <!-- Footer -->
      <div class="settings-footer">
        <button class="btn btn-secondary" @click="resetDefaults">恢复默认</button>
        <button class="btn btn-primary" @click="saveSettings" :disabled="savedFeedback">
          {{ savedFeedback ? '已保存 ✓' : '保存' }}
        </button>
      </div>
    </div>
  </div>
  <div aria-live="polite" aria-atomic="true" class="sr-only">
    {{ savedFeedback ? '设置已保存' : '' }}
  </div>
</template>

<style scoped>
/* ── SettingsPanel - Spotify Card & Sidebar System ── */
.settings-overlay {
  position: fixed;
  inset: 0;
  background: var(--timo-overlay-bg);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(8px);
}

.settings-standalone {
  width: 100%;
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--timo-bg);
  box-sizing: border-box;
  overflow: hidden;
}

.settings-panel {
  background: var(--timo-surface);
  border: none;
  border-radius: 16px;
  width: 700px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: var(--timo-shadow);
  color: var(--timo-text);
  overflow: hidden;
}

.settings-panel-full {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--timo-bg);
  color: var(--timo-text);
  overflow: hidden;
}

.settings-header {
  flex-shrink: 0;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 20px;
  border-bottom: 1px solid var(--timo-border);
  cursor: move;
  user-select: none;
}

.settings-header h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
}

.close-btn {
  background: none;
  border: none;
  color: var(--timo-gray);
  font-size: 16px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 6px;
  transition: all 0.15s;
}

.close-btn:hover {
  background: var(--timo-btn-hover);
  color: var(--timo-text);
}

.settings-body {
  flex: 1;
  display: flex;
  overflow: hidden;
}

/* ── Sidebar - Spotify Navigation Style ── */
.sidebar {
  width: 120px;
  flex-shrink: 0;
  background: var(--timo-surface-hover);
  border-right: 1px solid var(--timo-border);
  padding: 12px 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sidebar-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border: none;
  background: transparent;
  color: var(--timo-text-secondary);
  font-size: 14px;
  font-weight: 400;
  cursor: pointer;
  border-radius: 8px;
  transition: all 0.15s;
  text-align: left;
}

.sidebar-item:hover {
  background: var(--timo-list-item-hover);
  color: var(--timo-text);
}

.sidebar-item.active {
  background: var(--timo-list-item-active);
  color: var(--timo-text);
  font-weight: 700;
}

.sidebar-icon {
  font-size: 16px;
}

.settings-content {
  flex: 1;
  overflow-y: auto;
  padding: 20px 24px;
}

.settings-section {
  margin-bottom: 28px;
}

.settings-section h3 {
  margin: 0 0 4px 0;
  font-size: 14px;
  font-weight: 600;
}

.section-desc {
  margin: 0 0 14px 0;
  font-size: 12px;
  color: var(--timo-gray);
}

/* ── Priority List - Spotify Card Style ── */
.priority-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.priority-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  background: var(--timo-card-bg);
  border-radius: 10px;
  border: none;
}

.priority-label {
  flex: 1;
  font-size: 13px;
}

.priority-badge {
  font-size: 11px;
  color: var(--timo-gray);
  text-transform: uppercase;
  letter-spacing: 1.4px;
}

.move-btn {
  background: var(--timo-btn-bg);
  border: none;
  color: var(--timo-text);
  width: 26px;
  height: 26px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  transition: background 0.15s;
}

.move-btn:hover {
  background: var(--timo-btn-hover);
}

/* ── Radio Grid - Spotify Card Style ── */
.radio-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}

.radio-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  background: var(--timo-list-item-bg);
  border: none;
  transition: all 0.15s;
}

.radio-item:hover {
  background: var(--timo-list-item-hover);
}

.radio-item.active {
  background: var(--timo-list-item-active);
  border: 1px solid var(--timo-green);
}

.radio-item input {
  accent-color: var(--timo-green);
}

/* ── Theme Grid - Spotify Card Style ── */
.theme-grid {
  display: flex;
  gap: 12px;
}

.theme-card {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 14px 12px;
  border-radius: 12px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  border: 2px solid transparent;
  background: var(--timo-card-bg);
  transition: all 0.15s;
}

.theme-card:hover {
  background: var(--timo-card-hover);
}

.theme-card.active {
  border-color: var(--timo-green);
}

.theme-preview {
  width: 56px;
  height: 36px;
  border-radius: 8px;
  border: none;
}

.theme-preview-dark {
  background: linear-gradient(135deg, #121212 50%, #181818 50%);
}

.theme-preview-light {
  background: linear-gradient(135deg, #f0f0f0 50%, #ffffff 50%);
}

.theme-preview-frosted {
  background: linear-gradient(135deg, rgba(255,255,255,0.3) 50%, rgba(200,200,200,0.2) 50%);
  backdrop-filter: blur(10px);
}

/* ── Toggle List - Spotify Card Style ── */
.toggle-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.toggle-item {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-radius: 10px;
  cursor: pointer;
  font-size: 13px;
  background: var(--timo-list-item-bg);
  border: none;
  transition: all 0.15s;
}

.toggle-item:hover {
  background: var(--timo-list-item-hover);
}

.toggle-item input[type="checkbox"] {
  accent-color: var(--timo-green);
  width: 18px;
  height: 18px;
  margin: 0;
}

.toggle-label {
  font-weight: 600;
  flex: 1;
}

.toggle-desc {
  font-size: 12px;
  color: var(--timo-gray);
  width: 100%;
  margin-top: 4px;
  margin-left: 28px; /* align with checkbox */
}

/* ── Info Box - Spotify Card Style ── */
.info-box {
  background: var(--timo-card-bg);
  border-radius: 10px;
  padding: 14px 16px;
  font-size: 13px;
  line-height: 1.6;
}

.info-box p {
  margin: 0 0 8px 0;
}

.info-box ul {
  margin: 0;
  padding-left: 20px;
  color: var(--timo-text-secondary);
}

/* ── Hooks Grid - Spotify Card System ── */
.hooks-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.hook-card {
  padding: 16px;
  background: var(--timo-card-bg);
  border-radius: 12px;
  border: none;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.hook-header {
  display: flex;
  align-items: center;
  gap: 10px;
}

.hook-icon {
  font-size: 20px;
}

.hook-name {
  font-size: 14px;
  font-weight: 600;
  flex: 1;
}

.hook-status {
  font-size: 12px;
  color: var(--timo-text-secondary);
}

.hook-status.installed {
  color: var(--timo-green);
}

.hook-status.warning {
  color: var(--timo-yellow);
}

.hook-warning {
  font-size: 12px;
  color: var(--timo-yellow);
  background: rgba(255, 164, 43, 0.1);
  padding: 6px 10px;
  border-radius: 4px;
  margin: 8px 0;
  border-left: 3px solid var(--timo-yellow);
}

.hook-path {
  font-size: 12px;
  color: var(--timo-gray);
  font-family: monospace;
  background: var(--timo-badge-bg);
  padding: 4px 8px;
  border-radius: 4px;
  word-break: break-all;
}

.hook-actions {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}

.hooks-batch {
  margin-top: 20px;
  display: flex;
  justify-content: center;
}

.hooks-feedback {
  margin-top: 12px;
  padding: 10px 14px;
  background: rgba(30, 215, 96, 0.15);
  border-radius: 8px;
  font-size: 13px;
  text-align: center;
  color: var(--timo-green);
}

/* ── Footer - Spotify Button System ── */
.settings-footer {
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 20px;
  border-top: 1px solid var(--timo-border);
}

/* ── Button Styles - Spotify Pill & Circle Geometry ── */
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 8px 18px;
  border: none;
  border-radius: 9999px;
  font-size: 14px;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.15s, transform 0.1s;
  text-transform: uppercase;
  letter-spacing: 1.4px;
}

.btn:active {
  transform: scale(0.97);
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-small {
  padding: 6px 14px;
  font-size: 12px;
  border-radius: 9999px;
  letter-spacing: 1.4px;
}

.btn-secondary {
  background: var(--timo-btn-bg);
  color: var(--timo-text);
}

.btn-secondary:hover {
  background: var(--timo-btn-hover);
}

.btn-primary {
  background: var(--timo-green);
  color: #000;
}

.btn-primary:hover {
  opacity: 0.9;
  box-shadow: 0 0 0 2px var(--timo-green);
}

.btn-danger {
  background: rgba(243, 114, 127, 0.2);
  color: var(--timo-red);
}

.btn-danger:hover {
  background: rgba(243, 114, 127, 0.35);
}

.close-btn:focus-visible,
.btn:focus-visible,
.sidebar-item:focus-visible,
.theme-card:focus-visible,
.move-btn:focus-visible {
  outline: 2px solid var(--timo-green);
  outline-offset: 2px;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border-width: 0;
}
</style>