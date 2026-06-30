<script setup lang="ts">
import { computed } from 'vue'
import { useMediaStore, formatTime } from '../stores/media'
import { useNotificationStore } from '../stores/notification'
import { useSettingsStore } from '../stores/settings'
import { Next, Previous } from '../../bindings/timo/internal/app/mediaservice'

const store = useMediaStore()
const notif = useNotificationStore()
const settings = useSettingsStore()

async function doNext() {
  try { await Next() } catch (e) { console.error('Next failed:', e) }
}

async function doPrev() {
  try { await Previous() } catch (e) { console.error('Previous failed:', e) }
}

// Determine which panel to show
const showMediaPanel = computed(() => store.hasMedia && notif.state !== 'running')
const showAIPanel = computed(() => notif.state === 'running' || notif.state === 'attention')

// Tool history for display (last 5)
const recentTools = computed(() => notif.toolHistory.slice(-5).reverse())

// Format duration for display
function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}
</script>

<template>
  <div class="drop-panel">
    <!-- AI Status Panel -->
    <template v-if="showAIPanel">
      <div class="ai-panel">
        <!-- Header -->
        <div class="ai-header">
          <img
            :src="notif.source === 'reasonix' ? '/reasonix.png' : '/claude.png'"
            class="ai-logo"
            :alt="notif.source === 'reasonix' ? 'Reasonix' : 'Claude'"
          />
          <div class="ai-title-info">
            <div class="ai-status">
              <span class="ai-status-dot" :class="notif.state"></span>
              {{ notif.state === 'running' ? '运行中' : notif.state === 'attention' ? '需要关注' : '完成' }}
            </div>
            <div class="ai-topic" v-if="notif.topic">{{ notif.topic }}</div>
          </div>
          <div class="ai-elapsed" v-if="notif.state === 'running'">
            {{ notif.elapsedText }}
          </div>
        </div>

        <!-- Current tool info -->
        <div class="ai-current" v-if="notif.state === 'running' && notif.tool">
          <div class="current-tool">
            <span class="tool-icon">{{ notif.toolIcon }}</span>
            <span class="tool-name">{{ notif.toolTarget || notif.tool }}</span>
            <span class="tool-duration" v-if="notif.durationMs">{{ formatDuration(notif.durationMs) }}</span>
          </div>
          <div class="current-context" v-if="notif.toolContext">
            {{ notif.toolContext }}
          </div>
        </div>

        <!-- Subagent info -->
        <div class="ai-subagent" v-if="settings.showSubagentDetails && notif.subagent && notif.agentType">
          <div class="subagent-header">
            <span class="subagent-icon">🤖</span>
            <span class="subagent-type">{{ notif.agentTypeName }}</span>
            <span class="subagent-status" v-if="notif.agentResult">✓</span>
          </div>
          <div class="subagent-desc" v-if="notif.agentDesc">{{ notif.agentDesc }}</div>
          <div class="subagent-result" v-if="notif.agentResult">
            <span class="result-label">结果:</span>
            {{ notif.agentResult }}
          </div>
        </div>

        <!-- Tool history -->
        <div class="ai-history" v-if="settings.showToolProgress && recentTools.length > 0">
          <div class="history-title">最近操作</div>
          <div class="history-list">
            <div v-for="(item, idx) in recentTools" :key="idx" class="history-item">
              <span class="history-icon">{{ notif.TOOL_ICONS[item.tool] || '🔧' }}</span>
              <span class="history-target">{{ item.target || item.tool }}</span>
              <span class="history-duration" v-if="item.duration">{{ formatDuration(item.duration) }}</span>
            </div>
          </div>
        </div>

        <!-- Stats -->
        <div class="ai-stats" v-if="notif.state === 'running' && settings.showToolProgress">
          <div class="stat-item">
            <span class="stat-label">工具调用</span>
            <span class="stat-value">{{ notif.toolCount }}</span>
          </div>
          <div class="stat-item" v-if="notif.workDir">
            <span class="stat-label">目录</span>
            <span class="stat-value stat-path">{{ notif.workDir.split('/').slice(-2).join('/') }}</span>
          </div>
        </div>

        <!-- Attention message -->
        <div class="ai-attention" v-if="notif.state === 'attention'">
          <div class="attention-icon">⚠️</div>
          <div class="attention-message">{{ notif.message }}</div>
        </div>
      </div>
    </template>

    <!-- Media Panel -->
    <template v-else-if="showMediaPanel">
      <div class="panel-top">
        <img
          v-if="store.safeCoverUrl"
          :src="store.safeCoverUrl"
          class="panel-cover"
          alt="Album cover"
        />
        <div v-else class="panel-cover" />
        <div class="panel-info">
          <div class="panel-title">{{ store.title || 'Unknown' }}</div>
          <div class="panel-artist">{{ store.artist || 'Unknown Artist' }}</div>
        </div>
      </div>

      <div class="progress-container">
        <div
          class="progress-bar-bg"
          role="progressbar"
          :aria-valuenow="store.progressPercent"
          aria-valuemin="0"
          aria-valuemax="100"
          :aria-label="`播放进度 ${Math.round(store.progressPercent)}%`"
        >
          <div
            class="progress-bar-fill"
            :style="{ width: store.progressPercent + '%' }"
          />
        </div>
        <div class="progress-time">
          <span>{{ formatTime(store.positionMs) }}</span>
          <span>{{ formatTime(store.durationMs) }}</span>
        </div>
      </div>

      <div class="controls">
        <button class="control-btn" aria-label="上一首" @click="doPrev">⏮</button>
        <button class="control-btn play-pause" aria-label="播放/暂停" @click="store.togglePlay">
          {{ store.playing ? '❚❚' : '▶' }}
        </button>
        <button class="control-btn" aria-label="下一首" @click="doNext">⏭</button>
      </div>
    </template>
  </div>
  <div aria-live="polite" aria-atomic="true" class="sr-only">
    {{ store.playing ? '正在播放' : '已暂停' }}
  </div>
