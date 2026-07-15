<template>
  <div class="card channel-header-card">
    <div class="channel-header">
      <div class="channel-header-top">
        <h2 v-if="!isEditingName">
          <Radio :size="20" /> {{ displayName || channelName }}
          <button class="btn btn-sm" @click="startEditName" title="修改频道名称">
            <Pen :size="14" />
          </button>
        </h2>
        <div v-else class="edit-name-form">
          <input
            v-model="editingName"
            type="text"
            ref="nameInputRef"
            @keyup.enter="saveName"
            @keyup.escape="cancelEditName"
            @blur="saveName"
          />
          <button class="btn btn-sm btn-primary" @click="saveName">保存</button>
          <button class="btn btn-sm" @click="cancelEditName">取消</button>
        </div>
        <button class="btn" @click="$emit('leave')">退出</button>
      </div>
      <div class="channel-header-meta">
        <button class="device-selector" @click="$emit('openAssignPlayer')">
          <Volume2 :size="16" />
          <span class="device-count">{{ onlinePlayerCount }}</span>
          <span class="device-label">个设备</span>
        </button>
        <div class="header-alarm-area">
          <CountdownBadge
            :alarm-timer="alarmTimer"
            :sleep-timer="sleepTimer"
            @open-alarm-panel="$emit('openAlarmPanel')"
          />
          <span v-if="alarmCount > 0" class="alarm-count-badge">{{ alarmCount }} 个闹钟</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick } from 'vue'
import { Radio, Pen, Plus, AlarmClock, Volume2 } from '@lucide/vue'
import CountdownBadge from './CountdownBadge.vue'
import type { CountdownBadgeTimer } from '../types'

const props = defineProps<{
  channelName: string
  displayName: string
  onlinePlayerCount: number
  alarmTimer: CountdownBadgeTimer | null
  sleepTimer: CountdownBadgeTimer | null
  alarmCount: number
}>()

const emit = defineEmits<{
  'update:displayName': [name: string]
  leave: []
  openAssignPlayer: []
  openAlarmPanel: []
}>()

// ─── Name editing ─────────────────────────────────────
const isEditingName = ref(false)
const editingName = ref('')
const nameInputRef = ref<HTMLInputElement | null>(null)

function startEditName() {
  editingName.value = props.displayName || props.channelName
  isEditingName.value = true
  nextTick(() => {
    nameInputRef.value?.focus()
    nameInputRef.value?.select()
  })
}

async function saveName() {
  const newName = editingName.value.trim()
  if (!newName || newName === (props.displayName || props.channelName)) {
    isEditingName.value = false
    return
  }
  try {
    const resp = await fetch(`/api/channels/${encodeURIComponent(props.channelName)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ display_name: newName }),
    })
    if (resp.ok) {
      emit('update:displayName', newName)
      isEditingName.value = false
    }
  } catch (e) {
    console.error('Failed to update channel name:', e)
  }
}

function cancelEditName() {
  isEditingName.value = false
  editingName.value = ''
}
</script>

<style scoped>
.channel-header-card {
  flex-shrink: 0;
  margin-top: 10px;
  margin-bottom: 0;
}

.channel-header-card .channel-header {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.channel-header-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.channel-header h2 {
  color: var(--accent);
  margin: 0;
  display: flex;
  align-items: center;
  gap: 6px;
}

.edit-name-form {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
}

.edit-name-form input {
  max-width: 300px;
  padding: 6px 10px;
  font-size: 0.95rem;
}

.channel-header-meta {
  display: flex;
  align-items: center;
  gap: 12px;
}

.device-selector {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 16px;
  color: var(--text-primary);
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s;
  white-space: nowrap;
}

.device-selector:hover {
  border-color: var(--accent);
  background: var(--bg-primary);
}

.device-count {
  font-weight: 600;
  color: var(--accent);
}

.device-label {
  color: var(--text-secondary);
}

.header-alarm-area {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
}

.alarm-count-badge {
  font-size: 0.75rem;
  color: var(--text-secondary);
  background: var(--bg-card);
  padding: 2px 10px;
  border-radius: 12px;
  white-space: nowrap;
}
</style>
