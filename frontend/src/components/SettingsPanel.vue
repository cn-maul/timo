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

// Local working copy — populated on mount, applied on save
const localPriority = ref<string[]>(['ai', 'media'])
const localIdleDisplay = ref<'all' | 'cpu' | 'mem' | 'none'>('all')
const localTheme = ref<'dark' | 'light'>('dark')

onMounted(() => {
  // If settings are already loaded, use them
  if (settings.loaded) {
    localPriority.value = [...settings.displayPriority]
    localIdleDisplay.value = settings.idleDisplay
    localTheme.value = settings.theme
  }
})

// Watch for settings loading completion
const unsubLoaded = settings.$subscribe(() => {
  if (settings.loaded) {
    localPriority.value = [...settings.displayPriority]
    localIdleDisplay.value = settings.idleDisplay
    localTheme.value = settings.theme
  }
})

onUnmounted(() => {
  unsubLoaded()
})

const savedFeedback = ref(false)

function togglePriority(mode: string) {
  const idx = localPriority.value.indexOf(mode)
  if (idx === 0) return // already highest, cannot promote
  if (idx > 0) {
    // Move up one position
    const arr = [...localPriority.value]
    ;[arr[idx - 1], arr[idx]] = [arr[idx], arr[idx - 1]]
    localPriority.value = arr
  }
}

function saveSettings() {
  settings.displayPriority = [...localPriority.value]
  settings.idleDisplay = localIdleDisplay.value
  settings.theme = localTheme.value
  settings.save()
  // Show saved feedback on the button instead of closing
  showSavedFeedback()
}

function showSavedFeedback() {
  savedFeedback.value = true
  setTimeout(() => { savedFeedback.value = false }, 2000)
}

function resetDefaults() {
  localPriority.value = ['ai', 'media']
  localIdleDisplay.value = 'all'
  localTheme.value = 'dark'
}

function closeSettings() {
  if (props.standalone) {
    Events.Emit('close-settings')
  } else {
    emit('close')
  }
}

function close() {
  emit('close')
}
</script>

<template>
  <div :class="standalone ? 'settings-standalone' : 'settings-overlay'" @click.self="close">
    <div :class="standalone ? 'settings-panel-full' : 'settings-panel'">
      <div class="settings-header">
        <h2>设置</h2>
        <button class="close-btn" @click="closeSettings">✕</button>
      </div>

      <!-- Priority -->
      <div class="settings-section">
        <h3>显示优先级</h3>
        <p class="section-desc">当多个活动同时存在时，谁优先显示</p>

        <div class="priority-list">
          <div
            v-for="(mode, idx) in localPriority"
            :key="mode"
            class="priority-item"
          >
            <span class="priority-label">
              {{ mode === 'ai' ? '🤖 AI 编码状态' : '🎵 媒体播放' }}
            </span>
            <span class="priority-badge">{{ idx === 0 ? '最高优先' : idx === localPriority.length - 1 ? '最低优先' : '' }}</span>
            <button
              v-if="idx > 0"
              class="move-btn"
              @click="togglePriority(mode)"
              title="提高优先级"
            >↑</button>
          </div>
        </div>
      </div>

      <!-- Idle display -->
      <div class="settings-section">
        <h3>空闲显示</h3>
        <p class="section-desc">没有活动时显示什么</p>

        <div class="radio-group">
          <label class="radio-item" :class="{ active: localIdleDisplay === 'all' }">
            <input type="radio" v-model="localIdleDisplay" value="all" />
            CPU + 内存
          </label>
          <label class="radio-item" :class="{ active: localIdleDisplay === 'cpu' }">
            <input type="radio" v-model="localIdleDisplay" value="cpu" />
            仅 CPU
          </label>
          <label class="radio-item" :class="{ active: localIdleDisplay === 'mem' }">
            <input type="radio" v-model="localIdleDisplay" value="mem" />
            仅内存
          </label>
          <label class="radio-item" :class="{ active: localIdleDisplay === 'none' }">
            <input type="radio" v-model="localIdleDisplay" value="none" />
            不显示
          </label>
        </div>
      </div>

      <!-- Theme -->
      <div class="settings-section">
        <h3>主题样式</h3>
        <p class="section-desc">切换界面配色方案</p>

        <div class="theme-grid">
          <div
            class="theme-card"
            :class="{ active: localTheme === 'dark' }"
            @click="localTheme = 'dark'"
          >
            <div class="theme-preview theme-preview-dark"></div>
            <span>深色</span>
          </div>
          <div
            class="theme-card"
            :class="{ active: localTheme === 'light' }"
            @click="localTheme = 'light'"
          >
            <div class="theme-preview theme-preview-light"></div>
            <span>浅色</span>
          </div>
        </div>
      </div>

      <!-- Actions -->
      <div class="settings-actions">
        <button class="btn btn-secondary" @click="resetDefaults">恢复默认</button>
        <button class="btn btn-primary" @click="saveSettings" :disabled="savedFeedback">
          {{ savedFeedback ? '已保存 ✓' : '保存' }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ── Modal overlay (used when not standalone) ── */
.settings-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
}

