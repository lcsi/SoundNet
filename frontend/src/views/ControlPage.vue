<template>
  <div class="control-page">
    <!-- Connection status -->
    <div class="connection-status" :class="{ connected }">
      <span class="status-indicator"></span>
      {{ connected ? '已连接' : '重新连接中...' }}
    </div>

    <!-- ===== CHANNEL LIST ===== -->
    <div class="channel-select">
      <!-- Join/Create channel -->
      <div class="card">
        <h2 class="card-title"><Radio :size="20" /> 加入频道</h2>
        <div class="channel-form">
          <input
            v-model="channelInput"
            type="text"
            placeholder="输入频道名称..."
            @keyup.enter="joinChannel"
          />
          <button class="btn btn-primary" @click="joinChannel" :disabled="!channelInput.trim()">
            加入 / 创建
          </button>
        </div>
      </div>

      <!-- Active channels list -->
      <div class="card mt-4">
        <h2 class="card-title">
          <div><ListMusic :size="20" /> 活跃频道</div>
          <button class="btn btn-sm ml-auto" @click="fetchChannels">
          <RefreshCw :size="16" /> 刷新

        </button>
        </h2>
        <div v-if="channels.length === 0" class="empty-text">暂无活跃频道</div>
        <div v-else class="channel-list">
          <div
            v-for="ch in channels"
            :key="ch.name"
            class="channel-item clickable"
            @click="joinChannelByName(ch.name)"
          >
            <div class="channel-info">
              <span class="channel-name"># {{ ch.display_name || ch.name }}</span>
              <span class="channel-meta">{{ ch.player_count }} 个播放器</span>
            </div>
            <div class="channel-actions">
              <button class="btn btn-sm">加入</button>
              <button
                class="btn btn-sm btn-danger"
                :disabled="deletingChannel === ch.name"
                :title="'删除频道 ' + ch.name"
                @click.stop="deleteChannel(ch.name)"
              ><Trash2 :size="14" /></button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ===== PLAYER MANAGEMENT ===== -->
    <div class="card mt-4">
      <h2 class="card-title">
        <Monitor :size="20" /> 播放器
        <span class="badge badge-online">{{ onlinePlayers.length }} 在线</span>
        <span class="badge">{{ allPlayers.length }} 总计</span>
        <button class="btn btn-sm ml-auto" @click="fetchPlayers"><RefreshCw :size="16" /> 刷新</button>
      </h2>

      <div v-if="allPlayers.length === 0" class="empty-text">
        暂无注册的播放器。打开播放器页面 ({{ windowLocation }}/player) 即可自动注册
      </div>

      <div v-else class="player-mgmt-list">
        <div
          v-for="player in allPlayers"
          :key="player.id"
          class="player-mgmt-item"
        >
          <div class="player-mgmt-header">
            <span :class="['badge', player.online ? 'badge-online' : 'badge-offline']">
              {{ player.online ? '在线' : '离线' }}
            </span>
            <span
              class="player-mgmt-id clickable"
              :title="player.id + ' (点击配置)'"
              @click="openSettings(player)"
            >
              {{ player.name || player.id }}
            </span>

            <!-- Actions -->
            <div class="player-mgmt-actions">
              <button
                class="btn btn-sm btn-danger"
                :disabled="deletingPlayer === player.id"
                title="删除播放器"
                @click="deletePlayer(player.id)"
              ><Trash2 :size="14" /></button>
            </div>
          </div>

          <!-- Display area -->
          <div class="player-mgmt-info">
            <!--<div class="info-row">
              <span class="info-label">名称:</span>
              <span class="info-value">{{ player.name || '-' }}</span>
            </div>-->
            <div class="info-row">
              <span class="info-label">备注:</span>
              <span class="info-value">{{ player.note || '-' }}</span>
            </div>
            <div class="info-row" v-if="player.channel">
              <span class="info-label">频道:</span>
              <span class="channel-tag clickable" @click="joinChannelByName(player.channel)"># {{ player.channel }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Player Settings Modal -->
    <PlayerSettings
      v-model:visible="showSettings"
      :player="selectedPlayer"
      @saved="onSettingsSaved"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useWebSocket } from '../composables/useWebSocket'
import type { PlayerInfo, ChannelInfo, WSMessage } from '../types'
import { Actions } from '../types'
import { Radio, ListMusic, RefreshCw, Monitor, Trash2 } from '@lucide/vue'
import PlayerSettings from '../components/PlayerSettings.vue'

const {
  connected,
  connect: wsConnect,
  send,
  onMessage,
  log,
  logerror
} = useWebSocket()

const router = useRouter()

// --- Channel state ---
const channelInput = ref('')
const channels = ref<ChannelInfo[]>([])

// --- Player management state ---
const allPlayers = ref<PlayerInfo[]>([])

const onlinePlayers = computed(() => allPlayers.value.filter(p => p.online))

const windowLocation = window.location.origin

// --- Settings modal state ---
const showSettings = ref(false)
const selectedPlayer = ref<PlayerInfo | null>(null)

function openSettings(player: PlayerInfo) {
  selectedPlayer.value = player
  showSettings.value = true
}

function onSettingsSaved() {
  fetchPlayers() // Refresh player list
}

