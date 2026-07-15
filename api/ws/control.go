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

// ControlClient represents a WebSocket connection from a control page.
type ControlClient struct {
	hub     *Hub
	conn    *websocket.Conn
	send    chan []byte
	stopped chan struct{} // closed when WritePump exits, unblocks fallback goroutines
	channel string
	manager *channel.Manager
	mu      sync.RWMutex
}

func NewControlClient(hub *Hub, conn *websocket.Conn, manager *channel.Manager) *ControlClient {
	return &ControlClient{
		hub:     hub,
		conn:    conn,
		send:    make(chan []byte, 256),
		stopped: make(chan struct{}),
		manager: manager,
	}
}

func (cc *ControlClient) GetChannel() string {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.channel
}

func (cc *ControlClient) SetChannel(name string) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.channel = name
}

func (cc *ControlClient) SendStateUpdate(update *models.StateUpdate) error {
	msg := models.Message{
		Type:    models.MsgTypeStateUpdate,
		Action:  "",
		Channel: update.Channel,
		Payload: update,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	// Route through send channel so WritePump is the sole writer to cc.conn.
	select {
	case cc.send <- data:
	default:
		go func() {
			select {
			case cc.send <- data:
			case <-cc.stopped:
			}
		}()
	}
	return nil
}

func (cc *ControlClient) SendJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	select {
	case cc.send <- data:
	default:
		go func() {
			select {
			case cc.send <- data:
			case <-cc.stopped:
			}
		}()
	}
	return nil
}

// ReadPump reads messages from the WebSocket connection.
func (cc *ControlClient) ReadPump() {
	defer func() {
		// Remove from channel
		if ch := cc.getChannelObj(); ch != nil {
			cc.manager.RemoveControlFromChannel(ch, cc)
			// Notify players to stop reporting if no control clients remain
			if !ch.HasControls() {
				ch.BroadcastToPlayers(models.CmdSetReporting, map[string]interface{}{"enabled": false})
			}
		}
		cc.hub.UnregisterControl(cc)
		cc.conn.Close()
	}()

	cc.conn.SetReadLimit(maxMessageSize)
	cc.conn.SetReadDeadline(time.Now().Add(pongWait))
	cc.conn.SetPongHandler(func(string) error {
		cc.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := cc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[ControlClient] Read error: %v", err)
			}
			break
		}

		// Reset read deadline on every received message, not just PONG frames.
		cc.conn.SetReadDeadline(time.Now().Add(pongWait))

		var msg models.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("[ControlClient] Invalid message: %v", err)
			continue
		}

		cc.handleMessage(&msg)
	}
}

