package ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"music-player/channel"
	"music-player/models"
)



const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 4096
)

// PlayerClient represents a WebSocket connection from a player.
type PlayerClient struct {
	hub         *Hub
	conn        *websocket.Conn
	send        chan []byte
	stopped     chan struct{} // closed when WritePump exits, unblocks fallback goroutines
	id          string
	name        string
	note        string
	volume      int
	playing     bool
	currentSong *models.Song
	progress    float64
	ch          *ChannelRef
	isLocal     bool // true for browser-based local players (ID prefix: local-)
	mu          sync.RWMutex
}

// ChannelRef is a safe reference to a channel.
type ChannelRef struct {
	ch *Channel
	mu sync.RWMutex
}

// Channel is a local type alias for the channel package's Channel.
type Channel = channel.Channel

func NewPlayerClient(hub *Hub, conn *websocket.Conn) *PlayerClient {
	return &PlayerClient{
		hub:     hub,
		conn:    conn,
		send:    make(chan []byte, 256),
		stopped: make(chan struct{}),
		volume:  80,
		ch:      &ChannelRef{},
	}
}

func (pc *PlayerClient) GetID() string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.id
}

func (pc *PlayerClient) GetName() string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.name
}

func (pc *PlayerClient) GetNote() string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.note
}

func (pc *PlayerClient) IsLocalPlayer() bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.isLocal
}

func (pc *PlayerClient) GetVolume() int {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.volume
}

func (pc *PlayerClient) IsPlaying() bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.playing
}

func (pc *PlayerClient) GetCurrentSong() *models.Song {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.currentSong
}

func (pc *PlayerClient) GetProgress() float64 {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.progress
}

func (pc *PlayerClient) SendCommand(action string, payload interface{}) error {
	msg := models.Message{
		Type:    models.MsgTypeCommand,
		Action:  action,
		Payload: payload,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	// Route through send channel so WritePump is the sole writer to pc.conn.
	// gorilla/websocket does not support concurrent writes.
	select {
	case pc.send <- data:
	default:
		// Buffer full — fan out to a goroutine so the caller is not blocked.
		// Use stopped channel to prevent goroutine leak when WritePump exits.
		go func() {
			select {
			case pc.send <- data:
			case <-pc.stopped:
			}
		}()
	}
	return nil
}

func (pc *PlayerClient) SendJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	select {
	case pc.send <- data:
	default:
		go func() {
			select {
			case pc.send <- data:
			case <-pc.stopped:
			}
		}()
	}
	return nil
}

// SetChannel safely sets the player's channel reference.
func (pc *PlayerClient) SetChannel(ch *Channel) {
	pc.ch.mu.Lock()
	defer pc.ch.mu.Unlock()
	pc.ch.ch = ch
}

// getChannel safely gets the player's channel reference.
func (pc *PlayerClient) getChannel() *Channel {
	pc.ch.mu.RLock()
	defer pc.ch.mu.RUnlock()
	return pc.ch.ch
}

// buildPlayerUpdateMsg builds a state_update:player_update message for this player.
func (pc *PlayerClient) buildPlayerUpdateMsg() map[string]interface{} {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	payload := map[string]interface{}{
		"id":       pc.id,
		"name":     pc.name,
		"online":   true,
		"volume":   pc.volume,
		"playing":  pc.playing,
		"progress": pc.progress,
	}
	if pc.isLocal {
		payload["is_local"] = true
	}
	if pc.currentSong != nil {
		payload["current_song"] = models.SongBrief{
			ID:       pc.currentSong.ID,
			Title:    pc.currentSong.Title,
			Artist:   pc.currentSong.Artist,
			Cover:    pc.currentSong.Cover,
			Duration: pc.currentSong.Duration,
		}
	}

	return map[string]interface{}{
		"type":    models.MsgTypeStateUpdate,
		"action":  models.ActionPlayerUpdate,
		"payload": payload,
	}
}

// buildPlayerProgressMsg builds a state_update:player_progress message for this player.
func (pc *PlayerClient) buildPlayerProgressMsg() map[string]interface{} {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	return map[string]interface{}{
		"type":   models.MsgTypeStateUpdate,
		"action": models.ActionPlayerProgress,
		"payload": map[string]interface{}{
			"id":       pc.id,
			"progress": pc.progress,
			"playing":  pc.playing,
		},
	}
}

// BroadcastPlayerUpdate sends a player_update for this player to all controls in its channel.
func (pc *PlayerClient) BroadcastPlayerUpdate() {
	if ch := pc.getChannel(); ch != nil {
		ch.BroadcastJSON(pc.buildPlayerUpdateMsg())
	}
}

