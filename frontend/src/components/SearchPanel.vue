<template>
  <div class="search-panel">
    <div class="search-form">
      <select
        id="source-select"
        v-model="selectedSource"
        class="source-select"
        @change="onSourceChange"
      >
        <option
          v-for="opt in sourceOptions"
          :key="opt.value"
          :value="opt.value"
        >
          {{ opt.label }}
        </option>
      </select>
      <input
        ref="searchInput"
        v-model="query"
        type="search"
        placeholder="搜索歌曲/歌单..."
        @keyup.enter="search"
      />
      <button class="btn btn-primary btn-sm" @click="search" :disabled="!query.trim()">
        搜索
      </button>
    </div>

    <!-- Source selector (dropdown, song mode only) -->
    <!--<div v-if="effectiveType === 'song'" class="source-selector-wrap">
      <label class="source-label" for="source-select">来源:</label>

    </div> -->

    <!-- Loading -->
    <div v-if="loading || loadingPlaylists || loadingPlaylistSongs" class="loading">搜索中...</div>

    <!-- Song results -->
    <template v-else-if="effectiveType === 'song'">
      <div v-if="songResults.length > 0" class="search-results">
        <div
          v-for="song in songResults"
          :key="song.id"
          class="search-item"
        >
          <div class="search-cover" v-if="song.cover">
            <img :src="song.cover" :alt="song.title" />
          </div>
          <div class="search-item-body">
            <div class="search-item-top">
              <div class="search-info">
                <div class="search-title">{{ song.title }}</div>
                <div class="search-artist">{{ song.artist }} · {{ song.album }}</div>
              </div>
            </div>
            <div class="search-item-bottom">
              <span class="source-badge">{{ sourceLabel(song.source) }}</span>
              <span class="search-duration">{{ formatTime(song.duration) }}</span>
              <div class="search-actions">
                <button class="btn btn-sm" @click="$emit('addToQueue', song)" title="加入队列">
                  <ListMusic :size="16" />
                </button>
                <button class="btn btn-primary btn-sm" @click="$emit('playNow', song)" title="立即播放">
                  <Play :size="16" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div v-else-if="songSearched" class="no-results">
        未找到 "{{ query }}" 的相关结果
      </div>
    </template>

    <!-- Playlist mode: detail view -->
    <template v-else-if="selectedPlaylist">
      <div class="playlist-detail-view">
        <div class="playlist-detail-header">
          <button class="btn btn-sm" @click="backToPlaylists">
            <SkipBack :size="16" /> 返回
          </button>
        </div>
        <div class="playlist-detail-info">
          <img v-if="selectedPlaylist.cover" :src="selectedPlaylist.cover" class="playlist-detail-cover" />
          <div class="playlist-detail-meta">
            <div class="playlist-detail-name">{{ selectedPlaylist.name }}</div>
            <div class="playlist-detail-desc" v-if="selectedPlaylist.description">{{ selectedPlaylist.description }}</div>
            <div class="playlist-detail-stats">
              <span v-if="selectedPlaylist.creator">{{ selectedPlaylist.creator }}</span>
              <span>{{ selectedPlaylist.track_count }} 首</span>
            </div>
            <button
              v-if="playlistSongs.length > 0"
              class="btn btn-sm"
              style="position: absolute; bottom: 0; right: 0;"
              @click="$emit('playAll', playlistSongs)"
            >
              <Play :size="16" /> 播放
            </button>
          </div>
        </div>
        <div v-if="playlistSongs.length === 0" class="no-results">歌单暂无歌曲</div>
        <div v-else class="playlist-song-list">
          <div v-for="(song, index) in playlistSongs" :key="song.id + '-' + index" class="playlist-song-item">
            <div class="playlist-song-index">{{ index + 1 }}</div>
            <div class="playlist-song-info">
              <div class="playlist-song-title">{{ song.title }}</div>
              <div class="playlist-song-artist">{{ song.artist }}</div>
            </div>
            <div class="playlist-song-duration">{{ formatTime(song.duration) }}</div>
            <div class="playlist-song-actions">
              <button class="btn btn-sm" @click="$emit('addToQueue', song)" title="加入队列">
                <Plus :size="16" />
              </button>
              <button class="btn btn-primary btn-sm" @click="$emit('playNow', song)" title="立即播放">
                <Play :size="16" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- Playlist mode: list view -->
    <template v-else-if="effectiveType === 'playlist'">
      <div v-if="playlists.length > 0" class="playlist-results">
        <div
          v-for="pl in playlists"
          :key="pl.id"
          class="playlist-result-item"
          @click="selectPlaylist(pl)"
        >
          <img :src="pl.cover" :alt="pl.name" class="playlist-result-cover" />
          <div class="playlist-result-info">
            <div class="playlist-result-name">{{ pl.name }}</div>
            <div class="playlist-result-meta">{{ pl.track_count }} 首 · {{ pl.creator }}</div>
          </div>
        </div>
      </div>
      <div v-else-if="playlistSearched" class="no-results">
        未找到相关歌单
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { Song, Playlist } from '../types'
import { ListMusic, Play, Plus, SkipBack } from '@lucide/vue'