// WritePump writes messages to the WebSocket connection.
func (cc *ControlClient) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		close(cc.stopped)
		cc.conn.Close()
	}()

	for {
		select {
		case message, ok := <-cc.send:
			cc.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				cc.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := cc.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			cc.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := cc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (cc *ControlClient) getChannelObj() *channel.Channel {
	name := cc.GetChannel()
	if name == "" {
		return nil
	}
	return cc.manager.GetChannel(name)
}

func (cc *ControlClient) handleMessage(msg *models.Message) {
	switch msg.Action {
	case models.ActionJoinChannel:
		cc.handleJoinChannel(msg)
	case models.ActionLeaveChannel:
		cc.handleLeaveChannel()
	case models.ActionPlay:
		cc.handlePlay()
	case models.ActionPause:
		cc.handlePause()
	case models.ActionResume:
		cc.handleResume()
	case models.ActionNext:
		cc.handleNext()
	case models.ActionPrev:
		cc.handlePrev()
	case models.ActionSeek:
		cc.handleSeek(msg)
	case models.ActionVolume:
		cc.handleVolume(msg)
	case models.ActionSetPlayerVolume:
		cc.handleSetPlayerVolume(msg)
	case models.ActionAddToQueue:
		cc.handleAddToQueue(msg)
	case models.ActionRemoveFromQueue:
		cc.handleRemoveFromQueue(msg)
	case models.ActionReorderQueue:
		cc.handleReorderQueue(msg)
	case models.ActionClearQueue:
		cc.handleClearQueue()
	case models.ActionPlayIndex:
		cc.handlePlayIndex(msg)
	case models.ActionSetPlaybackMode:
		cc.handleSetPlaybackMode(msg)
	case models.ActionRemovePlayer:
		cc.handleRemovePlayer(msg)
	}
}

func (cc *ControlClient) handleJoinChannel(msg *models.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	channelName, _ := payload["channel"].(string)
	displayName, _ := payload["display_name"].(string)
	if channelName == "" {
		return
	}

	// Leave current channel first
	if oldCh := cc.getChannelObj(); oldCh != nil {
		cc.manager.RemoveControlFromChannel(oldCh, cc)
	}

	// Join new channel
	ch := cc.manager.GetOrCreateChannel(channelName, displayName)
	cc.manager.AddControlToChannel(ch, cc)
	cc.SetChannel(channelName)

	// Refresh queue from Redis
	if globalRedis != nil {
		ch.RefreshQueue(nil, globalRedis)
	}

	// Send initial state update (initial_state message)
	update := ch.BuildStateUpdate()
	outMsg := map[string]interface{}{
		"type":    models.MsgTypeStateUpdate,
		"action":  models.ActionInitialState,
		"channel": channelName,
		"payload": update,
	}
	cc.SendJSON(outMsg)

	// Notify players to start reporting if this is the first control client
    // why == 1？
	// if ch.ControlCount() == 1 {
    if ch.ControlCount() >= 1 {
		ch.BroadcastToPlayers(models.CmdSetReporting, map[string]interface{}{"enabled": true})
	}

	log.Printf("[ControlClient] Joined channel: %s", channelName)
}

func (cc *ControlClient) handleLeaveChannel() {
	if ch := cc.getChannelObj(); ch != nil {
		cc.manager.RemoveControlFromChannel(ch, cc)
		// Notify players to stop reporting if no control clients remain
		if !ch.HasControls() {
			ch.BroadcastToPlayers(models.CmdSetReporting, map[string]interface{}{"enabled": false})
		}
	}
	cc.SetChannel("")
	log.Printf("[ControlClient] Left channel")
}

func (cc *ControlClient) handlePlay() {
	if ch := cc.getChannelObj(); ch != nil {
		ch.BroadcastToPlayers(models.CmdPlay, nil)
	}
}

func (cc *ControlClient) handlePause() {
	if ch := cc.getChannelObj(); ch != nil {
		ch.BroadcastToPlayers(models.CmdPause, nil)
	}
}

func (cc *ControlClient) handleResume() {
	if ch := cc.getChannelObj(); ch != nil {
		ch.BroadcastToPlayers(models.CmdResume, nil)
	}
}

func (cc *ControlClient) handleNext() {
	if ch := cc.getChannelObj(); ch != nil {
		ch.MoveToNext()
		ch.BroadcastToPlayers(models.CmdPlaySong, ch.GetCurrentSongInfo())
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
	}
}

func (cc *ControlClient) handlePrev() {
	if ch := cc.getChannelObj(); ch != nil {
		ch.MoveToPrev()
		ch.BroadcastToPlayers(models.CmdPlaySong, ch.GetCurrentSongInfo())
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
	}
}

func (cc *ControlClient) handleSeek(msg *models.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	position, _ := payload["position"].(float64)
	if ch := cc.getChannelObj(); ch != nil {
		ch.BroadcastToPlayers(models.CmdSeek, map[string]interface{}{
			"position": position,
		})
	}
}

func (cc *ControlClient) handleVolume(msg *models.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	volume, _ := payload["volume"].(float64)
	if ch := cc.getChannelObj(); ch != nil {
		ch.BroadcastToPlayers(models.CmdVolume, map[string]interface{}{
			"volume": int(volume),
		})
	}
}

func (cc *ControlClient) handleSetPlayerVolume(msg *models.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	playerID, _ := payload["player_id"].(string)
	volume, _ := payload["volume"].(float64)
	if playerID == "" {
		return
	}
	pc := cc.hub.GetPlayer(playerID)
	if pc != nil {
		pc.SendCommand(models.CmdVolume, map[string]interface{}{
			"volume": int(volume),
		})
	}
}

func (cc *ControlClient) handleAddToQueue(msg *models.Message) {
	if globalRedis == nil {
		return
	}
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	songData, ok := payload["song"].(map[string]interface{})
	if !ok {
		return
	}

	song := models.Song{}
	if id, ok := songData["id"].(string); ok {
		song.ID = id
	}
	if title, ok := songData["title"].(string); ok {
		song.Title = title
	}
	if artist, ok := songData["artist"].(string); ok {
		song.Artist = artist
	}
	if album, ok := songData["album"].(string); ok {
		song.Album = album
	}
	if cover, ok := songData["cover"].(string); ok {
		song.Cover = cover
	}
	if duration, ok := songData["duration"].(float64); ok {
		song.Duration = int(duration)
	}
    if source, ok := songData["source"].(string); ok {
 		song.Source = source
 	}
	if url, ok := songData["url"].(string); ok {
		song.URL = url
	}

	channelName := cc.GetChannel()
	if target, ok := payload["channel"].(string); ok && target != "" {
		channelName = target
	}
	if channelName == "" {
		return
	}

	globalRedis.AddToQueue(context.Background(), channelName, song)

	// Get the target channel object (may differ from the current one)
	targetCh := cc.manager.GetChannel(channelName)
	if targetCh != nil {
		targetCh.RefreshQueue(nil, globalRedis)
		targetCh.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
	}
}

func (cc *ControlClient) handleRemoveFromQueue(msg *models.Message) {
	if globalRedis == nil {
		return
	}
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	index, _ := payload["index"].(float64)

	channelName := cc.GetChannel()
	if channelName == "" {
		return
	}

	globalRedis.RemoveFromQueue(context.Background(), channelName, int(index))

	if ch := cc.getChannelObj(); ch != nil {
		ch.RefreshQueue(nil, globalRedis)
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
	}
}

func (cc *ControlClient) handleReorderQueue(msg *models.Message) {
	if globalRedis == nil {
		return
	}
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	from, _ := payload["from"].(float64)
	to, _ := payload["to"].(float64)

	channelName := cc.GetChannel()
	if channelName == "" {
		return
	}

	globalRedis.ReorderQueue(context.Background(), channelName, int(from), int(to))

	if ch := cc.getChannelObj(); ch != nil {
		ch.RefreshQueue(nil, globalRedis)
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
	}
}

func (cc *ControlClient) handleClearQueue() {
	if globalRedis == nil {
		return
	}
	channelName := cc.GetChannel()
	if channelName == "" {
		return
	}

	globalRedis.ClearQueue(context.Background(), channelName)

	if ch := cc.getChannelObj(); ch != nil {
		ch.RefreshQueue(nil, globalRedis)
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
	}
}

func (cc *ControlClient) handlePlayIndex(msg *models.Message) {
	if globalRedis == nil {
		return
	}
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	index, _ := payload["index"].(float64)

	channelName := cc.GetChannel()
	if channelName == "" {
		return
	}

	globalRedis.SetCurrentIndex(context.Background(), channelName, int(index))

	if ch := cc.getChannelObj(); ch != nil {
		ch.RefreshQueue(nil, globalRedis)
		ch.BroadcastToPlayers(models.CmdPlaySong, ch.GetCurrentSongInfo())
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
	}
}

func (cc *ControlClient) handleSetPlaybackMode(msg *models.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	mode, _ := payload["mode"].(string)
	if mode == "" {
		return
	}

	// Validate mode
	if mode != models.PlaybackModeSequential && mode != models.PlaybackModeLoop && mode != models.PlaybackModeShuffle {
		return
	}

	if ch := cc.getChannelObj(); ch != nil {
		ch.SetPlaybackMode(mode)
		// Broadcast updated state to all controls in the channel
		update := ch.BuildStateUpdate()
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionInitialState,
			"channel": ch.Name,
			"payload": update,
		})
		log.Printf("[ControlClient] Playback mode changed to: %s in channel: %s", mode, ch.Name)
	}
}

