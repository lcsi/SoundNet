<template>
  <div class="player-list">
    <div v-if="players.length === 0" class="empty">暂无播放器</div>
    <div
      v-for="player in players"
      :key="player.id"
      class="player-item"
    >
      <div class="player-header">
        <span :class="['badge', player.online ? 'badge-online' : 'badge-offline']">
          {{ player.online ? '在线' : '离线' }}
        </span>
        <span class="player-name">{{ player.name || player.id }}</span>
        <span v-if="player.playing" class="playing-indicator"><Play :size="12" /></span>
      </div>
      <div class="player-meta" v-if="player.note">
        <span class="meta-note">{{ player.note }}</span>
      </div>
      <div class="player-volume">
        <span class="vol-label">音量:</span>
        <input
          type="range"
          min="0"
          max="100"
          :value="player.volume"
          @input="setVolume(player.id, Number(($event.target as HTMLInputElement).value))"
        />
        <span class="vol-value">{{ player.volume }}%</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { PlayerState } from '../types'
import { Play } from '@lucide/vue'

defineProps<{
  players: PlayerState[]
}>()

const emit = defineEmits<{
  'set-volume': [playerId: string, volume: number]
}>()

function setVolume(playerId: string, volume: number) {
  emit('set-volume', playerId, volume)
}
</script>

<style scoped>
.player-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.empty {
  text-align: center;
  color: var(--text-secondary);
  padding: 12px;
  font-size: 0.9rem;
}

.player-item {
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
  padding: 10px 12px;
}

.player-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.player-name {
  font-weight: 500;
  font-size: 0.9rem;
  flex: 1;
}

.playing-indicator {
  color: var(--success);
  animation: pulse 1s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.player-meta {
  font-size: 0.8rem;
  color: var(--text-secondary);
  margin-bottom: 6px;
}

.player-volume {
  display: flex;
  align-items: center;
  gap: 8px;
}

.vol-label {
  font-size: 0.8rem;
  color: var(--text-secondary);
  min-width: 35px;
}

.player-volume input[type="range"] {
  flex: 1;
  height: 3px;
}

.vol-value {
  font-size: 0.75rem;
  color: var(--text-secondary);
  min-width: 35px;
  text-align: right;
}
</style>
