package channel

import (
	"context"
	"math/rand"
	"sync"

	"music-player/db"
	"music-player/models"
	"music-player/redis"
)

var globalRedis *redis.Client

func SetRedis(r *redis.Client) {
	globalRedis = r
}

type Channel struct {
	Name         string
	DisplayName  string
	Players      map[string]PlayerClient // playerID -> client
	Controls     map[ControlClient]bool
	Queue        []models.Song
	CurrentIdx   int
	PlaybackMode string // "sequential", "loop", "shuffle"
	mu           sync.RWMutex
}

type PlayerClient interface {
	GetID() string
	GetName() string
	GetNote() string
	GetVolume() int
	IsPlaying() bool
	GetCurrentSong() *models.Song
	GetProgress() float64
	IsLocalPlayer() bool
	SendCommand(action string, payload interface{}) error
	SendJSON(v interface{}) error
}

type ControlClient interface {
	SendStateUpdate(update *models.StateUpdate) error
	SendJSON(v interface{}) error
}

type Manager struct {
	channels map[string]*Channel
	redis    *redis.Client
	db       *db.DB
	mu       sync.RWMutex
}

func NewManager(r *redis.Client, d *db.DB) *Manager {
	return &Manager{
		channels: make(map[string]*Channel),
		redis:    r,
		db:       d,
	}
}

func (m *Manager) GetOrCreateChannel(name string, displayName ...string) *Channel {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.channels[name]; ok {
		// If a display name is provided and the existing one matches the raw name,
		// update it to the friendly name.
		if len(displayName) > 0 && displayName[0] != "" && ch.DisplayName == name {
			ch.DisplayName = displayName[0]
			if m.db != nil {
				m.db.UpdateChannelDisplayName(name, displayName[0])
			}
		}
		return ch
	}

	// Try to load from DB first (supports persistence across restarts)
	display := name
	if len(displayName) > 0 && displayName[0] != "" {
		display = displayName[0]
	}
	if m.db != nil {
		record, err := m.db.GetChannelRecord(name)
		if err == nil && record != nil {
			display = record.DisplayName
		} else if err == nil {
			// Not in DB yet, create it
			m.db.EnsureChannel(name)
			// Persist the provided display name if any
			if len(displayName) > 0 && displayName[0] != "" {
				m.db.UpdateChannelDisplayName(name, displayName[0])
			}
		}
	}

	ch := &Channel{
		Name:         name,
		DisplayName:  display,
		Players:      make(map[string]PlayerClient),
		Controls:     make(map[ControlClient]bool),
		PlaybackMode: models.PlaybackModeLoop, // default to loop
	}
	m.channels[name] = ch
	return ch
}

func (m *Manager) GetChannel(name string) *Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channels[name]
}

func (m *Manager) ListChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// ChannelListItem is a lightweight channel info for listing.
type ChannelListItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	PlayerCount int    `json:"player_count"`
}

// ListAllChannels returns all channels from both DB (persisted) and memory (active).
// DB-only channels get a zero player count.
func (m *Manager) ListAllChannels() []ChannelListItem {
	// Collect all names: from memory first
	m.mu.RLock()
	memChannels := make(map[string]*Channel, len(m.channels))
	for name, ch := range m.channels {
		memChannels[name] = ch
	}
	m.mu.RUnlock()

	// Also load from DB
	seen := make(map[string]bool)
	for name := range memChannels {
		seen[name] = true
	}

	var dbRecords []*db.ChannelRecord
	if m.db != nil {
		var err error
		dbRecords, err = m.db.ListChannelRecords()
		if err != nil {
			// Log but don't fail; just use memory channels
			dbRecords = nil
		}
	}

	result := make([]ChannelListItem, 0, len(seen)+len(dbRecords))

	// Add in-memory channels first
	for name, ch := range memChannels {
		ch.mu.RLock()
		result = append(result, ChannelListItem{
			Name:        name,
			DisplayName: ch.DisplayName,
			PlayerCount: len(ch.Players),
		})
		ch.mu.RUnlock()
	}

	// Add DB-only channels (not in memory)
	if dbRecords != nil {
		for _, r := range dbRecords {
			if seen[r.Name] {
				continue
			}
			result = append(result, ChannelListItem{
				Name:        r.Name,
				DisplayName: r.DisplayName,
				PlayerCount: 0,
			})
			seen[r.Name] = true
		}
	}

	return result
}

func (m *Manager) RemoveChannel(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.channels, name)
}

