<template>
  <div class="channel-detail">
    <!-- Channel Header -->
    <ChannelHeader
      :channel-name="channelName"
      :display-name="displayName"
      :online-player-count="onlinePlayerCount"
      :alarm-timer="alarmTimerBadge"
      :sleep-timer="sleepTimerBadge"
      :alarm-count="alarms.length"
      @update:display-name="displayName = $event"
      @leave="leaveChannel"
      @open-assign-player="showAssignPlayerModal = true"
      @open-alarm-panel="showAlarmPanel = true"
    />

    <!-- Scrollable area (between header and bottom bar) -->
    <div class="scroll-area">
      <!-- Search -->
      <div class="card search-card">
        <button class="search-trigger" @click="showSearchModal = true">
          <Search :size="18" />
          <span>搜索歌曲或歌单...</span>
        </button>
      </div>

      <!-- Queue -->
      <div class="card queue-card">
        <h2 class="card-title">
          <ListMusic :size="20" /> 播放队列
          <span class="badge">{{ queue.length }} 首</span>
          <button class="btn btn-sm ml-auto" @click="clearQueue" title="清空队列">
            <Trash2 :size="16" /> 清空
          </button>
        </h2>
        <div class="queue-scroll-wrap">
          <PlayQueue
            :queue="queue"
            :current-index="currentIndex"
            @remove="removeFromQueue"
            @reorder="reorderQueue"
            @play-index="playIndex"
            @add-to-channel="openAddToChannelModal"
          />
        </div>
      </div>
    </div>

    <!-- Playback Bar -->
    <PlaybackBar
      :active-song="activeSong"
      :is-playing="isPlaying"
      :has-queue="hasQueue"
      :playback-mode="playbackMode"
      :master-volume="masterVolume"
      :player-progress="activePlayer?.progress || 0"
      :song-duration="songDuration"
      @prev="sendControl('prev')"
      @next="sendControl('next')"
      @toggle-play="togglePlay"
      @update:playback-mode="setPlaybackMode"
      @update:master-volume="setMasterVolume"
      @seek="(pos) => send({ type: 'control', action: Actions.SEEK, payload: { position: pos } })"
      @show-lyrics="showLyrics = true"
    />

    <!-- Search Modal -->
    <Modal
      :visible="showSearchModal"
      :max-width="'full'"
      closable
      @close="showSearchModal = false"
      @update:visible="(v) => showSearchModal = v"
    >
      <template #header>
        <div class="search-modal-header">
          <h3 class="modal-title"><Search :size="20" /> 搜索</h3>
          <div class="search-type-toggle-inline">
            <button
              class="type-btn-inline"
              :class="{ active: searchPanelType === 'song' }"
              @click="searchPanelType = 'song'"
            >
              <Music :size="14" /> 歌曲
            </button>
            <button
              class="type-btn-inline"
              :class="{ active: searchPanelType === 'playlist' }"
              @click="searchPanelType = 'playlist'"
            >
              <ListMusic :size="14" /> 歌单
            </button>
          </div>
          <button class="btn-close" @click="showSearchModal = false" aria-label="关闭">
            <X :size="20" />
          </button>
        </div>
      </template>
      <SearchPanel
        ref="searchPanelRef"
        :search-type="searchPanelType"
        @update:search-type="searchPanelType = $event"
        @add-to-queue="addToQueue"
        @play-now="playNow"
        @play-all="playAllSongs"
      />
    </Modal>

    <!-- Assign Player Modal -->
    <AssignPlayerModal
      :visible="showAssignPlayerModal"
      :all-players="allPlayers"
      :channel-name="channelName"
      :local-player-enabled="localPlayerEnabled"
      :channel-players="channelPlayers"
      @update:visible="showAssignPlayerModal = $event"
      @assign-player="assignPlayer"
      @remove-player-from-channel="removePlayerFromChannel"
      @toggle-local-player="toggleLocalPlayer"
      @set-player-volume="setPlayerVolume"
    />

    <!-- Lyric Display -->
    <LyricDisplay
      v-if="activeSong"
      :visible="showLyrics"
      :song-id="activeSong.id"
      :source="activeSong.source"
      :title="activeSong.title"
      :artist="activeSong.artist"
      :progress="activePlayer?.progress || 0"
      @close="showLyrics = false"
    />

    <!-- Alarm Panel -->
    <AlarmPanel
      :visible="showAlarmPanel"
      :alarms="alarms"
      :loading="alarmsLoading"
      :error="alarmsError"
      @close="showAlarmPanel = false"
      @create="openCreateAlarm"
      @edit="openEditAlarm"
      @delete="deleteAlarm"
      @toggle="toggleAlarm"
      @reset="resetAlarm"
      @refresh="fetchAlarms"
    />

    <!-- Alarm Form Modal -->
    <AlarmFormModal
      :visible="showAlarmForm"
      :channel-name="channelName"
      :editing-alarm="editingAlarm"
      :initial-type="alarmFormType"
      @save="onAlarmSave"
      @close="showAlarmForm = false"
    />

    <!-- Add to Channel Modal -->
    <AddToChannelModal
      :visible="showAddToChannelModal"
      :current-channel="channelName"
      @update:visible="showAddToChannelModal = $event"
      @confirm="onAddToChannelConfirm"
    />

    <!-- Toast -->
    <Toast
      ref="toastRef"
      :message="toastMessage"
      :type="toastType"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useWebSocket } from '../composables/useWebSocket'