const props = defineProps<{
  searchType?: 'song' | 'playlist'
}>()

const emit = defineEmits<{
  addToQueue: [song: Song]
  playNow: [song: Song]
  playAll: [songs: Song[]]
  'update:searchType': [type: 'song' | 'playlist']
}>()

const SOURCE_OPTIONS = [
  { value: 'all', label: '全部' },
  { value: 'netease', label: '网易' },
  { value: 'qq', label: 'QQ' },
  { value: 'kuwo', label: '酷我' },
  { value: 'kugou', label: '酷狗' },
  { value: 'migu', label: '咪咕' },
]

const SOURCE_LABELS: Record<string, string> = {
  netease: '网易云',
  qq: 'QQ音乐',
  kuwo: '酷我',
  kugou: '酷狗',
  migu: '咪咕'
}

const searchInput = ref<HTMLInputElement | null>(null)
const query = ref('')

// searchType is now controlled by parent via prop
const effectiveType = computed(() => props.searchType || 'song')

// Watch for searchType changes from parent → trigger switch logic
watch(() => props.searchType, (newType, oldType) => {
  if (newType && newType !== oldType) {
    selectedPlaylist.value = null
    playlistSongs.value = []
    if (newType === 'playlist' && playlists.value.length === 0) {
      query.value = '每日推荐'
      searchPlaylists()
    }
  }
})

// Song search state
const songResults = ref<Song[]>([])
const songSearched = ref(false)
const selectedSource = ref('all')

// Playlist search state
const playlists = ref<Playlist[]>([])
const playlistSearched = ref(false)
const selectedPlaylist = ref<Playlist | null>(null)
const playlistSongs = ref<Song[]>([])

const loading = ref(false)
const loadingPlaylists = ref(false)
const loadingPlaylistSongs = ref(false)

const sourceOptions = SOURCE_OPTIONS

function sourceLabel(source: string): string {
  return SOURCE_LABELS[source] || source
}

function onSourceChange() {
  if (query.value.trim()) searchSongs()
}

function formatTime(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

async function search() {
  const q = query.value.trim()
  if (!q) return

  if (effectiveType.value === 'playlist') {
    selectedPlaylist.value = null
    playlistSongs.value = []
    await searchPlaylists()
  } else {
    await searchSongs()
  }
}

async function searchSongs() {
  const q = query.value.trim()
  if (!q) return

  loading.value = true
  songSearched.value = true

  let sourcesParam: string
  if (selectedSource.value === 'all') {
    sourcesParam = 'netease,qq,kuwo,kugou'
  } else {
    sourcesParam = selectedSource.value
  }

  try {
    const resp = await fetch(`/api/search?q=${encodeURIComponent(q)}&sources=${sourcesParam}`)
    const data = await resp.json()
    songResults.value = (data.results || []).filter((v: Song) => {
      return v.source === 'netease' || v.source === 'qq' || v.source === 'kuwo' || v.source === 'kugou'
    })
  } catch (e) {
    console.error('Search failed:', e)
    songResults.value = []
  } finally {
    loading.value = false
  }
}

// Playlist search
async function searchPlaylists() {
  const q = query.value.trim() || '每日推荐'
  loadingPlaylists.value = true
  playlistSearched.value = true

  try {
    const resp = await fetch(`/api/search?q=${encodeURIComponent(q)}&type=playlist&sources=netease`)
    const data = await resp.json()
    playlists.value = data.results || []
  } catch (e) {
    console.error('Playlist search failed:', e)
    playlists.value = []
  } finally {
    loadingPlaylists.value = false
  }
}

// Select a playlist to view its songs
async function selectPlaylist(pl: Playlist) {
  selectedPlaylist.value = pl
  loadingPlaylistSongs.value = true
  playlistSongs.value = []

  try {
    const resp = await fetch(`/api/playlist/detail?id=${pl.id}&source=${pl.source}`)
    const data = await resp.json()
    playlistSongs.value = data.results || []
  } catch (e) {
    console.error('Failed to load playlist songs:', e)
    playlistSongs.value = []
  } finally {
    loadingPlaylistSongs.value = false
  }
}

// Back to playlist list
function backToPlaylists() {
  selectedPlaylist.value = null
  playlistSongs.value = []
}

defineExpose({
  focusInput() {
    searchInput.value?.focus()
  },
})
</script>

<style scoped>
.search-panel {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  flex: 1;
}


.search-form {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}

.search-form input {
  flex: 1;
}


/* Source dropdown — replaces buttons (P2#9) */
.source-selector-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.source-label {
  font-size: 0.85rem;
  color: var(--text-secondary);
  flex-shrink: 0;
}

.source-select {
  padding: 10px 10px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text-primary);
  font-size: 0.9rem;
  cursor: pointer;
  outline: none;
  min-width: 60px;
  min-height: 44px;
  transition: border-color 0.2s ease;
}