</template>

<style scoped>
/* AI Panel Styles */
.ai-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 4px;
}

.ai-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.ai-logo {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  flex-shrink: 0;
}

.ai-title-info {
  flex: 1;
  min-width: 0;
}

.ai-status {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 600;
  color: var(--timo-text);
}

.ai-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.ai-status-dot.running {
  background: var(--timo-green);
  box-shadow: 0 0 6px var(--timo-green);
  animation: pulse-dot 1.5s ease-in-out infinite;
}

.ai-status-dot.attention {
  background: var(--timo-yellow);
  box-shadow: 0 0 6px var(--timo-yellow);
}

.ai-status-dot.done {
  background: var(--timo-red);
}

@keyframes pulse-dot {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.ai-topic {
  font-size: 12px;
  color: var(--timo-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
}

.ai-elapsed {
  font-size: 18px;
  font-weight: 700;
  font-family: 'SF Mono', 'Fira Code', monospace;
  color: var(--timo-text);
}

.ai-current {
  background: var(--timo-card-bg);
  border-radius: 8px;
  padding: 10px 12px;
}

.current-tool {
  display: flex;
  align-items: center;
  gap: 8px;
}

.current-tool .tool-icon {
  font-size: 16px;
}

.current-tool .tool-name {
  flex: 1;
  font-size: 13px;
  font-weight: 600;
  color: var(--timo-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.current-tool .tool-duration {
  font-size: 12px;
  color: var(--timo-gray);
  font-family: monospace;
}

.current-context {
  font-size: 12px;
  color: var(--timo-text-secondary);
  margin-top: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ai-subagent {
  background: rgba(34, 197, 94, 0.1);
  border: 1px solid rgba(34, 197, 94, 0.2);
  border-radius: 8px;
  padding: 10px 12px;
}

.subagent-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.subagent-icon {
  font-size: 16px;
}

.subagent-type {
  font-size: 13px;
  font-weight: 600;
  color: var(--timo-green);
}

.subagent-desc {
  font-size: 12px;
  color: var(--timo-text-secondary);
  margin-top: 4px;
}

.subagent-status {
  color: var(--timo-green);
  margin-left: 8px;
}

.subagent-result {
  font-size: 12px;
  color: var(--timo-text-secondary);
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px solid var(--timo-border);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.result-label {
  color: var(--timo-gray);
  margin-right: 4px;
}

.ai-history {
  border-top: 1px solid var(--timo-border);
  padding-top: 10px;
}

.history-title {
  font-size: 12px;
  color: var(--timo-gray);
  margin-bottom: 6px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.history-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.history-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--timo-text-secondary);
}

.history-icon {
  font-size: 12px;
  width: 16px;
  text-align: center;
}

.history-target {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.history-duration {
  font-size: 10px;
  color: var(--timo-gray);
  font-family: monospace;
}

.ai-stats {
  display: flex;
  gap: 16px;
  border-top: 1px solid var(--timo-border);
  padding-top: 10px;
}

.stat-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.stat-label {
  font-size: 10px;
  color: var(--timo-gray);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.stat-value {
  font-size: 12px;
  color: var(--timo-text);
  font-weight: 600;
}

.ai-attention {
  display: flex;
  align-items: center;
  gap: 12px;
  background: rgba(234, 179, 8, 0.1);
  border: 1px solid rgba(234, 179, 8, 0.2);
  border-radius: 8px;
  padding: 12px;
}

.attention-icon {
  font-size: 20px;
}

.attention-message {
  font-size: 13px;
  color: var(--timo-text);
}

.control-btn:focus-visible {
  outline: 2px solid var(--timo-green, #22c55e);
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