/* ── Standalone (fills the entire window, no overlay) ── */
.settings-standalone {
  width: 100%;
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--timo-bg);
  box-sizing: border-box;
}

/* Panel (modal) */
.settings-panel {
  background: var(--timo-surface);
  border: 1px solid var(--timo-border);
  border-radius: 16px;
  padding: 24px;
  width: 380px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.4);
  color: var(--timo-text);
}

/* Panel (standalone — fills the window edge-to-edge) */
.settings-panel-full {
  background: var(--timo-bg);
  padding: 24px;
  width: 100%;
  height: 100vh;
  overflow-y: auto;
  color: var(--timo-text);
  box-sizing: border-box;
}

/* Header */
.settings-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.settings-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
}

.close-btn {
  background: none;
  border: none;
  color: var(--timo-gray);
  font-size: 18px;
  cursor: pointer;
  padding: 2px 8px;
  border-radius: 6px;
  transition: background 0.15s;
}

.close-btn:hover {
  background: rgba(255, 255, 255, 0.1);
  color: var(--timo-text);
}

/* Sections */
.settings-section {
  margin-bottom: 22px;
}

.settings-section h3 {
  margin: 0 0 4px 0;
  font-size: 14px;
  font-weight: 600;
}

.section-desc {
  margin: 0 0 12px 0;
  font-size: 12px;
  color: var(--timo-gray);
}

/* Priority list */
.priority-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.priority-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.priority-label {
  flex: 1;
  font-size: 13px;
  font-weight: 500;
}

.priority-badge {
  font-size: 10px;
  color: var(--timo-gray);
  white-space: nowrap;
}

.move-btn {
  background: rgba(255, 255, 255, 0.08);
  border: none;
  color: var(--timo-text);
  width: 28px;
  height: 28px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  transition: background 0.15s;
}

.move-btn:hover {
  background: rgba(255, 255, 255, 0.18);
}

/* Radio group */
.radio-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.radio-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  transition: background 0.15s;
}

.radio-item:hover {
  background: rgba(255, 255, 255, 0.05);
}

.radio-item.active {
  background: rgba(255, 255, 255, 0.08);
}

.radio-item input {
  accent-color: var(--timo-green);
}

/* Theme grid */
.theme-grid {
  display: flex;
  gap: 12px;
}

.theme-card {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 12px 8px;
  border-radius: 10px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 500;
  border: 2px solid transparent;
  transition: border-color 0.15s, background 0.15s;
}

.theme-card:hover {
  background: rgba(255, 255, 255, 0.05);
}

.theme-card.active {
  border-color: var(--timo-green);
}

.theme-preview {
  width: 48px;
  height: 32px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.12);
}

.theme-preview-dark {
  background: linear-gradient(135deg, #0a0a0a 50%, #1a1a2e 50%);
}

.theme-preview-light {
  background: linear-gradient(135deg, #f5f5f5 50%, #ffffff 50%);
}

.theme-preview-frosted {
  background: linear-gradient(135deg, rgba(30, 30, 50, 0.65) 50%, rgba(60, 60, 90, 0.5) 50%);
}

/* Action buttons */
.settings-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.btn {
  padding: 8px 20px;
  border-radius: 8px;
  border: none;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s, transform 0.1s;
}

.btn:active {
  transform: scale(0.97);
}

.btn-secondary {
  background: rgba(255, 255, 255, 0.08);
  color: var(--timo-text);
}

.btn-secondary:hover {
  background: rgba(255, 255, 255, 0.14);
}

.btn-primary {
  background: var(--timo-green);
  color: #000;
}

.btn-primary:hover {
  opacity: 0.9;
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: default;
}
</style>
