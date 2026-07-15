<template>
  <Modal
    :visible="visible"
    title="闹钟管理"
    :icon="AlarmClock"
    closable
    @close="$emit('close')"
    @update:visible="(v) => !v && $emit('close')"
  >
    <!-- Loading state -->
    <div v-if="loading" class="panel-skeleton">
      <div class="skeleton-row" style="width: 80%"></div>
      <div class="skeleton-row" style="width: 60%"></div>
      <div class="skeleton-row" style="width: 70%"></div>
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="panel-error">
      <AlertTriangle :size="20" />
      <span>{{ error }}</span>
      <button class="btn btn-sm" @click="$emit('refresh')">重试</button>
    </div>

    <!-- Normal state -->
    <template v-else>
      <!-- ─── Alarm Start List ─── -->
      <div class="alarm-section">
        <div class="section-header">
          <h4><AlarmClock :size="16" /> 定时播放</h4>
          <button class="btn btn-sm" @click="$emit('create', 'alarm_start')">
            <Plus :size="14" /> 添加
          </button>
        </div>

        <!-- Empty state -->
        <div v-if="alarmStartList.length === 0" class="section-empty">
          <AlarmClock :size="32" />
          <p>还没有定时播放</p>
        </div>

        <!-- List -->
        <div v-else class="alarm-list">
          <div
            v-for="alarm in alarmStartList"
            :key="alarm.id"
            class="alarm-list-item"
            :class="{ 'item-missed': alarm.last_triggered_at && alarm.repeat === 'once', 'item-disabled': !alarm.enabled }"
          >
            <div class="item-left">
              <div class="item-primary">
                <span class="item-time">{{ formatCountdown(alarm) }}</span>
                <span class="item-repeat">{{ repeatLabel(alarm.repeat) }}</span>
              </div>
              <div class="item-secondary">
                <span v-if="alarm.auto_stop_mode !== 'no_limit'" class="item-tag">
                  {{ autoStopLabel(alarm) }}
                </span>
                <span class="item-tag">{{ conflictLabel(alarm.conflict_strategy) }}</span>
                <span v-if="alarm.fade_in_seconds > 0" class="item-tag">
                  淡入{{ alarm.fade_in_seconds }}s
                </span>
                <span v-if="alarm.last_triggered_at" class="item-tag tag-triggered">
                  已触发
                </span>
              </div>
            </div>
            <div class="item-actions">
              <!-- 已触发的单次闹钟：显示重置按钮代替开关 -->
              <template v-if="alarm.last_triggered_at && alarm.repeat === 'once'">
                <button class="btn-icon btn-icon-reset" @click="$emit('reset', alarm.id)" title="重新启用">
                  <RotateCcw :size="14" />
                </button>
              </template>
              <template v-else>
                <label class="toggle-switch" @click.stop>
                  <input
                    type="checkbox"
                    :checked="alarm.enabled"
                    @change="$emit('toggle', alarm.id)"
                  />
                  <span class="toggle-slider"></span>
                </label>
              </template>
              <button class="btn-icon" @click="$emit('edit', alarm)" title="编辑">
                <Pencil :size="14" />
              </button>
              <button class="btn-icon btn-icon-danger" @click="$emit('delete', alarm.id)" title="删除">
                <Trash2 :size="14" />
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Divider -->
      <div class="section-divider"></div>

      <!-- ─── Sleep Timer ─── -->
      <div class="alarm-section">
        <div class="section-header">
          <h4><Moon :size="16" /> 定时关闭</h4>
          <button v-if="!activeSleepTimer" class="btn btn-sm" @click="$emit('create', 'sleep_timer')">
            <Plus :size="14" /> 设置
          </button>
        </div>

        <!-- Empty state -->
        <div v-if="!activeSleepTimer" class="section-empty">
          <Moon :size="32" />
          <p>尚未设置定时关闭</p>
        </div>

        <!-- Active sleep timer -->
        <div v-else class="alarm-list">
          <div class="alarm-list-item">
            <div class="item-left">
              <div class="item-primary">
                <span class="item-time">{{ formatCountdown(activeSleepTimer) }}</span>
                <span class="item-repeat">{{ repeatLabel(activeSleepTimer.repeat) }}</span>
              </div>
              <div class="item-secondary">
                <span class="item-tag">{{ triggerModeLabel(activeSleepTimer.trigger_mode) }}</span>
                <span class="item-tag">{{ stopActionLabel(activeSleepTimer.stop_action) }}</span>
                <span v-if="activeSleepTimer.fade_out_seconds > 0" class="item-tag">
                  淡出{{ activeSleepTimer.fade_out_seconds }}s
                </span>
              </div>
            </div>
            <div class="item-actions">
              <label class="toggle-switch" @click.stop>
                <input
                  type="checkbox"
                  :checked="activeSleepTimer.enabled"
                  @change="$emit('toggle', activeSleepTimer.id)"
                />
                <span class="toggle-slider"></span>
              </label>
              <button class="btn-icon" @click="$emit('edit', activeSleepTimer)" title="编辑">
                <Pencil :size="14" />
              </button>
              <button class="btn-icon btn-icon-danger" @click="$emit('delete', activeSleepTimer.id)" title="删除">
                <Trash2 :size="14" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Alarm } from '../types'