func (cc *ControlClient) handleRemovePlayer(msg *models.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	playerID, _ := payload["player_id"].(string)
	if playerID == "" {
		return
	}

	// Find the player in the hub
	pc := cc.hub.GetPlayer(playerID)
	if pc == nil {
		log.Printf("[ControlClient] Player %s not found", playerID)
		return
	}

	// Move the player to the idle channel
	idleCh := cc.manager.GetOrCreateChannel(models.IdleChannelName)
	cc.manager.AddPlayerToChannel(idleCh, pc)
	pc.SetChannel(idleCh)

	// Send stop and leave_channel commands to the player
	pc.SendCommand(models.CmdStop, nil)
	pc.SendCommand("leave_channel", nil)

	// Broadcast updated channel state if the player was in the current channel
	if ch := cc.getChannelObj(); ch != nil {
		ch.RefreshQueue(nil, globalRedis)
		ch.BroadcastJSON(map[string]interface{}{
			"type":    models.MsgTypeStateUpdate,
			"action":  models.ActionPlayerUpdate,
			"payload": map[string]interface{}{"id": playerID, "online": false},
		})
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
	}

	// Update global player/channel lists
	cc.hub.BroadcastPlayersUpdate()
	cc.hub.BroadcastChannelsUpdate()

	log.Printf("[ControlClient] Player %s moved to idle channel", playerID)
}