.source-select:focus {
  border-color: var(--accent);
}

.source-select option {
  background: var(--bg-secondary);
  color: var(--text-primary);
}

.loading,
.no-results {
  text-align: center;
  color: var(--text-secondary);
  padding: 16px;
  font-size: 0.9rem;
}

/* Song results */
.search-results {
  display: flex;
  /* max-height: 65vh; */
  flex-direction: column;
  gap: 6px;
  overflow-y: auto;
  flex: 1;
  overscroll-behavior: contain;
}

.search-item {
  display: flex;
  gap: 10px;
  padding: 8px 10px;
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
}

.search-cover {
  width: 48px;
  height: 48px;
  border-radius: 4px;
  overflow: hidden;
  flex-shrink: 0;
}

.search-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.search-item-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.search-item-top {
  display: flex;
  align-items: flex-start;
}

.search-info {
  flex: 1;
  min-width: 0;
}

.search-title {
  font-size: 0.85rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.search-artist {
  font-size: 0.75rem;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.search-item-bottom {
  display: flex;
  align-items: center;
  gap: 8px;
}

.source-badge {
  font-size: 0.7rem;
  padding: 2px 8px;
  border-radius: 10px;
  background: var(--bg-card);
  color: var(--text-secondary);
  flex-shrink: 0;
  border: 1px solid var(--border);
  white-space: nowrap;
}

.search-duration {
  font-size: 0.75rem;
  color: var(--text-secondary);
  flex-shrink: 0;
}

.search-actions {
  display: flex;
  gap: 4px;
  margin-left: auto;
}

/* Playlist results */
.playlist-results {
  display: flex;
  flex-direction: column;
  gap: 6px;
  overflow-y: auto;
  flex: 1;
  overscroll-behavior: contain;
}

.playlist-result-item {
  display: flex;
  gap: 10px;
  padding: 8px 10px;
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background 0.15s;
}

.playlist-result-item:hover {
  background: var(--bg-card);
}

.playlist-result-cover {
  width: 56px;
  height: 56px;
  border-radius: 4px;
  object-fit: cover;
  flex-shrink: 0;
}

.playlist-result-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 4px;
}

.playlist-result-name {
  font-size: 0.85rem;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.playlist-result-meta {
  font-size: 0.75rem;
  color: var(--text-secondary);
}

/* Playlist detail view */
.playlist-detail-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow-y: auto;
  flex: 1;
  overscroll-behavior: contain;
}

.playlist-detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.playlist-detail-info {
  display: flex;
  gap: 12px;
  padding: 10px;
  background: var(--bg-primary);
  border-radius: var(--radius-sm);
}

.playlist-detail-cover {
  width: 80px;
  height: 80px;
  border-radius: 6px;
  object-fit: cover;
  flex-shrink: 0;
}

.playlist-detail-meta {
  position: relative;
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 4px;
}

.playlist-detail-name {
  font-size: 1rem;
  font-weight: 600;
}

.playlist-detail-desc {
  font-size: 0.75rem;
  color: var(--text-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.playlist-detail-stats {
  font-size: 0.75rem;
  color: var(--text-secondary);
  display: flex;
  gap: 8px;
}

/* Playlist song list */
.playlist-song-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  overflow-y: auto;
}

.playlist-song-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: var(--radius-sm);
  transition: background 0.15s;
}

.playlist-song-item:hover {
  background: var(--bg-primary);
}

.playlist-song-index {
  width: 24px;
  text-align: center;
  font-size: 0.75rem;
  color: var(--text-secondary);
  flex-shrink: 0;
}

.playlist-song-info {
  flex: 1;
  min-width: 0;
}

.playlist-song-title {
  font-size: 0.85rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.playlist-song-artist {
  font-size: 0.75rem;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.playlist-song-duration {
  font-size: 0.75rem;
  color: var(--text-secondary);
  flex-shrink: 0;
}

.playlist-song-actions {
  display: flex;
  gap: 4px;
  /* opacity: 0; */
  /* transition: opacity 0.15s; */
}

.playlist-song-item:hover .playlist-song-actions {
  opacity: 1;
}
</style>
