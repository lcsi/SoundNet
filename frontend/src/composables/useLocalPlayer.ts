import { ref, onUnmounted } from 'vue'
import { useWebSocket } from './useWebSocket'
import type { Song, WSMessage } from '../types'
import { Actions } from '../types'
import { fetchSongUrl } from '../api/song'

/**
 * 本地播放器组合式函数
 * 管理浏览器内 Audio 播放、WebSocket 连接、注册与命令处理
 */
export function useLocalPlayer(channelName: string) {
  const {
    connected: localConnected,
    connect: localConnect,
    send: localSend,
    onMessage: localOnMessage,
    disconnect: localDisconnect,
    log: localLog,
    logerror: localLogerror,
  } = useWebSocket()

  const localPlayerEnabled = ref(false)
  const localPlayerId = ref('')
  let localAudio: HTMLAudioElement | null = null
  const localCurrentSong = ref<Song | null>(null)
  const localProgress = ref(0)
  const localPlaying = ref(false)

  // ─── Helpers ────────────────────────────────────────

  function getOrCreateLocalPlayerId(): string {
    const key = `local_player_id_${channelName}`
    const stored = localStorage.getItem(key)
    if (stored) return stored
    const id = `local-${channelName}-${Math.random().toString(36).substring(2, 8)}`
    localStorage.setItem(key, id)
    return id
  }

  // ─── Audio ──────────────────────────────────────────

  async function localPlaySong(song: Song) {
    if (!localAudio) {
      localAudio = new Audio()
      localAudio.onended = () => {
        localPlaying.value = false
        localProgress.value = 0
        localCurrentSong.value = null
        localSend({ type: 'player', action: Actions.FINISHED, payload: {} })
      }
      localAudio.onerror = (e) => {
        const error = localAudio?.error
        localLogerror('[LocalPlayer] Playback error:', error?.code, error?.message)
        localPlaying.value = false
        localProgress.value = 0
        localCurrentSong.value = null
        localSend({
          type: 'player',
          action: Actions.FINISHED,
          payload: {
            reason: 'play_error',
            error_code: error?.code,
            error_message: error?.message,
          },
        })
      }
      localAudio.ontimeupdate = () => {
        if (localAudio) {
          localProgress.value = localAudio.currentTime
        }
      }
    }

    const url = await fetchSongUrl(song)
    if (!url) {
      localLogerror('[LocalPlayer] No audio URL available for song:', song.title)
      return
    }

    localAudio.src = url
    localAudio.volume = 0.8
    await localAudio.play()
    localCurrentSong.value = { ...song, url }
    localPlaying.value = true
    localProgress.value = 0
  }

  // ─── WebSocket 消息处理 ────────────────────────────

  function setupLocalPlayerMessages() {
    localOnMessage('command', '*', (msg: WSMessage) => {
      localLog('[LocalPlayer] Received command:', msg.action, msg.payload)

      switch (msg.action) {
        case 'play':
        case 'resume':
          if (localAudio) {
            localAudio.play()
            localPlaying.value = true
          }
          break

        case 'pause':
          if (localAudio) {
            localAudio.pause()
            localPlaying.value = false
          }
          break

        case 'stop':
          if (localAudio) {
            localAudio.pause()
            localAudio.currentTime = 0
            localPlaying.value = false
            localCurrentSong.value = null
            localProgress.value = 0
          }
          break

        case 'play_song':
          if (msg.payload) {
            const song = msg.payload as Song
            localPlaySong(song).catch((error) => {
              localLogerror('[LocalPlayer] Failed to play song:', error)
            })
          }
          break

        case 'seek':
          if (localAudio && msg.payload) {
            const pos = (msg.payload as any).position
            if (typeof pos === 'number') {
              localAudio.currentTime = pos
            }
          }
          break

        case 'volume':
          if (msg.payload) {
            const vol = (msg.payload as any).volume
            if (typeof vol === 'number' && localAudio) {
              localAudio.volume = vol / 100
            }
          }
          break

        case 'join_channel':
        case 'leave_channel':
        case 'set_reporting':
          break
      }
    })
  }

  // ─── 启用 / 禁用 ───────────────────────────────────

  async function enableLocalPlayer() {
    const id = getOrCreateLocalPlayerId()
    localPlayerId.value = id

    // 先尝试通过 API 分配到频道
    try {
      const resp = await fetch(`/api/players/${id}/assign-channel`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ channel: channelName }),
      })
      if (!resp.ok && resp.status !== 404) {
        localLogerror('[LocalPlayer] Failed to assign channel')
        return
      }
    } catch (e) {
      localLogerror('[LocalPlayer] Failed to assign channel:', e)
    }

    // 以 player 身份连接 WebSocket
    setupLocalPlayerMessages()
    localConnect('player')

    // 等待连接后注册
    const checkConnection = setInterval(() => {
      if (localConnected.value) {
        localSend({
          type: 'player',
          action: Actions.REGISTER,
          payload: { player_id: id, name: id },
        })
        clearInterval(checkConnection)

        // 再次尝试分配到频道
        setTimeout(async () => {
          try {
            await fetch(`/api/players/${id}/assign-channel`, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ channel: channelName }),
            })
          } catch {
            // ignore
          }
        }, 500)
      }
    }, 200)
  }

  function disableLocalPlayer() {
    if (localAudio) {
      localAudio.pause()
      localAudio.currentTime = 0
      localPlaying.value = false
      localCurrentSong.value = null
      localProgress.value = 0
    }
    localDisconnect()
    localPlayerEnabled.value = false
  }

  function toggleLocalPlayer() {
    if (localPlayerEnabled.value) {
      disableLocalPlayer()
    } else {
      localPlayerEnabled.value = true
      enableLocalPlayer()
    }
  }

  // ─── 清理 ──────────────────────────────────────────

  onUnmounted(() => {
    if (localAudio) {
      localAudio.pause()
      localAudio = null
    }
    if (localConnected.value) {
      localDisconnect()
    }
  })

  return {
    localPlayerEnabled,
    toggleLocalPlayer,
  }
}