// --- Channel operations ---
function generateChannelId(): string {
  return 'ch-' + Math.random().toString(36).substring(2, 8)
}

function joinChannel() {
  const displayName = channelInput.value.trim()
  if (!displayName) return
  // Generate a random channel ID
  const channelId = generateChannelId()
  // Navigate with display name as query param
  router.push(`/channel/${encodeURIComponent(channelId)}?name=${encodeURIComponent(displayName)}`)
}

function joinChannelByName(name: string) {
  channelInput.value = ''
  router.push(`/channel/${encodeURIComponent(name)}`)
}

async function fetchChannels() {
  try {
    const resp = await fetch('/api/channels')
    channels.value = await resp.json()
  } catch (e) {
    logerror('Failed to fetch channels:', e)
  }
}

// --- Player management operations ---
async function fetchPlayers() {
  try {
    const resp = await fetch('/api/players')
    allPlayers.value = await resp.json()
  } catch (e) {
    logerror('Failed to fetch players:', e)
  }
}

// ─── Channel deletion ─────────────────────────────────
const deletingChannel = ref<string | null>(null)

async function deleteChannel(name: string) {
  if (!confirm(`确定删除频道 "#${name}" ？将清空队列并断开所有播放器和控制器连接。`)) return
  deletingChannel.value = name
  try {
    await fetch(`/api/channels/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    })
    // Remove locally immediately for responsive UI
    channels.value = channels.value.filter(ch => ch.name !== name)
  } catch (e) {
    logerror('Failed to delete channel:', e)
  } finally {
    deletingChannel.value = null
  }
}

// ─── Player deletion ──────────────────────────────────
const deletingPlayer = ref<string | null>(null)

async function deletePlayer(id: string) {
  if (!confirm(`确定删除播放器 "${id}" ？`)) return
  deletingPlayer.value = id
  try {
    await fetch(`/api/players/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
    // Remove locally immediately for responsive UI
    allPlayers.value = allPlayers.value.filter(p => p.id !== id)
  } catch (e) {
    logerror('Failed to delete player:', e)
  } finally {
    deletingPlayer.value = null
  }
}



onMounted(() => {
  wsConnect('control')
  fetchChannels()
  fetchPlayers()

  // --- Real-time updates via WebSocket ---

  // System-level broadcasts (player/channel list changes)
  onMessage('system', 'players_update', (msg: WSMessage) => {
    log('[Control] Players update received:', msg.payload)
    allPlayers.value = msg.payload as PlayerInfo[]
  })

  onMessage('system', 'channels_update', (msg: WSMessage) => {
    log('[Control] Channels update received:', msg.payload)
    channels.value = msg.payload as ChannelInfo[]
  })
})
</script>

<style scoped>
.control-page {
  position: relative;
  padding-bottom: 100px;
}

/* Connection status */
.connection-status {
  position: fixed;
  bottom: 16px;
  right: 16px;
  padding: 8px 16px;
  border-radius: var(--radius-sm);
  background: rgba(239, 68, 68, 0.9);
  color: white;
  font-size: 0.8rem;
  font-weight: 500;
  z-index: 100;
  display: flex;
  align-items: center;
  gap: 6px;
}
.connection-status.connected {
  background: rgba(74, 222, 128, 0.9);
}

.status-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.8;
}

/* Channel select */
.channel-form {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
.channel-form input {
  flex: 1;
}

.empty-text {
  text-align: center;
  color: var(--text-secondary);
  padding: 16px;
  font-size: 0.9rem;
}

.channel-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 50vh;
  overflow-y: auto;
}

.channel-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
  transition: background 0.2s;
}
.channel-item.clickable {
  cursor: pointer;
}
.channel-item.clickable:hover {
  background: var(--bg-card);
}

.channel-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.channel-name {
  font-weight: 600;
  color: var(--accent);
}
.channel-meta {
  color: var(--text-secondary);
  font-size: 0.85rem;
}

.channel-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.btn-danger {
  color: #ef4444;
}
.btn-danger:hover {
  background: #ef4444;
  color: white;
}

/* Player management */
.player-mgmt-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 50vh;
  overflow-y: auto;
}

.player-mgmt-item {
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
  padding: 12px;
}

.player-mgmt-header {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.player-mgmt-id {
  font-weight: 500;
  font-size: 0.9rem;
  word-break: break-all;
}

.player-mgmt-id.clickable {
  cursor: pointer;
  color: var(--accent);
  text-decoration: none;
  transition: opacity 0.2s;
}

.player-mgmt-id.clickable:hover {
  opacity: 0.8;
  text-decoration: underline;
}

/* Player info display */
.player-mgmt-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-left: 4px;
  flex: 1;
  min-width: 0;
}

.player-mgmt-actions {
  position: absolute;
  right: 10px;
  display: flex;
  align-items: flex-start;
  gap: 4px;
  flex-shrink: 0;
}

.info-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.85rem;
}

.info-label {
  color: var(--text-secondary);
  min-width: 42px;
}

.info-value {
  color: var(--text-primary);
}

.channel-tag {
  color: var(--accent);
  font-size: 0.85rem;
  font-weight: 500;
}

</style>
