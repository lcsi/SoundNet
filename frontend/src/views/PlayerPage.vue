<template>
  <div class="player-page">
    <div class="card">
      <h2 class="card-title"><Volume2 :size="20" /> 播放器</h2>

      <div class="player-info">
        <div class="info-row">
          <span class="label">ID:</span>
          <span class="value id-value">{{ playerId }}</span>
          <!-- <button class="btn btn-sm" @click="copyId">📋 复制</button> -->
        </div>
        <div class="info-row">
          <span class="label">状态:</span>
          <span :class="['badge', connected ? 'badge-online' : 'badge-offline']">
            {{ connected ? '已连接' : '未连接' }}
          </span>
        </div>
        <div class="info-row" v-if="currentChannel">
          <span class="label">频道:</span>
          <span class="value" style="color: var(--accent);"># {{ currentChannel }}</span>
        </div>
      </div>

      <div class="now-playing" v-if="currentSong">
        <div class="song-info">
          <div class="song-cover" v-if="currentSong.cover">
            <img :src="currentSong.cover" :alt="currentSong.title" />
          </div>
          <div class="song-details">
            <div class="song-title">{{ currentSong.title }}</div>
            <div class="song-artist">{{ currentSong.artist }}</div>
          </div>
        </div>

        <div class="progress-bar">
          <div class="progress-fill" :style="{ width: progressPercent + '%' }"></div>
        </div>
        <div class="progress-time">
          <span>{{ formatTime(currentProgress) }}</span>
          <span>{{ formatTime(currentSong.duration) }}</span>
        </div>
      </div>

      <div class="no-song" v-else>
        <p>等待播放指令...</p>
      </div>

      <div class="player-status">
        <span :class="['status-dot', audioState === 'playing' ? 'playing' : 'paused']"></span>
        {{ audioState === 'playing' ? '播放中' : audioState === 'paused' ? '已暂停' : '已停止' }}
      </div>
    </div>

    <div class="card">
      <h2 class="card-title"><Settings :size="20" /> 调试信息</h2>
      <pre class="debug">{{ debugInfo }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useWebSocket } from '../composables/useWebSocket'
import type { Song, WSMessage } from '../types'
import { Actions } from '../types'
import { Volume2, Settings } from '@lucide/vue'

// Generate a random player ID
function generateId(): string {
  return 'player-' + Math.random().toString(36).substring(2, 10) +
    '-' + Math.random().toString(36).substring(2, 6)
}

// Persist player ID in localStorage so it survives page refreshes
function getOrCreatePlayerId(): string {
  const stored = localStorage.getItem('player_id')
  if (stored) return stored
  const id = generateId()
  localStorage.setItem('player_id', id)
  return id
}

const playerId = ref(getOrCreatePlayerId())

const { ws, connected, connect, send, onMessage, log, logerror } = useWebSocket()

const audioState = ref<'stopped' | 'playing' | 'paused'>('stopped')
const currentSong = ref<Song | null>(null)
const currentProgress = ref(0)
const currentChannel = ref('')
const volume = ref(80)
const reportingEnabled = ref(false)

let audioElement: HTMLAudioElement | null = null
let progressInterval: number | null = null

const progressPercent = computed(() => {
  if (!currentSong.value || currentSong.value.duration === 0) return 0
  return (currentProgress.value / currentSong.value.duration) * 100
})

const debugInfo = computed(() => {
  return JSON.stringify({
    id: playerId.value,
    connected: connected.value,
    channel: currentChannel.value,
    state: audioState.value,
    song: currentSong.value?.title || 'none',
    progress: currentProgress.value,
    volume: volume.value,
  }, null, 2)
})

