package models

import "time"

type PlayerSettings struct {
	CacheDir      string `json:"cache_dir,omitempty"`
	InitialVolume int    `json:"initial_volume,omitempty"`
}

type Player struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Note      string         `json:"note"`
	Settings  PlayerSettings `json:"settings"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type Song struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Cover    string `json:"cover"`
	Source   string `json:"source"`
	Duration int    `json:"duration"` // seconds
	URL      string `json:"url,omitempty"`
}

// WebSocket message types
const (
	MsgTypeControl     = "control"
	MsgTypePlayer      = "player"
	MsgTypeStateUpdate = "state_update"
	MsgTypeCommand     = "command"
	MsgTypeSystem      = "system"
)

// State update sub-actions (action field in state_update messages)
const (
	ActionInitialState   = "initial_state"
	ActionPlayerUpdate   = "player_update"
	ActionPlayerProgress = "player_progress"
	ActionQueueRefresh   = "queue_refresh"
)

// Playback mode constants
const (
	PlaybackModeSequential = "sequential" // 顺序播放
	PlaybackModeLoop       = "loop"       // 循环播放
	PlaybackModeShuffle    = "shuffle"    // 随机播放
)

// Control actions
const (
	ActionJoinChannel     = "join_channel"
	ActionLeaveChannel    = "leave_channel"
	ActionPlay            = "play"
	ActionPause           = "pause"
	ActionResume          = "resume"
	ActionNext            = "next"
	ActionPrev            = "prev"
	ActionSeek            = "seek"
	ActionVolume          = "volume"
	ActionSetPlayerVolume = "set_player_volume"
	ActionAddToQueue      = "add_to_queue"
	ActionRemoveFromQueue = "remove_from_queue"
	ActionReorderQueue    = "reorder_queue"
	ActionClearQueue      = "clear_queue"
	ActionPlayIndex       = "play_index"
	ActionSetPlaybackMode = "set_playback_mode"
	ActionRemovePlayer    = "remove_player"
)

// Player actions
const (
	ActionRegister       = "register"
	ActionStatusUpdate   = "status_update"
	ActionFinished       = "finished"
	ActionPong           = "pong"
)

// ─── Alarm (闹钟) types ────────────────────────────────

type Alarm struct {
	ID               string   `json:"id"`
	Channel          string   `json:"channel"`
	Type             string   `json:"type"`                // "alarm_start" | "sleep_timer"
	Enabled          bool     `json:"enabled"`

	// 触发方式
	TriggerMode      string   `json:"trigger_mode"`       // "at_time" | "countdown" | "song_count"
	TriggerTime      string   `json:"trigger_time"`       // "07:00"
	CountdownMinutes int      `json:"countdown_minutes"`
	SongCount        int      `json:"song_count"`

	// 播放内容（仅 alarm_start）
	ContentMode      string   `json:"content_mode"`       // "queue_start" | "specific_songs" | "shuffle"
	SongIDs          []string `json:"song_ids,omitempty"`

	// 自动停止（仅 alarm_start）
	AutoStopMode     string   `json:"auto_stop_mode"`     // "play_time" | "song_count" | "no_limit"
	AutoStopValue    int      `json:"auto_stop_value"`

	// 停止行为（仅 sleep_timer）
	StopAction       string   `json:"stop_action"`        // "stop" | "pause" | "stop_and_clear"

	// 冲突策略
	ConflictStrategy string   `json:"conflict_strategy"`  // "queue" | "replace" | "skip"

	// 重复规则
	Repeat           string   `json:"repeat"`             // "once" | "daily" | "weekday" | "mon,tue,wed..."
	RepeatDays       []int    `json:"repeat_days,omitempty"`

	// 淡入淡出
	FadeInSeconds    int      `json:"fade_in_seconds"`
	FadeOutSeconds   int      `json:"fade_out_seconds"`

	// 元信息
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
	LastTriggeredAt  int64    `json:"last_triggered_at,omitempty"`
}

// Alarm type constants
const (
	AlarmTypeStart      = "alarm_start"
	AlarmTypeSleepTimer = "sleep_timer"
)

// Trigger mode constants
const (
	AlarmTriggerAtTime    = "at_time"
	AlarmTriggerCountdown = "countdown"
	AlarmTriggerSongCount = "song_count"
)

// Content mode constants
const (
	AlarmContentQueueStart       = "queue_start"
	AlarmContentSpecificSongs    = "specific_songs"
	AlarmContentShuffle          = "shuffle"
	AlarmContentContinueCurrent  = "continue_current"
)

// Auto-stop mode constants
const (
	AlarmAutoStopPlayTime   = "play_time"
	AlarmAutoStopSongCount  = "song_count"
	AlarmAutoStopNoLimit    = "no_limit"
)

// Stop action constants (sleep timer)
const (
	AlarmStopActionStop        = "stop"
	AlarmStopActionPause       = "pause"
	AlarmStopActionStopAndClear = "stop_and_clear"
)

// Conflict strategy constants
const (
	AlarmConflictQueue   = "queue"
	AlarmConflictReplace = "replace"
	AlarmConflictSkip    = "skip"
)

// Repeat constants
const (
	AlarmRepeatOnce    = "once"
	AlarmRepeatDaily   = "daily"
	AlarmRepeatWeekday = "weekday"
	AlarmRepeatCustom  = "custom"
)

// Alarm WebSocket actions
const (
	ActionAlarmSync      = "alarm_sync"
	ActionAlarmTriggered = "alarm_triggered"
	ActionSleepTimerTriggered = "sleep_timer_triggered"
	ActionFadeVolume     = "fade_volume"
)

// Special channel name for idle players
const IdleChannelName = "_idle"

// Commands to player
const (
	CmdPlay         = "play"
	CmdPause        = "pause"
	CmdResume       = "resume"
	CmdStop         = "stop"
	CmdNext         = "next"
	CmdPrev         = "prev"
	CmdSeek         = "seek"
	CmdVolume       = "volume"
	CmdPlaySong     = "play_song"
	CmdJoinChannel  = "join_channel"
	CmdSetReporting = "set_reporting"
)

// System broadcast actions (pushed from server to control clients)
const (
	ActionPlayersUpdate  = "players_update"
	ActionChannelsUpdate = "channels_update"
)

// SongBrief is a lightweight song representation used in real-time player updates.
type SongBrief struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Cover    string `json:"cover"`
	Duration int    `json:"duration"`
}

type Message struct {
	Type    string      `json:"type"`
	Action  string      `json:"action"`
	Channel string      `json:"channel,omitempty"`
	Payload interface{} `json:"payload"`
}

type StateUpdate struct {
	Channel      string        `json:"channel"`
	DisplayName  string        `json:"display_name"`
	Players      []PlayerState `json:"players"`
	Queue        []Song        `json:"queue"`
	CurrentIndex int           `json:"current_index"`
	Playing      bool          `json:"playing"`
	Progress     float64       `json:"progress"`
	PlaybackMode string        `json:"playback_mode"`
}

type PlayerState struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Note        string  `json:"note"`
	Online      bool    `json:"online"`
	Volume      int     `json:"volume"`
	Playing     bool    `json:"playing"`
	CurrentSong *Song   `json:"current_song,omitempty"`
	Progress    float64 `json:"progress,omitempty"`
	IsLocal     bool    `json:"is_local,omitempty"`
}
