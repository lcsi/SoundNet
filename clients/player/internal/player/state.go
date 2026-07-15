package player

import (
	"sync"

	"player/internal/network"
)

// Song represents a playable song (matches server model).
type Song struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Cover    string  `json:"cover"`
	Source   string  `json:"source"`
	Duration float64 `json:"duration"`
	URL      string  `json:"url,omitempty"`
}

// State represents the current state of the player.
type State struct {
	mu sync.RWMutex

	PlayerID  string
	ServerURL string
	Channel   string
	Name      string

	Connected bool
	State     string // "stopped" | "playing" | "paused"

	CurrentSong *Song
	Progress    float64
	Volume      int

	Settings network.PlayerSettings
}

// NewState creates a new player state.
func NewState(playerID, serverURL, name, channel string) *State {
	return &State{
		PlayerID:  playerID,
		ServerURL: serverURL,
		Name:      name,
		Channel:   channel,
		State:     "stopped",
		Volume:    80,
	}
}

// GetPlayerID returns the player ID (thread-safe).
func (s *State) GetPlayerID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.PlayerID
}

// GetVolume returns the current volume (thread-safe).
func (s *State) GetVolume() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Volume
}

// SetState updates the player state (thread-safe).
func (s *State) SetState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
}

// GetState returns the current state string (thread-safe).
func (s *State) GetState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// GetCurrentSong returns the current song (thread-safe).
func (s *State) GetCurrentSong() *Song {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentSong
}

// SetConnected updates the connection state (thread-safe).
func (s *State) SetConnected(connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Connected = connected
}

// SetChannel updates the channel (thread-safe).
func (s *State) SetChannel(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Channel = channel
}

// SetCurrentSong updates the current song and resets progress (thread-safe).
func (s *State) SetCurrentSong(song *Song) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentSong = song
	s.Progress = 0
}

// SetVolume updates the volume (thread-safe).
func (s *State) SetVolume(vol int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Volume = vol
}

// SetProgress updates the playback progress (thread-safe).
func (s *State) SetProgress(progress float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Progress = progress
}

// GetSettings returns the player settings (thread-safe).
func (s *State) GetSettings() network.PlayerSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Settings
}

// SetSettings updates the player settings (thread-safe).
func (s *State) SetSettings(settings network.PlayerSettings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Settings = settings

	// Apply initial volume from settings if set
	if settings.InitialVolume > 0 {
		s.Volume = settings.InitialVolume
	}
}