import { useLocalPlayer } from '../composables/useLocalPlayer'
import type { Song, PlayerState, PlayerInfo, WSMessage, PlaybackMode, Alarm, ActiveTimer, CountdownBadgeTimer } from '../types'
import { Actions, PlaybackModes } from '../types'
import { ListMusic, Trash2, Search, Music, X } from '@lucide/vue'
import ChannelHeader from '../components/ChannelHeader.vue'
import PlayQueue from '../components/PlayQueue.vue'
import SearchPanel from '../components/SearchPanel.vue'
import LyricDisplay from '../components/LyricDisplay.vue'
import Modal from '../components/Modal.vue'
import PlaybackBar from '../components/PlaybackBar.vue'
import AssignPlayerModal from '../components/AssignPlayerModal.vue'
import AlarmPanel from '../components/AlarmPanel.vue'
import AlarmFormModal from '../components/AlarmFormModal.vue'
import AddToChannelModal from '../components/AddToChannelModal.vue'
import Toast from '../components/Toast.vue'

const router = useRouter()
const route = useRoute()

const channelName = computed(() => route.params.name as string)
const channelDisplayNameFromQuery = computed(() => (route.query.name as string) || '')

const {
  connected,
  connect: wsConnect,
  send,
  onMessage,
  log,
  logerror,
} = useWebSocket()

// ─── Local Player ─────────────────────────────────────
const { localPlayerEnabled, toggleLocalPlayer } = useLocalPlayer(channelName.value)

// ─── State ────────────────────────────────────────────
const showSearchModal = ref(false)
const searchPanelType = ref<'song' | 'playlist'>('song')
const showAssignPlayerModal = ref(false)
const searchPanelRef = ref<InstanceType<typeof SearchPanel> | null>(null)
const showLyrics = ref(false)
const showAddToChannelModal = ref(false)
const addToChannelSong = ref<Song | null>(null)
const toastMessage = ref('')
const toastType = ref<'success' | 'error' | 'info'>('success')
const toastRef = ref<InstanceType<typeof Toast> | null>(null)
const channelPlayers = ref<PlayerState[]>([])
const allPlayers = ref<PlayerInfo[]>([])
const displayName = ref('')

// ─── Alarm state ────────────────────────────────────────
const alarms = ref<Alarm[]>([])
const activeTimers = ref<ActiveTimer[]>([])
const showAlarmPanel = ref(false)
const showAlarmForm = ref(false)
const editingAlarm = ref<Alarm | null>(null)
const alarmFormType = ref<'alarm_start' | 'sleep_timer'>('alarm_start')
const alarmsLoading = ref(false)
const alarmsError = ref<string | null>(null)

const queue = ref<Song[]>([])
const playbackMode = ref<PlaybackMode>(PlaybackModes.LOOP)
const masterVolume = ref(80)

// ─── Computed ─────────────────────────────────────────
const currentIndex = computed(() => {
  const songId = activePlayer.value?.current_song?.id
  if (!songId || queue.value.length === 0) return -1
  return queue.value.findIndex(s => s.id === songId)
})

const isPlaying = computed(() => channelPlayers.value.some(p => p.playing))
const onlinePlayerCount = computed(() => channelPlayers.value.filter(p => p.online).length)
const hasQueue = computed(() => queue.value.length > 0)

const activePlayer = computed(() => channelPlayers.value.find(p => p.current_song) || null)