import { AlarmClock, Moon, Plus, Pencil, Trash2, RotateCcw, AlertTriangle } from '@lucide/vue'
import Modal from './Modal.vue'

const props = defineProps<{
  visible: boolean
  alarms: Alarm[]
  loading: boolean
  error: string | null
}>()

defineEmits<{
  close: []
  create: [type: 'alarm_start' | 'sleep_timer']
  edit: [alarm: Alarm]
  delete: [id: string]
  toggle: [id: string]
  reset: [id: string]
  refresh: []
}>()

const alarmStartList = computed(() =>
  props.alarms
    .filter(a => a.type === 'alarm_start')
    .sort((a, b) => {
      // 已触发的单次闹钟排到末尾
      const aDone = a.last_triggered_at && a.repeat === 'once' ? 1 : 0
      const bDone = b.last_triggered_at && b.repeat === 'once' ? 1 : 0
      return aDone - bDone
    })
)

const activeSleepTimer = computed(() =>
  props.alarms.find(a => a.type === 'sleep_timer') || null
)

function repeatLabel(repeat: string): string {
  const labels: Record<string, string> = {
    once: '单次',
    daily: '每天',
    weekday: '工作日',
  }
  return labels[repeat] || repeat
}

function conflictLabel(strategy: string): string {
  const labels: Record<string, string> = {
    queue: '插队',
    replace: '替换',
    skip: '跳过',
  }
  return labels[strategy] || strategy
}

function autoStopLabel(alarm: Alarm): string {
  if (alarm.auto_stop_mode === 'play_time') return `播${alarm.auto_stop_value}分钟`
  if (alarm.auto_stop_mode === 'song_count') return `播${alarm.auto_stop_value}首`
  return ''
}

function triggerModeLabel(mode: string): string {
  const labels: Record<string, string> = {
    at_time: '指定时刻',
    countdown: '倒计时',
    song_count: '播放N首',
  }
  return labels[mode] || mode
}

function stopActionLabel(action: string): string {
  const labels: Record<string, string> = {
    stop: '停止播放',
    pause: '暂停',
    stop_and_clear: '停止并清空',
  }
  return labels[action] || action
}

function formatCountdown(alarm: Alarm): string {
  if (alarm.trigger_mode === 'countdown') {
    return `${alarm.countdown_minutes}分钟后`
  }
  if (alarm.trigger_mode === 'song_count') {
    return `${alarm.song_count}首歌后`
  }
  return alarm.trigger_time || '--:--'
}
</script>

<style scoped>
/* Skeleton loading */
.panel-skeleton {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 20px 0;
}
.skeleton-row {
  height: 20px;
  background: var(--border);
  border-radius: var(--radius-sm);
  animation: shimmer 1.5s infinite;
}
@keyframes shimmer {
  0% { opacity: 0.5; }
  50% { opacity: 1; }
  100% { opacity: 0.5; }
}

/* Error */
.panel-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px;
  color: var(--warning);
  font-size: 0.85rem;
}

/* Sections */
.alarm-section {
  margin-bottom: 8px;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.section-header h4 {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.9rem;
  font-weight: 600;
  margin: 0;
}

.section-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 20px;
  color: var(--text-secondary);
  font-size: 0.85rem;
}

.section-divider {
  height: 1px;
  background: var(--border);
  margin: 16px 0;
}

/* Alarm list */
.alarm-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.alarm-list-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
  transition: background 0.15s;
  gap: 8px;
}

.alarm-list-item:hover {
  background: var(--bg-card);
}

.alarm-list-item.item-missed {
  opacity: 0.5;
}

.alarm-list-item.item-disabled {
  opacity: 0.4;
}

.item-left {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.item-primary {
  display: flex;
  align-items: center;
  gap: 8px;
}

.item-time {
  font-size: 1rem;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--accent);
}

.item-repeat {
  font-size: 0.75rem;
  color: var(--text-secondary);
  background: var(--bg-card);
  padding: 1px 6px;
  border-radius: 10px;
}

.item-secondary {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.item-tag {
  font-size: 0.7rem;
  color: var(--text-secondary);
  padding: 1px 6px;
  background: var(--bg-card);
  border-radius: 8px;
}

.tag-triggered {
  color: var(--text-secondary);
}

.item-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

/* Toggle switch */
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 36px;
  height: 20px;
  cursor: pointer;
}

.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.toggle-slider {
  position: absolute;
  inset: 0;
  background: var(--border);
  border-radius: 20px;
  transition: background 0.2s;
}

.toggle-slider::before {
  content: '';
  position: absolute;
  width: 16px;
  height: 16px;
  left: 2px;
  bottom: 2px;
  background: white;
  border-radius: 50%;
  transition: transform 0.2s;
}

.toggle-switch input:checked + .toggle-slider {
  background: var(--accent);
}

.toggle-switch input:checked + .toggle-slider::before {
  transform: translateX(16px);
}

/* Icon button */
.btn-icon {
  background: none;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 6px;
  border-radius: var(--radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s;
  min-width: 32px;
  min-height: 32px;
}

.btn-icon:hover {
  background: var(--bg-card);
  color: var(--text-primary);
}

.btn-icon-danger:hover {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.btn-icon-reset:hover {
  background: rgba(52, 211, 153, 0.15);
  color: #34d399;
}


</style>
