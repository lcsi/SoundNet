<template>
  <Teleport to="body">
    <!-- Backdrop fade -->
    <Transition name="fade">
      <div v-if="visible" class="lyric-backdrop" @click="$emit('close')"></div>
    </Transition>

    <!-- Panel slide up -->
    <Transition name="lyric-slide">
      <div v-if="visible" class="lyric-panel">
        <!-- Header -->
        <div class="lyric-header">
          <button class="lyric-close-btn" @click="$emit('close')" title="关闭">
            <ChevronDown :size="24" />
          </button>
          <div class="lyric-header-info">
            <div class="lyric-song-title">{{ title }}</div>
            <div class="lyric-song-artist">{{ artist }}</div>
          </div>
        </div>

        <!-- Lyrics body -->
        <div class="lyric-body" ref="lyricBodyRef">
          <div v-if="loading" class="lyric-status">加载歌词中...</div>
          <div v-else-if="error" class="lyric-status lyric-error">{{ error }}</div>
          <div v-else-if="lyricLines.length === 0" class="lyric-status">暂无歌词</div>
          <div v-else class="lyric-list" ref="lyricListRef">
            <div
              v-for="(line, index) in lyricLines"
              :key="index"
              class="lyric-line"
              :class="{
                'lyric-line-active': index === currentLineIndex,
                'lyric-line-past': index < currentLineIndex
              }"
            >
              {{ line.text }}
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted } from 'vue'
import { ChevronDown } from '@lucide/vue'

interface LyricLine {
  time: number
  text: string
}

const props = defineProps<{
  visible: boolean
  songId: string
  source: string
  title: string
  artist: string
  progress: number
}>()

const emit = defineEmits<{
  close: []
}>()

const lyricLines = ref<LyricLine[]>([])
const loading = ref(false)
const error = ref('')
const lyricBodyRef = ref<HTMLElement | null>(null)
const lyricListRef = ref<HTMLElement | null>(null)

// ─── LRC parser ────────────────────────────────────────
// Handles both standard LRC:
//   [mm:ss.xx]lyric text
// And per-character LRC (netease style):
//   [mm:ss.xx]字[mm:ss.xx]符[mm:ss.xx]...
function parseLRC(lrc: string): LyricLine[] {
  const lines = lrc.trim().split('\n')
  const result: LyricLine[] = []
  // Metadata tags to filter out
  const metaRegex = /^(作曲|作词|编曲|制作人|OP|SP|原曲|原唱|混音|录音|监制|和声|吉他|贝斯|鼓|键盘|弦乐|program|制作|推广|发行|出品|统筹|企划|宣发)/

  for (const line of lines) {
    if (!line.trim()) continue

    const timeRegex = /\[(\d{2}):(\d{2})\.(\d{2,3})\]/g
    const times: number[] = []
    let text = ''
    let lastIdx = 0
    let match: RegExpExecArray | null

    while ((match = timeRegex.exec(line)) !== null) {
      const min = parseInt(match[1])
      const sec = parseInt(match[2])
      // .xx or .xxx (centiseconds or milliseconds)
      const frac = parseInt(match[3].padEnd(3, '0'))
      times.push(min * 60 + sec + frac / 1000)

      // Collect text between this timestamp and previous position
      if (match.index > lastIdx) {
        text += line.substring(lastIdx, match.index)
      }
      lastIdx = match.index + match[0].length
    }

    // Trailing text after last timestamp
    if (lastIdx < line.length) {
      text += line.substring(lastIdx)
    }

    if (times.length > 0) {
      const trimmed = text.trim().replaceAll('，', ' ').replaceAll('。', '')
      // Skip empty lines and metadata
      if (trimmed && !metaRegex.test(trimmed)) {
        // trimmed = trimmed.replaceAll('。', '')
        result.push({ time: times[0], text: trimmed })
      }
    }
  }

  return result.sort((a, b) => a.time - b.time)
}