// DeleteChannel fully removes a channel: disconnects players/controls, cleans up Redis and DB.
func (m *Manager) DeleteChannel(name string) {
	// 1) Remove from the map under the manager lock
	m.mu.Lock()
	ch, ok := m.channels[name]
	if ok {
		delete(m.channels, name)
	}
	m.mu.Unlock()

	if !ok {
		// Channel was not in memory (e.g. after a server restart it only lives in the
		// DB). Still fall through to delete it from DB and clean up Redis below, so the
		// deletion is not silently skipped.
	} else {
		// 2) Notify players (stop + leave) and controls (channel_deleted)
		ch.mu.RLock()
		for _, p := range ch.Players {
			p.SendCommand("stop", nil)
			p.SendCommand("leave_channel", nil)
			if globalRedis != nil {
				globalRedis.DelPlayerChannel(context.Background(), p.GetID())
			}
		}
		for ctrl := range ch.Controls {
			ctrl.SendJSON(map[string]interface{}{
				"type":    "system",
				"action":  "channel_deleted",
				"payload": map[string]string{"channel": name},
			})
		}
		ch.mu.RUnlock()
	}

	// 3) Clean up Redis queue data
	if globalRedis != nil {
		globalRedis.ClearQueue(context.Background(), name)
	}

	// 4) Delete from DB (works even if the channel was only persisted, not in memory)
	if m.db != nil {
		m.db.DeleteChannel(name)
	}

	// 5) Clean up Redis player→channel mappings that still point to this channel.
	//    Online players were cleared above, but offline players (not in memory)
	//    may still have a stale Redis mapping that would resurrect the channel
	//    on their next reconnect.
	if m.redis != nil && m.db != nil {
		if players, err := m.db.ListPlayers(); err == nil {
			for _, p := range players {
				if chName, _ := m.redis.GetPlayerChannel(context.Background(), p.ID); chName == name {
					m.redis.DelPlayerChannel(context.Background(), p.ID)
				}
			}
		}
	}
}

// AddPlayerToChannel adds a player to a channel and persists to Redis.
func (m *Manager) AddPlayerToChannel(ch *Channel, p PlayerClient) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	// Remove from old channel first (find which channel this player is in)
	m.mu.RLock()
	for _, c := range m.channels {
		if c == ch {
			continue
		}
		c.mu.Lock()
		if _, ok := c.Players[p.GetID()]; ok {
			delete(c.Players, p.GetID())
		}
		c.mu.Unlock()
	}
	m.mu.RUnlock()

	ch.Players[p.GetID()] = p
	m.redis.SetPlayerChannel(context.Background(), p.GetID(), ch.Name)
}

// RemovePlayerFromChannel removes a player from a channel and deletes the Redis mapping.
func (m *Manager) RemovePlayerFromChannel(ch *Channel, playerID string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	delete(ch.Players, playerID)
	m.redis.DelPlayerChannel(context.Background(), playerID)
}

// DisconnectPlayer removes a player from a channel's in-memory map but keeps the Redis mapping
// so the player can re-join the same channel on reconnect.
func (m *Manager) DisconnectPlayer(ch *Channel, playerID string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	delete(ch.Players, playerID)
}

// UpdateChannelName updates the display name of a channel and persists to DB.
func (m *Manager) UpdateChannelName(name, displayName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.channels[name]; ok {
		ch.DisplayName = displayName
	}
	if m.db != nil {
		return m.db.UpdateChannelDisplayName(name, displayName)
	}
	return nil
}

// RemovePlayer removes a player from a channel's in-memory map by ID (if exists).
// This is used during reconnect to ensure stale pointers are cleaned up.
func (ch *Channel) RemovePlayer(playerID string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	delete(ch.Players, playerID)
}

// AddControlToChannel adds a control client to a channel.
func (m *Manager) AddControlToChannel(ch *Channel, ctrl ControlClient) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.Controls[ctrl] = true
}

// RemoveControlFromChannel removes a control client from a channel.
func (m *Manager) RemoveControlFromChannel(ch *Channel, ctrl ControlClient) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	delete(ch.Controls, ctrl)
}

// BroadcastToPlayers sends a message to all players in the channel.
func (ch *Channel) BroadcastToPlayers(action string, payload interface{}) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	for _, p := range ch.Players {
		p.SendCommand(action, payload)
	}
}

// HasControls returns whether there are any control clients connected to this channel.
func (ch *Channel) HasControls() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.Controls) > 0
}

