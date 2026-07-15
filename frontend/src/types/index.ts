// Song types
export interface Song {
  id: string
  title: string
  artist: string
  album: string
  cover: string
  source: string
  duration: number
  url?: string
}

// Playlist types
export interface Playlist {
  id: string
  name: string
  cover: string
  track_count: number
  play_count: number
  creator: string
  description: string
  source: string
  link: string
}

// Player state
export interface PlayerState {
  id: string
  name: string
  note: string
  online: boolean
  volume: number
  playing: boolean
  current_song?: Song
  progress?: number
}

// State update from platform
export interface StateUpdate {
  channel: string
  players: PlayerState[]
  queue: Song[]
  current_index: number
  playing: boolean
  progress: number
  playback_mode: PlaybackMode
}

// WebSocket message
export interface WSMessage {
  type: string
  action: string
  channel?: string
  payload: any
}

// Playback mode constants
export const PlaybackModes = {
  SEQUENTIAL: 'sequential',
  LOOP: 'loop',
  SHUFFLE: 'shuffle',
} as const

export type PlaybackMode = typeof PlaybackModes[keyof typeof PlaybackModes]

// Playback mode display labels
export const PlaybackModeLabels: Record<PlaybackMode, string> = {
  [PlaybackModes.SEQUENTIAL]: '顺序',
  [PlaybackModes.LOOP]: '循环',
  [PlaybackModes.SHUFFLE]: '随机',
}

// WebSocket action types
export const Actions = {
  // Control actions
  JOIN_CHANNEL: 'join_channel',
  LEAVE_CHANNEL: 'leave_channel',
  PLAY: 'play',
  PAUSE: 'pause',
  RESUME: 'resume',
  NEXT: 'next',
  PREV: 'prev',
  SEEK: 'seek',
  VOLUME: 'volume',
  SET_PLAYER_VOLUME: 'set_player_volume',
  ADD_TO_QUEUE: 'add_to_queue',
  REMOVE_FROM_QUEUE: 'remove_from_queue',
  REORDER_QUEUE: 'reorder_queue',
  CLEAR_QUEUE: 'clear_queue',
  PLAY_INDEX: 'play_index',
  SET_PLAYBACK_MODE: 'set_playback_mode',
  REMOVE_PLAYER: 'remove_player',

  // Player actions
  REGISTER: 'register',
  STATUS_UPDATE: 'status_update',
  FINISHED: 'finished',
  SET_REPORTING: 'set_reporting',

  // Message types
  MSG_CONTROL: 'control',
  MSG_STATE_UPDATE: 'state_update',
  MSG_COMMAND: 'command',
}

// API response types
export interface SearchResult {
  results: Song[]
  query: string
}

export interface PlayerInfo {
  id: string
  name: string
  note: string
  settings?: {
    cache_dir?: string
    initial_volume?: number
  }
  created_at: string
  updated_at: string
  online: boolean
  channel?: string
}

export interface ChannelInfo {
  name: string
  display_name: string
  player_count: number
  control_count: number
}

// ─── Alarm types ──────────────────────────────────────────

export interface Alarm {
  id: string
  channel: string
  type: 'alarm_start' | 'sleep_timer'
  enabled: boolean

  // 触发方式
  trigger_mode: 'at_time' | 'countdown' | 'song_count'
  trigger_time: string        // "07:00"
  countdown_minutes: number
  song_count: number

  // 播放内容（仅 alarm_start）
  content_mode: 'queue_start' | 'specific_songs' | 'shuffle' | 'continue_current'
  song_ids?: string[]

  // 自动停止（仅 alarm_start）
  auto_stop_mode: 'play_time' | 'song_count' | 'no_limit'
  auto_stop_value: number

  // 停止行为（仅 sleep_timer）
  stop_action: 'stop' | 'pause' | 'stop_and_clear'

  // 冲突策略
  conflict_strategy: 'queue' | 'replace' | 'skip'

  // 重复规则
  repeat: 'once' | 'daily' | 'weekday' | string
  repeat_days?: number[]

  // 淡入淡出
  fade_in_seconds: number
  fade_out_seconds: number

  // 元信息
  created_at: number
  updated_at: number
  last_triggered_at?: number
}

export interface ActiveTimer {
  alarm_id: string
  type: 'alarm_start' | 'sleep_timer'
  remaining_seconds: number
  trigger_time?: string
}

export interface AlarmSyncPayload {
  alarms: Alarm[]
  active_timers: ActiveTimer[]
  deleted_id?: string
}

// Alarm default factory
export function createDefaultAlarm(channel: string, type: 'alarm_start' | 'sleep_timer'): Alarm {
  return {
    id: '',
    channel,
    type,
    enabled: true,
    trigger_mode: 'at_time',
    trigger_time: '07:00',
    countdown_minutes: 30,
    song_count: 3,
    content_mode: 'queue_start',
    auto_stop_mode: 'no_limit',
    auto_stop_value: 30,
    stop_action: 'stop',
    conflict_strategy: 'queue',
    repeat: 'once',
    fade_in_seconds: 0,
    fade_out_seconds: 0,
    created_at: Date.now(),
    updated_at: Date.now(),
  }
}

// Countdown badge display type
export interface CountdownBadgeTimer {
  type: 'alarm_start' | 'sleep_timer'
  alarm_id: string
  remaining_seconds: number
  trigger_time?: string
  display: string
}