function formatTime(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

function copyId() {
  navigator.clipboard.writeText(playerId.value)
}

// Audio control functions
async function fetchSongUrl(song: Song): Promise<string> {
  // If the song already has a URL, use it directly
  if (song.url) return song.url
  // Otherwise, fetch the real audio URL from the API
  // &quality=128k
  const resp = await fetch(`/api/song/url?source=${encodeURIComponent(song.source?song.source:'kuwo')}&musicId=${encodeURIComponent(song.id)}`)
  const data = await resp.json()
  return data.url || ''
}

async function playSong(song: Song) {
  if (!audioElement) {
    audioElement = new Audio()
    audioElement.onended = () => {
      audioState.value = 'stopped'
      currentProgress.value = 0
      send({
        type: 'player',
        action: Actions.FINISHED,
        payload: {},
      })
    }
    audioElement.onerror = (e) => {
      const error = audioElement?.error
      logerror('[Player] Playback error:', error?.code, error?.message)
      audioState.value = 'stopped'
      currentProgress.value = 0
      // Notify server to play next song
      send({
        type: 'player',
        action: Actions.FINISHED, // 先用完成事件，会自动下一曲
        payload: {
          reason: 'play_error',
          error_code: error?.code,
          error_message: error?.message,
        },
      })
    }
    audioElement.ontimeupdate = () => {
      if (audioElement) {
        currentProgress.value = audioElement.currentTime
      }
    }
  }
  // Get the real audio URL
  const url = await fetchSongUrl(song)
  if (!url) {
    alert('[Player] No audio URL available for song:'+ song.title)
    return
  }
  audioElement.src = url
  audioElement.volume = volume.value / 100
  await audioElement.play()
  currentSong.value = song
  currentSong.value.url = url
  audioState.value = 'playing'
  currentProgress.value = 0

  // Start progress reporting
  startProgressReporting()
}

function startProgressReporting() {
  if (progressInterval) clearInterval(progressInterval)
  progressInterval = window.setInterval(() => {
    // Only send status update if reporting is enabled (when control clients are connected)
    if (!reportingEnabled.value) return

    send({
      type: 'player',
      action: Actions.STATUS_UPDATE,
      payload: {
        player_id: playerId.value,
        playing: audioState.value === 'playing',
        progress: currentProgress.value,
        volume: volume.value,
        current_song: currentSong.value ? {
          id: currentSong.value.id,
          title: currentSong.value.title,
          artist: currentSong.value.artist,
          source: currentSong.value.source,
          url: currentSong.value.url,
        } : null,
      },
    })
  }, 1000)
}

// Handle commands from platform
onMessage('command', '*', (msg: WSMessage) => {
  log('[Player] Received command:', msg.action, msg.payload)

  switch (msg.action) {
    case 'play':
    case 'resume':
      if (audioElement) {
        audioElement.play()
        audioState.value = 'playing'
      }
      break

    case 'pause':
      if (audioElement) {
        audioElement.pause()
        audioState.value = 'paused'
      }
      break

    case 'stop':
      if (audioElement) {
        audioElement.pause()
        audioElement.currentTime = 0
        audioState.value = 'stopped'
      }
      break

    case 'play_song':
      if (msg.payload) {
        const song = msg.payload as Song
        playSong(song).catch((error) => {
          logerror('[Player] Failed to play song:', error)
          // 播放器已有错误处理逻辑，这里不用处理
          // send({
          //   type: 'player',
          //   action: Actions.FINISHED,
          //   payload: {
          //     reason: 'play_error',
          //     error_code: error?.code,
          //     error_message: error?.message,
          //   },
          // })
        })
      }
      break

    case 'seek':
      if (audioElement && msg.payload) {
        const pos = (msg.payload as any).position
        if (typeof pos === 'number') {
          audioElement.currentTime = pos
        }
      }
      break

    case 'volume':
      if (msg.payload) {
        const vol = (msg.payload as any).volume
        if (typeof vol === 'number') {
          volume.value = vol
          if (audioElement) {
            audioElement.volume = vol / 100
          }
        }
      }
      break

    case 'join_channel':
      if (msg.payload) {
        currentChannel.value = (msg.payload as any).channel || ''
      }
      break

    case 'update_info':
      // Update player info (name, note) - displayed in control page
      break

    case 'set_reporting':
      if (msg.payload) {
        reportingEnabled.value = (msg.payload as any).enabled || false
        log('[Player] Reporting enabled:', reportingEnabled.value)
      }
      break
  }
})

onMounted(() => {
  connect('player')

  // Register with the platform once connected
  const checkConnection = setInterval(() => {
    if (connected.value) {
      log('## 注册 ##')
      send({
        type: 'player',
        action: Actions.REGISTER,
        payload: {
          player_id: playerId.value,
          name: playerId.value,
        },
      })
      clearInterval(checkConnection)
    }
  }, 500)
})

onUnmounted(() => {
  if (progressInterval) clearInterval(progressInterval)
  if (audioElement) {
    audioElement.pause()
    audioElement = null
  }
})

// Watch for reconnection: automatically re-register the player
watch(connected, (newVal, oldVal) => {
  if (newVal && !oldVal) {
    log('[Player] Reconnected, re-registering...')
    send({
      type: 'player',
      action: Actions.REGISTER,
      payload: {
        player_id: playerId.value,
        name: playerId.value,
      },
    })
  }
})
</script>

<style scoped>
.player-page {
  max-width: 500px;
  margin: 0 auto;
}

.player-info {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 16px;
}

.info-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.label {
  color: var(--text-secondary);
  font-size: 0.85rem;
  min-width: 50px;
}

.value {
  font-family: monospace;
  font-size: 0.9rem;
}

.id-value {
  font-size: 0.75rem;
  color: var(--text-secondary);
  word-break: break-all;
}

.now-playing {
  background: var(--bg-primary);
  border-radius: var(--radius);
  padding: 16px;
  margin: 16px 0;
}

.song-info {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.song-cover {
  width: 60px;
  height: 60px;
  border-radius: 8px;
  overflow: hidden;
  flex-shrink: 0;
}

.song-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.song-details {
  flex: 1;
  min-width: 0;
}

.song-title {
  font-weight: 600;
  font-size: 1rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.song-artist {
  color: var(--text-secondary);
  font-size: 0.85rem;
  margin-top: 4px;
}

.progress-bar {
  height: 4px;
  background: var(--border);
  border-radius: 2px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: var(--accent);
  transition: width 0.3s;
}

.progress-time {
  display: flex;
  justify-content: space-between;
  color: var(--text-secondary);
  font-size: 0.75rem;
  margin-top: 4px;
}

.no-song {
  text-align: center;
  padding: 20px;
  color: var(--text-secondary);
}

.player-status {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
  font-size: 0.9rem;
  color: var(--text-secondary);
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-secondary);
}

.status-dot.playing {
  background: var(--success);
  animation: pulse 1s infinite;
}

.status-dot.paused {
  background: var(--warning);
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.debug {
  background: var(--bg-primary);
  padding: 12px;
  border-radius: var(--radius-sm);
  font-size: 0.75rem;
  overflow-x: auto;
  color: var(--text-secondary);
}
</style>