// ControlCount returns the number of control clients connected to this channel.
func (ch *Channel) ControlCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.Controls)
}

// BroadcastJSON sends an arbitrary JSON-serializable message to all controls in the channel.
func (ch *Channel) BroadcastJSON(v interface{}) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	for ctrl := range ch.Controls {
		ctrl.SendJSON(v)
	}
}

// BroadcastToControls sends a state update to all control clients in the channel.
func (ch *Channel) BroadcastToControls(update *models.StateUpdate) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	for ctrl := range ch.Controls {
		ctrl.SendStateUpdate(update)
	}
}

// BuildStateUpdate builds a state update for the channel.
// Local players are excluded from the state update sent to controls.
func (ch *Channel) BuildStateUpdate() *models.StateUpdate {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	players := make([]models.PlayerState, 0, len(ch.Players))
	for _, p := range ch.Players {
		// Skip local players — they should be invisible to other controls
		if p.IsLocalPlayer() {
			continue
		}
		ps := models.PlayerState{
			ID:          p.GetID(),
			Name:        p.GetName(),
			Note:        p.GetNote(),
			Online:      true,
			Volume:      p.GetVolume(),
			Playing:     p.IsPlaying(),
			CurrentSong: p.GetCurrentSong(),
			Progress:    p.GetProgress(),
		}
		players = append(players, ps)
	}

	return &models.StateUpdate{
		Channel:      ch.Name,
		DisplayName:  ch.DisplayName,
		Players:      players,
		Queue:        ch.Queue,
		CurrentIndex: ch.CurrentIdx,
		PlaybackMode: ch.PlaybackMode,
	}
}

// RefreshQueue loads queue from Redis into the channel.
func (ch *Channel) RefreshQueue(ctx context.Context, r *redis.Client) error {
	if r == nil || ctx == nil {
		ctx = context.Background()
	}
	if r == nil {
		return nil
	}
	queue, err := r.GetQueue(ctx, ch.Name)
	if err != nil {
		return err
	}
	idx, err := r.GetCurrentIndex(ctx, ch.Name)
	if err != nil {
		return err
	}
	ch.mu.Lock()
	ch.Queue = queue
	ch.CurrentIdx = idx
	ch.mu.Unlock()
	return nil
}

// MoveToNext advances the current index to the next song based on playback mode.
func (ch *Channel) MoveToNext() {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if len(ch.Queue) == 0 {
		return
	}

	switch ch.PlaybackMode {
	case models.PlaybackModeSequential:
		// Stop at the end of the queue
		if ch.CurrentIdx < len(ch.Queue)-1 {
			ch.CurrentIdx++
		} else {
			// Stay at the last song; caller should check and stop playback
			return
		}
	case models.PlaybackModeShuffle:
		// Pick a random song different from the current one (if possible)
		if len(ch.Queue) == 1 {
			ch.CurrentIdx = 0
		} else {
			newIdx := rand.Intn(len(ch.Queue))
			for newIdx == ch.CurrentIdx {
				newIdx = rand.Intn(len(ch.Queue))
			}
			ch.CurrentIdx = newIdx
		}
	default: // models.PlaybackModeLoop and fallback
		ch.CurrentIdx = (ch.CurrentIdx + 1) % len(ch.Queue)
	}

	// Persist to Redis
	if globalRedis != nil {
		globalRedis.SetCurrentIndex(context.Background(), ch.Name, ch.CurrentIdx)
	}
}

// SetPlaybackMode sets the playback mode for the channel.
func (ch *Channel) SetPlaybackMode(mode string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.PlaybackMode = mode
}

// MoveToPrev moves the current index to the previous song in the queue.
func (ch *Channel) MoveToPrev() {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if len(ch.Queue) == 0 {
		return
	}
	ch.CurrentIdx = (ch.CurrentIdx - 1 + len(ch.Queue)) % len(ch.Queue)
	if globalRedis != nil {
		globalRedis.SetCurrentIndex(context.Background(), ch.Name, ch.CurrentIdx)
	}
}

// GetCurrentSongInfo returns the current song info, or nil if queue is empty.
func (ch *Channel) GetCurrentSongInfo() interface{} {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	if len(ch.Queue) == 0 || ch.CurrentIdx < 0 || ch.CurrentIdx >= len(ch.Queue) {
		return map[string]interface{}{"empty": true}
	}
	return ch.Queue[ch.CurrentIdx]
}
