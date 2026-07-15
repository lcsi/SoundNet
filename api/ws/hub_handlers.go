package ws

import (
	"music-player/alarm"
	"music-player/channel"
	"music-player/db"
	"music-player/models"
	"music-player/redis"
)

var globalRedis *redis.Client
var globalDB *db.DB
var globalChannelMgr *channel.Manager
var globalAlarmEngine *alarm.Engine

func SetRedis(r *redis.Client) {
	globalRedis = r
}

func SetDB(d *db.DB) {
	globalDB = d
}

func SetChannelManager(m *channel.Manager) {
	globalChannelMgr = m
}

func SetAlarmEngine(e *alarm.Engine) {
	globalAlarmEngine = e
}

// handlePlayerFinished is called when a player reports a song has finished.
func (h *Hub) handlePlayerFinished(pc *PlayerClient, ch *channel.Channel) {
	ch.RefreshQueue(nil, globalRedis)

	// Check if we're at the end of queue in sequential mode
	chCopy := ch.BuildStateUpdate()
	if chCopy.PlaybackMode == models.PlaybackModeSequential && chCopy.CurrentIndex >= len(chCopy.Queue)-1 {
		// Last song in sequential mode — stop playback
		ch.BroadcastToPlayers(models.CmdPause, nil)
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
		return
	}

	// Advance to next song in queue (respects playback mode)
	ch.MoveToNext()

	// Get the current song info
	ch.BroadcastToPlayers(models.CmdPlaySong, ch.GetCurrentSongInfo())
	ch.BroadcastJSON(map[string]interface{}{
		"type":   models.MsgTypeStateUpdate,
		"action": models.ActionQueueRefresh,
	})
}
