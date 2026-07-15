<template>
  <!-- Assign Player Modal -->
  <Modal
    :visible="visible"
    title="选择播放器"
    :icon="Plus"
    :max-width="'480px'"
    closable
    @close="$emit('update:visible', false)"
    @update:visible="(v) => $emit('update:visible', v)"
  >
    <!-- Local Player Toggle -->
    <div class="local-player-section">
      <div class="local-player-header">
        <span class="local-player-icon"><Volume2 :size="18" /></span>
        <div class="local-player-info">
          <span class="local-player-title">本地播放器</span>
          <span class="local-player-desc">开启后在本浏览器中播放</span>
        </div>
        <label class="toggle-switch">
          <input type="checkbox" :checked="localPlayerEnabled" @change="$emit('toggleLocalPlayer')" />
          <span class="toggle-slider"></span>
        </label>
      </div>
    </div>

    <div class="modal-divider"></div>

    <!-- Player List -->
    <div v-if="allPlayers.length === 0" class="empty-text">
      暂无注册的播放器。请先在播放器页面注册。
    </div>

    <div v-else class="assign-player-list">
      <div
        v-for="player in sortedPlayers"
        :key="player.id"
        class="assign-player-item"
      >
        <div class="assign-player-info">
          <div class="assign-player-header">
            <span :class="['badge', player.online ? 'badge-online' : 'badge-offline']">
              {{ player.online ? '在线' : '离线' }}
            </span>
            <span class="assign-player-name">{{ player.name || player.id }}</span>
          </div>
        </div>

        <!-- Player actions: volume (if in channel) + toggle -->
        <div class="assign-player-actions">
          <button
            v-if="player.channel === channelName"
            class="btn btn-sm"
            :disabled="!player.online"
            @click="openPlayerVolumeModal(player)"
            title="调节音量"
          >
            <Volume2 :size="16" />
          </button>
          <label class="toggle-switch toggle-sm">
            <input
              type="checkbox"
              :checked="player.channel === channelName"
              :disabled="!player.online"
              @change="player.channel === channelName ? $emit('removePlayerFromChannel', player.id) : $emit('assignPlayer', player.id)"
            />
            <span class="toggle-slider"></span>
          </label>
        </div>
      </div>
    </div>
  </Modal>

  <!-- Player Volume Modal (sub-modal) -->
  <Modal
    :visible="showVolumeModal"
    title="调节音量"
    :icon="Volume2"
    :max-width="'360px'"
    show-footer
    closable
    @close="showVolumeModal = false"
    @update:visible="(v) => showVolumeModal = v"
  >
    <div class="volume-control">
      <label>{{ volumePlayerName }}</label>
      <div class="volume-slider-row">
        <input
          type="range"
          min="0"
          max="100"
          v-model.number="volumeValue"
        />
        <span class="volume-value">{{ volumeValue }}%</span>
      </div>
    </div>

    <template #footer>
      <button class="btn" @click="showVolumeModal = false">取消</button>
      <button class="btn btn-primary" @click="confirmVolume">确定</button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import type { PlayerInfo, PlayerState } from '../types'
import { Plus, Volume2 } from '@lucide/vue'
import Modal from './Modal.vue'

const props = defineProps<{
  visible: boolean
  allPlayers: PlayerInfo[]
  channelName: string
  localPlayerEnabled: boolean
  channelPlayers: PlayerState[]
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  assignPlayer: [playerId: string]
  removePlayerFromChannel: [playerId: string]
  toggleLocalPlayer: []
  setPlayerVolume: [playerId: string, volume: number]
}>()

// Sort players: in current channel first, then online, then by name
const sortedPlayers = computed(() => {
  return [...props.allPlayers].sort((a, b) => {
    const aInChannel = a.channel === props.channelName
    const bInChannel = b.channel === props.channelName
    if (aInChannel && !bInChannel) return -1
    if (!aInChannel && bInChannel) return 1
    if (a.online && !b.online) return -1
    if (!a.online && b.online) return 1
    return (a.name || a.id).localeCompare(b.name || b.id)
  })
})

// ─── Player Volume Sub-modal ──────────────────────────
const showVolumeModal = ref(false)
const volumePlayerId = ref('')
const volumePlayerName = ref('')
const volumeValue = ref(80)

function openPlayerVolumeModal(player: PlayerInfo) {
  volumePlayerId.value = player.id
  volumePlayerName.value = player.name || player.id
  const channelPlayer = props.channelPlayers.find(p => p.id === player.id)
  volumeValue.value = channelPlayer?.volume ?? 80
  showVolumeModal.value = true
}

function confirmVolume() {
  emit('setPlayerVolume', volumePlayerId.value, volumeValue.value)
  showVolumeModal.value = false
}
</script>

<style scoped>
.empty-text {
  text-align: center;
  color: var(--text-secondary);
  padding: 16px;
  font-size: 0.9rem;
}

/* Assign player list */
.assign-player-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.assign-player-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
}

.assign-player-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.assign-player-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.assign-player-name {
  font-weight: 500;
  font-size: 0.9rem;
  word-break: break-all;
}

.assign-player-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.assign-player-actions .btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* Local player section */
.local-player-section {
  padding: 12px 14px;
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
  margin-bottom: 12px;
}

.local-player-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.local-player-icon {
  color: var(--accent);
  flex-shrink: 0;
}

.local-player-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.local-player-title {
  font-weight: 500;
  font-size: 0.9rem;
}

.local-player-desc {
  font-size: 0.75rem;
  color: var(--text-secondary);
}

/* Toggle switch */
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
  flex-shrink: 0;
}

.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.toggle-slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--border);
  transition: 0.3s;
  border-radius: 24px;
}

.toggle-slider:before {
  position: absolute;
  content: "";
  height: 18px;
  width: 18px;
  left: 3px;
  bottom: 3px;
  background-color: white;
  transition: 0.3s;
  border-radius: 50%;
}

.toggle-switch input:checked + .toggle-slider {
  background-color: var(--accent);
}

.toggle-switch input:checked + .toggle-slider:before {
  transform: translateX(20px);
}

.toggle-sm {
  width: 36px;
  height: 20px;
}

.toggle-sm .toggle-slider:before {
  height: 14px;
  width: 14px;
}

.toggle-sm input:checked + .toggle-slider:before {
  transform: translateX(16px);
}

.toggle-switch input:disabled + .toggle-slider {
  opacity: 0.4;
  cursor: not-allowed;
}

.toggle-switch input:disabled + .toggle-slider:before {
  cursor: not-allowed;
}

/* Modal divider */
.modal-divider {
  height: 1px;
  background: var(--border);
  margin: 8px 0;
}

/* Volume control */
.volume-control {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.volume-control label {
  font-size: 0.9rem;
  font-weight: 500;
  color: var(--text-primary);
}

.volume-slider-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.volume-slider-row input[type="range"] {
  flex: 1;
}

.volume-value {
  min-width: 40px;
  text-align: right;
  font-size: 0.9rem;
  color: var(--text-primary);
  font-variant-numeric: tabular-nums;
}
</style>