// ReadPump reads messages from the WebSocket connection.
func (pc *PlayerClient) ReadPump() {
	defer func() {
		// Check whether this connection has already been replaced by a new one
		// (reconnect race: new handleRegister ran before this defer).
		// If so, skip all stale cleanup — the new connection is managing this player.
		if current := pc.hub.GetPlayer(pc.GetID()); current != pc {
			log.Printf("[PlayerClient] Player %s reconnected, skipping stale cleanup", pc.GetID())
		} else {
			// Truly disconnected: remove from channel's in-memory map (keep Redis mapping for reconnect)
			if ch := pc.getChannel(); ch != nil && globalChannelMgr != nil {
				globalChannelMgr.DisconnectPlayer(ch, pc.GetID())
				// Push a player_update with online=false so controls see the player disappear immediately
				// Skip for local players — they should be invisible to other controls
				if !pc.isLocal {
					msg := map[string]interface{}{
						"type":   models.MsgTypeStateUpdate,
						"action": models.ActionPlayerUpdate,
						"payload": map[string]interface{}{
							"id":     pc.GetID(),
							"name":   pc.GetName(),
							"online": false,
						},
					}
					ch.BroadcastJSON(msg)
				}
			}
			pc.hub.UnregisterPlayer(pc)

			// Broadcast updated player/channel lists after a player disconnects (skip for local players)
			if !pc.isLocal {
				pc.hub.BroadcastPlayersUpdate()
				pc.hub.BroadcastChannelsUpdate()
			}
		}

		pc.conn.Close()
	}()

	pc.conn.SetReadLimit(maxMessageSize)
	pc.conn.SetReadDeadline(time.Now().Add(pongWait))
	pc.conn.SetPongHandler(func(string) error {
		pc.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := pc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[PlayerClient] Read error: %v", err)
			}
			break
		}

		// Reset read deadline on every received message — not just PONG frames.
		// This keeps the connection alive even when WebSocket-level PING/PONG
		// is throttled (e.g., background tab, proxy), as long as the client
		// sends application-level heartbeats.
		pc.conn.SetReadDeadline(time.Now().Add(pongWait))

		var msg models.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("[PlayerClient] Invalid message: %v", err)
			continue
		}

		pc.handleMessage(&msg)
	}
}

