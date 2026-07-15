<template>
  <Modal
    :visible="visible"
    title="添加到频道"
    :icon="ListMusic"
    closable
    max-width="420px"
    @close="$emit('update:visible', false)"
    @update:visible="(v) => $emit('update:visible', v)"
  >
    <div class="channel-select-list">
      <div v-if="loading" class="loading">加载中...</div>
      <div v-else-if="channels.length === 0" class="empty">暂无其他频道</div>
      <template v-else>
        <div
          v-for="ch in channels"
          :key="ch.name"
          :class="['channel-option', { selected: selectedChannel === ch.name }]"
          @click="selectedChannel = ch.name"
        >
          <div class="channel-info">
            <div class="channel-name">{{ ch.display_name || ch.name }}</div>
            <div class="channel-meta" v-if="ch.player_count > 0">{{ ch.player_count }} 个播放器</div>
          </div>
          <div v-if="selectedChannel === ch.name" class="check-icon">
            <Check :size="18" />
          </div>
        </div>
      </template>
    </div>

    <template #footer>
      <button class="btn" @click="$emit('update:visible', false)">取消</button>
      <button
        class="btn btn-primary"
        :disabled="!selectedChannel"
        @click="onConfirm"
      >
        确认
      </button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { ListMusic, Check } from '@lucide/vue'
import Modal from './Modal.vue'

interface ChannelItem {
  name: string
  display_name: string
  player_count: number
}

const props = defineProps<{
  visible: boolean
  currentChannel: string
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  confirm: [channelName: string]
}>()

const channels = ref<ChannelItem[]>([])
const selectedChannel = ref('')
const loading = ref(false)

const STORAGE_KEY = 'lastAddToChannel'

async function fetchChannels() {
  loading.value = true
  try {
    const resp = await fetch('/api/channels')
    const data = await resp.json()
    // Filter out current channel
    channels.value = (data || []).filter((ch: ChannelItem) => ch.name !== props.currentChannel)
  } catch (e) {
    console.error('Failed to fetch channels:', e)
    channels.value = []
  } finally {
    loading.value = false
  }
}

function onConfirm() {
  if (!selectedChannel.value) return
  // Save to localStorage
  localStorage.setItem(STORAGE_KEY, selectedChannel.value)
  emit('confirm', selectedChannel.value)
  emit('update:visible', false)
}

watch(() => props.visible, (val) => {
  if (val) {
    selectedChannel.value = localStorage.getItem(STORAGE_KEY) || ''
    fetchChannels()
  }
})
</script>

<style scoped>
.channel-select-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 50vh;
  overflow-y: auto;
}

.loading,
.empty {
  text-align: center;
  color: var(--text-secondary);
  padding: 20px;
  font-size: 0.9rem;
}

.channel-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.15s ease;
  border: 1px solid transparent;
}

.channel-option:hover {
  background: var(--bg-card);
}

.channel-option.selected {
  border-color: var(--accent);
  background: rgba(233, 69, 96, 0.08);
}

.channel-info {
  flex: 1;
  min-width: 0;
}

.channel-name {
  font-size: 0.9rem;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.channel-meta {
  font-size: 0.75rem;
  color: var(--text-secondary);
  margin-top: 2px;
}

.check-icon {
  color: var(--accent);
  flex-shrink: 0;
  margin-left: 8px;
}
</style>