// ─── Fetch lyrics ─────────────────────────────────────
async function fetchLyrics() {
  if (!props.songId || !props.source) {
    console.log('### props is null ##', props)
    return
  }

  loading.value = true
  error.value = ''
  lyricLines.value = []

  try {
    const resp = await fetch(
      `/api/lyric?id=${encodeURIComponent(props.songId)}&source=${encodeURIComponent(props.source)}`
    )
    const data = await resp.json()
    if (data.success && data.data?.lyric) {
      lyricLines.value = parseLRC(data.data.lyric)
    } else {
      error.value = '暂无歌词'
    }
  } catch {
    error.value = '加载歌词失败'
  } finally {
    loading.value = false
  }
}

// ─── Current line based on playback progress ──────────
const currentLineIndex = computed(() => {
  const lines = lyricLines.value
  if (lines.length === 0) return -1

  let idx = -1
  for (let i = 0; i < lines.length; i++) {
    if (props.progress >= lines[i].time) {
      idx = i
    } else {
      break
    }
  }
  return idx
})

// ─── Auto-scroll active line into view ────────────────
watch(currentLineIndex, async (idx) => {
  if (idx < 0 || !lyricListRef.value) return
  await nextTick()
  const child = lyricListRef.value.children[idx] as HTMLElement | undefined
  if (child) {
    child.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }
})

// ─── Fetch when opening or song changes ───────────────
// onMounted: handles initial mount (v-if becoming true)
onMounted(() => {
  if (props.visible) fetchLyrics()
})

watch(() => props.visible, (val) => {
  if (val) fetchLyrics()
})

// Always fetch when song changes, even if panel stays open.
// The (if visible) guard is NOT used here because:
//   - onMounted handles initial mount
//   - visible watcher handles panel re-open
//   - this watcher handles song switch while panel stays open
watch([() => props.songId], () => {
  fetchLyrics()
})
</script>

<style scoped>
/* ── Backdrop ────────────────────────────────── */
.lyric-backdrop {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(6px);
  z-index: 2000;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* ── Panel ──────────────────────────────────── */
.lyric-panel {
  position: fixed;
  bottom: 0;
  left: 0;
  width: 100vw;
  z-index: 2001;
  background: var(--bg-secondary, #1a1a2e);
  /* border-radius: 16px 16px 0 0; */
  height: 100vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 -8px 30px rgba(0, 0, 0, 0.4);
}

.lyric-slide-enter-active {
  transition: transform 0.35s cubic-bezier(0.32, 0.72, 0, 1);
}
.lyric-slide-leave-active {
  transition: transform 0.25s ease-in;
}
.lyric-slide-enter-from {
  transform: translateY(100%);
}
.lyric-slide-leave-to {
  transform: translateY(100%);
}

/* ── Header ──────────────────────────────────── */
.lyric-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border, #2a2a3e);
  flex-shrink: 0;
}

.lyric-close-btn {
  background: var(--bg-card, #2a2a3e);
  border: none;
  border-radius: 50%;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  color: var(--text-primary, #eee);
  flex-shrink: 0;
  transition: background 0.2s;
}
.lyric-close-btn:hover {
  background: var(--accent, #e74c3c);
  color: white;
}

.lyric-header-info {
  flex: 1;
  min-width: 0;
}

.lyric-song-title {
  font-size: 1rem;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.lyric-song-artist {
  font-size: 0.8rem;
  color: var(--text-secondary, #999);
  margin-top: 2px;
}

/* ── Lyrics body ─────────────────────────────── */
.lyric-body {
  flex: 1;
  overflow-y: auto;
  padding: 24px 20px 40px;
  min-height: 200px;
  max-height: none;
  -webkit-overflow-scrolling: touch;
}

.lyric-status {
  text-align: center;
  padding: 80px 20px;
  color: var(--text-secondary, #999);
  font-size: 0.95rem;
}

.lyric-error {
  color: var(--danger, #e74c3c);
}

.lyric-list {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding-bottom: 40px;
}

.lyric-line {
  font-size: 1.05rem;
  color: var(--text-secondary, #999);
  text-align: center;
  padding: 6px 0;
  transition: all 0.35s ease;
  line-height: 1.7;
  user-select: none;
}

.lyric-line-past {
  color: color-mix(in srgb, var(--text-secondary, #999) 50%, transparent);
  font-size: 0.95rem;
}

.lyric-line-active {
  color: var(--accent, #e74c3c);
  font-size: 1.2rem;
  font-weight: 600;
}
</style>