// WritePump writes messages to the WebSocket connection.
func (pc *PlayerClient) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		close(pc.stopped)
		pc.conn.Close()
	}()

	for {
		select {
		case message, ok := <-pc.send:
			pc.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				pc.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := pc.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			pc.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := pc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (pc *PlayerClient) handleMessage(msg *models.Message) {
	switch msg.Action {
	case models.ActionRegister:
		pc.handleRegister(msg)
	case models.ActionStatusUpdate:
		pc.handleStatusUpdate(msg)
	case models.ActionFinished:
		pc.handleFinished()
	case models.ActionPong:
		// pong handled automatically
	}
}

func (pc *PlayerClient) handleRegister(msg *models.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		log.Printf("[PlayerClient] Invalid register payload")
		return
	}

	id, _ := payload["player_id"].(string)
	if id == "" {
		log.Printf("[PlayerClient] Register missing player_id")
		return
	}

	pc.mu.Lock()
	pc.id = id
	if n, ok := payload["name"].(string); ok {
		pc.name = n
	}
	// Detect local players by ID prefix
	if len(id) > 6 && id[:6] == "local-" {
		pc.isLocal = true
	}
	pc.mu.Unlock()

	// Persist player to database (create if not exists, update timestamp)
	if globalDB != nil {
		if err := globalDB.UpsertPlayer(id); err != nil {
			log.Printf("[PlayerClient] Failed to persist player %s: %v", id, err)
		}
	}

	// Register in the hub (in-memory). This overwrites the old pointer so that
	// any stale connection's defer can detect it has been replaced.
	pc.hub.RegisterPlayer(pc)

	// Broadcast updated player/channel lists to all controls (skip for local players)
	if !pc.isLocal {
		pc.hub.BroadcastPlayersUpdate()
		pc.hub.BroadcastChannelsUpdate()
	}

	// Re-join player to previous channel on reconnect (if any)
	if globalRedis != nil && globalChannelMgr != nil {
		chName, err := globalRedis.GetPlayerChannel(context.Background(), id)
		if err == nil && chName != "" {
			// Only rejoin if the channel still exists (in memory or DB).
			// If it was deleted, the Redis mapping is stale — clear it instead of
			// resurrecting the channel.
			ch := globalChannelMgr.GetChannel(chName)
			if ch == nil && globalDB != nil {
				if rec, recErr := globalDB.GetChannelRecord(chName); recErr == nil && rec != nil {
					ch = globalChannelMgr.GetOrCreateChannel(chName)
				}
			}
			if ch == nil {
				// Channel was deleted — drop the stale player→channel mapping.
				globalRedis.DelPlayerChannel(context.Background(), id)
			} else {
				// Remove any stale pointer for this player ID from the channel first
				ch.RemovePlayer(id)

				globalChannelMgr.AddPlayerToChannel(ch, pc)
				pc.SetChannel(ch)

				// Push a player_update so controls see the player back online immediately
				if !pc.isLocal {
					ch.BroadcastJSON(pc.buildPlayerUpdateMsg())
				}

				pc.SendCommand("join_channel", map[string]interface{}{
					"channel": chName,
				})

				// If channel is currently playing, send the current song to the new player
				// so it can seek to the right position and start playing
				songInfo := ch.GetCurrentSongInfo()
				hasSong := false
				if m, ok := songInfo.(map[string]interface{}); ok {
					if _, empty := m["empty"]; !empty {
						hasSong = true
					}
				}

				if ch.HasControls() && hasSong {
					// There are controls connected and a song is queued — send current song
					ch.RefreshQueue(nil, globalRedis)
					song := ch.GetCurrentSongInfo()

					// Find the progress from an existing player in the channel
					var currentProgress float64
					players := ch.Players
					for _, p := range players {
						if p.GetID() != id && p.IsPlaying() {
							currentProgress = p.GetProgress()
							break
						}
					}

					pc.SendCommand(models.CmdPlaySong, song)
					if currentProgress > 0 {
						pc.SendCommand(models.CmdSeek, map[string]interface{}{"position": currentProgress})
					}
					log.Printf("[PlayerClient] Player %s joined playing channel %s, sent current song (progress: %.1f)", id, chName, currentProgress)
				}

				log.Printf("[PlayerClient] Player %s re-joined channel %s after reconnect", id, chName)
			}
		}
	}
}

func (pc *PlayerClient) handleStatusUpdate(msg *models.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	var songChanged bool
	var prevSongWasNil bool

	pc.mu.Lock()

	if p, ok := payload["progress"].(float64); ok {
		pc.progress = p
	}
	if pl, ok := payload["playing"].(bool); ok {
		pc.playing = pl
	}
	if v, ok := payload["volume"].(float64); ok {
		pc.volume = int(v)
	}
	if songData, ok := payload["current_song"].(map[string]interface{}); ok {
		song := &models.Song{}
		if id, ok := songData["id"].(string); ok {
			song.ID = id
		}
		if title, ok := songData["title"].(string); ok {
			song.Title = title
		}
		if artist, ok := songData["artist"].(string); ok {
			song.Artist = artist
		}
		if source, ok := songData["source"].(string); ok {
			song.Source = source
		}
		if url, ok := songData["url"].(string); ok {
			song.URL = url
		}
		// Detect song transition: track whether previous song was nil
		// (skip first-song detection to avoid counting song starts as completions)
		if pc.currentSong == nil {
			prevSongWasNil = true
		} else if pc.currentSong.ID != song.ID {
			songChanged = true
		}
		pc.currentSong = song
	}

	pc.mu.Unlock()

	ch := pc.getChannel()
	if ch == nil {
		return
	}

	// Always send lightweight progress update
	ch.BroadcastJSON(pc.buildPlayerProgressMsg())

	// Send full player update when song changed
	if songChanged || prevSongWasNil {
		ch.BroadcastJSON(pc.buildPlayerUpdateMsg())
	}

	// Check alarm song-count auto-stop on song transition
	// Only count transitions between real songs (skip the first song after alarm start)
	if songChanged && !prevSongWasNil && globalAlarmEngine != nil {
		if globalAlarmEngine.HandleSongFinished(ch) {
			log.Printf("[PlayerClient] Auto-stop triggered by song change for player %s", pc.GetID())
		}
	}
}

func (pc *PlayerClient) handleFinished() {
	// Song finished playing - platform should trigger next song
	// This is handled by the hub logic
	if ch := pc.getChannel(); ch != nil {
		// The platform will process this via the main handler
		pc.hub.handlePlayerFinished(pc, ch)
	}
}