const activeSong = computed(() => {
  const player = activePlayer.value
  if (!player?.current_song) return null
  const song = { ...player.current_song }
  if (!song.cover || !song.duration || !song.source) {
    const found = queue.value.find(s => s.id === song.id)
    if (found) {
      if (!song.cover && found.cover) song.cover = found.cover
      if (!song.duration && found.duration) song.duration = found.duration
      if (!song.source && found.source) song.source = found.source
    }
  }
  return song
})

const songDuration = computed(() => activeSong.value?.duration || 0)

// ─── Alarm computed ─────────────────────────────────────
function formatTimerDisplay(seconds: number, triggerTime?: string): string {
  if (triggerTime) {
    return triggerTime
  }
  if (seconds <= 0) return '00:00'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) {
    return `${h}:${String(m).padStart(2, '0')}`
  }
  if (seconds > 300) {
    return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  }
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const alarmTimerBadge = computed<CountdownBadgeTimer | null>(() => {
  const timer = activeTimers.value.find(t => t.type === 'alarm_start')
  if (!timer) return null
  return {
    type: 'alarm_start',
    alarm_id: timer.alarm_id,
    remaining_seconds: timer.remaining_seconds,
    trigger_time: timer.trigger_time,
    display: formatTimerDisplay(timer.remaining_seconds, timer.trigger_time),
  }
})

const sleepTimerBadge = computed<CountdownBadgeTimer | null>(() => {
  const timer = activeTimers.value.find(t => t.type === 'sleep_timer')
  if (!timer) return null
  return {
    type: 'sleep_timer',
    alarm_id: timer.alarm_id,
    remaining_seconds: timer.remaining_seconds,
    trigger_time: timer.trigger_time,
    display: formatTimerDisplay(timer.remaining_seconds, timer.trigger_time),
  }
})

// ─── Control operations ───────────────────────────────
function sendControl(action: string) {
  send({ type: 'control', action, payload: {} })
}

function leaveChannel() {
  send({ type: 'control', action: Actions.LEAVE_CHANNEL })
  router.push('/')
}

function setPlaybackMode(mode: PlaybackMode) {
  playbackMode.value = mode
  send({ type: 'control', action: Actions.SET_PLAYBACK_MODE, payload: { mode } })
}

function setMasterVolume(volume: number) {
  masterVolume.value = volume
  send({ type: 'control', action: Actions.VOLUME, payload: { volume } })
}

function togglePlay() {
  if (isPlaying.value) sendControl('pause')
  else sendControl('resume')
}

function addToQueue(song: Song) {
  send({ type: 'control', action: Actions.ADD_TO_QUEUE, payload: { song } })
}

function playNow(song: Song) {
  addToQueue(song)
  send({ type: 'control', action: Actions.PLAY, payload: {} })
}

function playAllSongs(songs: Song[]) {
  send({ type: 'control', action: Actions.CLEAR_QUEUE, payload: {} })
  for (const song of songs) {
    send({ type: 'control', action: Actions.ADD_TO_QUEUE, payload: { song } })
  }
  send({ type: 'control', action: Actions.PLAY, payload: {} })
}

function removeFromQueue(index: number) {
  send({ type: 'control', action: Actions.REMOVE_FROM_QUEUE, payload: { index } })
}

function reorderQueue(from: number, to: number) {
  send({ type: 'control', action: Actions.REORDER_QUEUE, payload: { from, to } })
}

function playIndex(index: number) {
  send({ type: 'control', action: Actions.PLAY_INDEX, payload: { index } })
}

function clearQueue() {
  send({ type: 'control', action: Actions.CLEAR_QUEUE, payload: {} })
}

// ─── Add to Channel ──────────────────────────────────────
function openAddToChannelModal(song: Song) {
  addToChannelSong.value = song
  showAddToChannelModal.value = true
}

function onAddToChannelConfirm(targetChannel: string) {
  const song = addToChannelSong.value
  if (!song) return
  send({
    type: 'control',
    action: Actions.ADD_TO_QUEUE,
    payload: { song, channel: targetChannel },
  })
  const label = targetChannel
  toastMessage.value = `已添加到 ${label}`
  toastType.value = 'success'
  toastRef.value?.show()
  addToChannelSong.value = null
}

// ─── Alarm CRUD ─────────────────────────────────────────
async function fetchAlarms() {
  alarmsLoading.value = true
  alarmsError.value = null
  try {
    const resp = await fetch(`/api/alarms/channel/${encodeURIComponent(channelName.value)}`)
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    const data = await resp.json()
    alarms.value = data.alarms || []
    activeTimers.value = data.active_timers || []
  } catch (e: any) {
    alarmsError.value = '加载闹钟失败: ' + (e.message || '未知错误')
    console.error('[Alarm] Failed to fetch:', e)
  } finally {
    alarmsLoading.value = false
  }
}

async function createAlarm(alarm: Alarm) {
  try {
    const resp = await fetch(`/api/alarms/channel/${encodeURIComponent(channelName.value)}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(alarm),
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    await fetchAlarms()
    showAlarmForm.value = false
  } catch (e: any) {
    console.error('[Alarm] Failed to create:', e)
    alert('保存失败: ' + (e.message || '未知错误'))
  }
}

async function updateAlarm(alarm: Alarm) {
  try {
    const resp = await fetch(`/api/alarms/${encodeURIComponent(alarm.id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(alarm),
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    await fetchAlarms()
    showAlarmForm.value = false
  } catch (e: any) {
    console.error('[Alarm] Failed to update:', e)
    alert('保存失败: ' + (e.message || '未知错误'))
  }
}

async function deleteAlarm(id: string) {
  if (!confirm('确定删除这个闹钟？')) return
  try {
    const resp = await fetch(`/api/alarms/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    await fetchAlarms()
  } catch (e: any) {
    console.error('[Alarm] Failed to delete:', e)
    alert('删除失败: ' + (e.message || '未知错误'))
  }
}

async function toggleAlarm(id: string) {
  try {
    const resp = await fetch(`/api/alarms/${encodeURIComponent(id)}/toggle`, {
      method: 'POST',
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    await fetchAlarms()
  } catch (e: any) {
    console.error('[Alarm] Failed to toggle:', e)
  }
}

async function resetAlarm(id: string) {
  try {
    const resp = await fetch(`/api/alarms/${encodeURIComponent(id)}/reset`, {
      method: 'POST',
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    await fetchAlarms()
  } catch (e: any) {
    console.error('[Alarm] Failed to reset:', e)
  }
}

function openCreateAlarm(type: 'alarm_start' | 'sleep_timer') {
  alarmFormType.value = type
  editingAlarm.value = null
  showAlarmForm.value = true
}

function openEditAlarm(alarm: Alarm) {
  editingAlarm.value = alarm
  alarmFormType.value = alarm.type as 'alarm_start' | 'sleep_timer'
  showAlarmForm.value = true
}

function onAlarmSave(alarm: Alarm) {
  if (alarm.id) {
    updateAlarm(alarm)
  } else {
    createAlarm(alarm)
  }
}

// ─── Channel / Player API operations ──────────────────
async function fetchChannelState() {
  try {
    const resp = await fetch(`/api/channels/${encodeURIComponent(channelName.value)}`)
    const data = await resp.json()
    queue.value = data.queue || []
  } catch (e) {
    logerror('Failed to fetch channel state:', e)
  }
}

async function fetchAllPlayers() {
  try {
    const resp = await fetch('/api/players')
    allPlayers.value = await resp.json()
  } catch (e) {
    logerror('Failed to fetch players:', e)
  }
}

async function assignPlayer(playerId: string) {
  try {
    const resp = await fetch(`/api/players/${playerId}/assign-channel`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ channel: channelName.value }),
    })
    if (resp.ok) {
      fetchAllPlayers()
      showAssignPlayerModal.value = false
    }
  } catch (e) {
    logerror('Failed to assign player:', e)
  }
}

function removePlayerFromChannel(playerId: string) {
  send({
    type: 'control',
    action: Actions.REMOVE_PLAYER,
    payload: { player_id: playerId },
  })
  fetchAllPlayers()
}

function setPlayerVolume(playerId: string, volume: number) {
  send({
    type: 'control',
    action: Actions.SET_PLAYER_VOLUME,
    payload: { player_id: playerId, volume },
  })
}

function joinChannel() {
  const payload: any = { channel: channelName.value }
  if (channelDisplayNameFromQuery.value) {
    payload.display_name = channelDisplayNameFromQuery.value
  }
  send({ type: 'control', action: Actions.JOIN_CHANNEL, payload })
}

// ─── Lifecycle ────────────────────────────────────────
onMounted(() => {
  wsConnect('control')
  fetchAllPlayers()
  fetchAlarms()


  const unwatch = watch(connected, (val) => {
    if (val) {
      joinChannel()
      unwatch()
    }
  })
  if (connected.value) {
    joinChannel()
  }

  // Real-time updates via WebSocket
  onMessage('state_update', 'initial_state', (msg: WSMessage) => {
    log('[ChannelDetail] Initial state:', msg.payload)
    channelPlayers.value = msg.payload.players || []
    queue.value = msg.payload.queue || []
    if (msg.payload.playback_mode) {
      playbackMode.value = msg.payload.playback_mode
    }
    if (msg.payload.display_name) {
      displayName.value = msg.payload.display_name
    }
  })

  onMessage('state_update', 'alarm_sync', (msg: WSMessage) => {
    log('[ChannelDetail] Alarm sync:', msg.payload)
    if (msg.payload.alarms) {
      alarms.value = msg.payload.alarms
    }
    if (msg.payload.active_timers) {
      activeTimers.value = msg.payload.active_timers
    }
  })

  onMessage('state_update', 'player_update', (msg: WSMessage) => {
    const updated = msg.payload as PlayerState
    log('[ChannelDetail] Player update:', updated.id, updated.online)
    const idx = channelPlayers.value.findIndex(p => p.id === updated.id)
    if (idx >= 0) {
      channelPlayers.value[idx] = { ...channelPlayers.value[idx], ...updated }
    } else if (updated.online) {
      channelPlayers.value.push(updated)
    }
  })

  onMessage('state_update', 'player_progress', (msg: WSMessage) => {
    const { id, progress, playing } = msg.payload as { id: string; progress: number; playing: boolean }
    const player = channelPlayers.value.find(p => p.id === id)
    if (player) {
      player.progress = progress
      player.playing = playing
      player.online = true
    }
  })

  onMessage('state_update', 'queue_refresh', () => {
    fetchChannelState()
  })

  onMessage('system', 'players_update', (msg: WSMessage) => {
    log('[ChannelDetail] Players update received:', msg.payload)
    allPlayers.value = msg.payload as PlayerInfo[]
  })

  // Also listen for alarm_triggered notifications
  onMessage('notification', 'alarm_triggered', (msg: WSMessage) => {
    log('[ChannelDetail] Alarm triggered:', msg.payload)
    fetchAlarms()
  })

  onMessage('notification', 'sleep_timer_triggered', (msg: WSMessage) => {
    log('[ChannelDetail] Sleep timer triggered:', msg.payload)
    fetchAlarms()
  })
})

// Watch for reconnection
watch(connected, (newVal, oldVal) => {
  if (newVal && !oldVal) {
    log('[ChannelDetail] Reconnected, re-joining channel:', channelName.value)
    joinChannel()
  }
})

// Auto-focus search input when modal opens
watch(showSearchModal, (val) => {
  if (val) {
    nextTick(() => {
      searchPanelRef.value?.focusInput()
    })
  }
})
</script>

<style scoped>
.channel-detail {
  height: 100vh;
  display: flex;
  flex-direction: column;
  padding-bottom: 72px;
  box-sizing: border-box;
}

/* Scrollable area between header and bottom bar */
.scroll-area {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 12px;
}

.search-card {
  flex-shrink: 0;
  padding: 6px 20px;
}

.search-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 10px 14px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.15s;
}

.search-trigger:hover {
  border-color: var(--accent);
  color: var(--text-primary);
}

.queue-card {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  margin-bottom: 32px;
}

.queue-scroll-wrap {
  flex: 1;
  min-height: 0;
}

.queue-scroll-wrap :deep(.play-queue) {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.queue-scroll-wrap :deep(.queue-list) {
  max-height: none;
  flex: 1;
}

/* Search Modal header with inline type toggle */
.search-modal-header {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  width: 100%;
}

.search-modal-header .modal-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 1rem;
  font-weight: 600;
  margin: 0;
  color: var(--text-primary);
}

.search-type-toggle-inline {
  display: flex;
  gap: 3px;
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
  padding: 2px;
  justify-self: center;
}

.type-btn-inline {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 5px 12px;
  font-size: 0.8rem;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
  min-height: 32px;
}

.type-btn-inline:hover {
  color: var(--text-primary);
}

.type-btn-inline.active {
  background: var(--accent);
  color: white;
}

.search-modal-header .btn-close {
  background: none;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 6px;
  border-radius: var(--radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 44px;
  min-height: 44px;
  transition: all 0.15s ease;
  flex-shrink: 0;
  justify-self: end;
}

.search-modal-header .btn-close:hover {
  background: var(--bg-primary);
  color: var(--text-primary);
}
</style>
