package player

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"player/internal/audio"
	"player/internal/codec"
	"player/internal/network"
)

// ==========================================================================
// Constants matching server-side models
// ==========================================================================

// Player actions (sent from player to server)
const (
	ActionRegister     = "register"
	ActionStatusUpdate = "status_update"
	ActionFinished     = "finished"
)

// Commands (received from server)
const (
	CmdPlay          = "play"
	CmdPause         = "pause"
	CmdResume        = "resume"
	CmdStop          = "stop"
	CmdPlaySong      = "play_song"
	CmdSeek          = "seek"
	CmdVolume        = "volume"
	CmdJoinChan      = "join_channel"
	CmdUpdInfo       = "update_info"
	CmdRefreshConfig = "refresh_config"
	CmdSetReporting  = "set_reporting"
)

// ==========================================================================
// Player — orchestrates the audio engine, WS client, and API client.
// ==========================================================================

type Player struct {
	state    *State
	ws       *network.WSClient
	api      *network.APIClient
	cacheDir string

	// Status reporting ticker (1s interval)
	statusTicker *time.Ticker
	stopStatus   chan struct{}
	statusWG     sync.WaitGroup

	// Reporting control
	reportingEnabled bool
	reportingMu      sync.RWMutex

	done chan struct{}
}

// NewPlayer creates and initializes a new player.
func NewPlayer(serverURL, playerID, name, channel string) (*Player, error) {
	if playerID == "" {
		playerID = generatePlayerID()
	}
	if name == "" {
		name = playerID
	}

	// Create cache directory for downloaded audio files
	// Uses system temp directory for cross-platform compatibility
	cacheDir := filepath.Join(os.TempDir(), "player-cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir %s: %w", cacheDir, err)
	}

	p := &Player{
		state:      NewState(playerID, serverURL, name, channel),
		ws:         network.NewWSClient(serverURL, 100),
		api:        network.NewAPIClient(serverURL),
		cacheDir:   cacheDir,
		stopStatus: make(chan struct{}),
		done:       make(chan struct{}),
	}

	return p, nil
}

// Run starts the player and blocks until it's stopped.
func (p *Player) Run() error {
	// ---------------------------------------------------------------
	// 1. Initialize audio engine
	// ---------------------------------------------------------------
	log.Printf("[Player] Initializing audio engine...")
	if err := audio.Init(); err != nil {
		return fmt.Errorf("audio init failed: %w", err)
	}
	defer audio.Destroy()

	// Register finished callback
	audio.SetOnFinished(p.onSongFinished)

	// ---------------------------------------------------------------
	// 2. Connect WebSocket
	// ---------------------------------------------------------------
	log.Printf("[Player] Connecting to %s ...", p.state.ServerURL)
	if err := p.ws.Connect("player"); err != nil {
		return fmt.Errorf("websocket connect failed: %w", err)
	}
	defer p.ws.Close()
	// ---------------------------------------------------------------
	// 3. Register with the server
	// ---------------------------------------------------------------
	p.register()

	// ---------------------------------------------------------------
	// 4. Fetch and apply settings from server
	// ---------------------------------------------------------------
	p.refreshConfig()

	// ---------------------------------------------------------------
	// 5. Start status reporting ticker (every 1 second)
	// ---------------------------------------------------------------
	p.statusTicker = time.NewTicker(1 * time.Second)
	p.statusWG.Add(1)
	go p.statusReportLoop()
	// ---------------------------------------------------------------
	// 6. Main event loop
	// ---------------------------------------------------------------
	log.Printf("[Player] Running (ID: %s)", p.state.GetPlayerID())

	// Tick for frequent audio.Update() calls (100ms = 10 times per second)
	updateTicker := time.NewTicker(100 * time.Millisecond)
	defer updateTicker.Stop()
	for {
		select {
		case <-p.done:
			log.Printf("[Player] Shutting down...")
			return nil

		case <-updateTicker.C:
			// Handle device changes frequently (must be called from non-audio thread)
			audio.Update()

		case msg, ok := <-p.ws.Messages():
			if !ok {
				// Channel closed, shut down
				log.Printf("[Player] Shutting down 222...")
				return nil
			}
			p.handleMessage(msg)
		}
	}
}

// Stop gracefully stops the player.
func (p *Player) Stop() {
	select {
	case <-p.done:
		// Already stopped
	default:
		close(p.done)
	}
	p.statusTicker.Stop()
	select {
	case <-p.stopStatus:
	default:
		close(p.stopStatus)
	}
	p.statusWG.Wait()
}

// ==========================================================================
// Message Routing
// ==========================================================================

func (p *Player) handleMessage(msg network.Message) {
	switch msg.Type {
	case "system":
		p.handleSystemMessage(msg)
	case "command":
		p.handleCommand(msg)
	default:
		log.Printf("[Player] Unknown message type: %s", msg.Type)
	}
}

func (p *Player) handleSystemMessage(msg network.Message) {
	switch msg.Action {
	case "reconnected":
		log.Printf("[Player] Reconnected, re-registering...")
		p.register()

	case "disconnected":
		p.state.SetConnected(false)
		log.Printf("[Player] Disconnected")
	}
}

func (p *Player) handleCommand(msg network.Message) {
	log.Printf("[Player] Command: %s", msg.Action)

	switch msg.Action {
	case CmdPlay, CmdResume:
		audio.Play()
		p.state.SetState("playing")

	case CmdPause:
		audio.Pause()
		p.state.SetState("paused")

	case CmdStop:
		audio.Stop()
		p.state.SetState("stopped")

	case CmdPlaySong:
		var song Song
		if err := decodePayload(msg.Payload, &song); err != nil {
			log.Printf("[Player] Invalid play_song payload: %v", err)
			return
		}
		p.playSong(&song)

	case CmdSeek:
		var seekPayload struct {
			Position float64 `json:"position"`
		}
		if err := decodePayload(msg.Payload, &seekPayload); err != nil {
			log.Printf("[Player] Invalid seek payload: %v", err)
			return
		}
		audio.Seek(seekPayload.Position)

	case CmdVolume:
		var volPayload struct {
			Volume int `json:"volume"`
		}
		if err := decodePayload(msg.Payload, &volPayload); err != nil {
			log.Printf("[Player] Invalid volume payload: %v", err)
			return
		}
		p.state.SetVolume(volPayload.Volume)
		audio.SetVolume(volPayload.Volume)

	case CmdJoinChan:
		var chanPayload struct {
			Channel string `json:"channel"`
		}
		if err := decodePayload(msg.Payload, &chanPayload); err != nil {
			log.Printf("[Player] Invalid join_channel payload: %v", err)
			return
		}
		p.state.SetChannel(chanPayload.Channel)
		log.Printf("[Player] Joined channel: %s", chanPayload.Channel)

	case CmdSetReporting:
		var reportPayload struct {
			Enabled bool `json:"enabled"`
		}
		if err := decodePayload(msg.Payload, &reportPayload); err != nil {
			log.Printf("[Player] Invalid set_reporting payload: %v", err)
			return
		}
		p.SetReportingEnabled(reportPayload.Enabled)
		log.Printf("[Player] Reporting enabled: %v", reportPayload.Enabled)

	case CmdRefreshConfig:
		log.Printf("[Player] Refreshing config...")
		p.refreshConfig()
	}
}

// ==========================================================================
// Song Playback
// ==========================================================================

// ==========================================================================
// Config Refresh
// ==========================================================================

// refreshConfig fetches the latest settings from the server and applies them.
func (p *Player) refreshConfig() {
	settings, err := p.api.FetchPlayerSettings(p.state.GetPlayerID())
	if err != nil {
		log.Printf("[Player] Failed to fetch settings: %v", err)
		return
	}

	log.Printf("[Player] Received settings: cache_dir=%s, initial_volume=%d",
		settings.CacheDir, settings.InitialVolume)

	// Apply settings
	p.state.SetSettings(settings)

	// Update cache directory if changed
	if settings.CacheDir != "" {
		if settings.CacheDir != p.cacheDir {
			log.Printf("[Player] Cache dir changed: %s -> %s", p.cacheDir, settings.CacheDir)
			p.cacheDir = settings.CacheDir
			// Create the new cache directory
			if err := os.MkdirAll(p.cacheDir, 0755); err != nil {
				log.Printf("[Player] Failed to create cache dir %s: %v", p.cacheDir, err)
			}
		}
	}

	// Apply volume from settings
	if settings.InitialVolume > 0 {
		p.state.SetVolume(settings.InitialVolume)
		audio.SetVolume(settings.InitialVolume)
		log.Printf("[Player] Volume set to %d", settings.InitialVolume)
	}

	log.Printf("[Player] Config refreshed successfully")
}

// ==========================================================================
// Song Playback
// ==========================================================================

func (p *Player) playSong(song *Song) {
	// 1. Get the actual playable audio URL if not provided in the command
	if song.URL == "" {
		audioURL, err := p.api.FetchSongURL(song.Source, song.ID)
		if err != nil {
			log.Printf("[Player] Failed to get audio URL for %s: %v", song.Title, err)
			return
		}
		song.URL = audioURL
	}

	// 2. Download the audio file to local cache
	localPath, err := p.downloadAudio(song.URL, song.ID)
	if err != nil {
		log.Printf("[Player] Failed to download audio for %s: %v", song.Title, err)
		// Notify the server that playback failed so it can skip to next song
		_ = p.ws.Send(network.Message{
			Type:   "player",
			Action: ActionFinished,
			Payload: map[string]interface{}{
				"player_id": p.state.GetPlayerID(),
				"error":     err.Error(),
			},
		})
		return
	}

	log.Printf("[Player] Playing: %s - %s (local: %s)", song.Artist, song.Title, localPath)

	// 3. Load into miniaudio engine
	if err := audio.LoadFile(localPath); err != nil {
		log.Printf("[Player] Failed to load audio file %s: %v", localPath, err)
		return
	}

	// 4. Set volume
	audio.SetVolume(p.state.GetVolume())

	// 5. Update state
	p.state.SetCurrentSong(song)
	p.state.SetState("playing")

	// 6. Start playback
	audio.Play()
}

// onSongFinished is called by the audio engine (via callback) when
// the current song finishes playing. It notifies the server.
func (p *Player) onSongFinished() {
	log.Printf("[Player] Song finished")
	p.state.SetState("stopped")

	_ = p.ws.Send(network.Message{
		Type:   "player",
		Action: ActionFinished,
		Payload: map[string]interface{}{
			"player_id": p.state.GetPlayerID(),
		},
	})
}

// ==========================================================================
// Audio Download & Caching
// ==========================================================================

// downloadAudio downloads an audio file from the given URL and saves it
// to the local cache directory. Returns a path to a playable file (WAV
// for AAC/M4A sources, or the original format for natively-supported ones).
//
// Flow:
//  1. Cache lookup (see lookupCachedPlayableFile)
//  2. Download .tmp → detect magic bytes → rename to correct extension
//  3. If format needs decoding (.aac or .m4a), decode to .wav
func (p *Player) downloadAudio(audioURL, songID string) (string, error) {
	// Sanitize songID for use as filename
	safeID := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, songID)

	// ── 1. Look for a previously-cached playable file ───────────────
	if cached := lookupCachedPlayableFile(p.cacheDir, safeID); cached != "" {
		return cached, nil
	}

	// ── 2. Look for a previously-cached file that needs decoding ────
	if cached := lookupCachedDecodableFile(p.cacheDir, safeID); cached != "" {
		wavPath, err := codec.DecodeToWAV(cached)
		if err != nil {
			log.Printf("[Player] Failed to decode cached %s: %v, removing bad cache and re-downloading", cached, err)
			// Remove the corrupt cached file and its meta so the re-download is fresh
			os.Remove(cached)
			os.Remove(cached + ".meta")
			// Fall through to re-download
		} else {
			return wavPath, nil
		}
	}

	// ── 3. Download to a temp file ──────────────────────────────────
	tmpPath := filepath.Join(p.cacheDir, safeID+".tmp")
	if err := network.CachedDownload(tmpPath, audioURL); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	// ── 4. Detect real audio format from magic bytes, then rename ───
	renamed := codec.DetectAndRename(tmpPath)
	_ = renamed

	// Log file size and first bytes for debugging
	if fi, err := os.Stat(renamed); err == nil {
		log.Printf("[Player] Downloaded %s (%s)", filepath.Base(renamed), codec.ReadableSize(fi.Size()))
	} else {
		log.Printf("[Player] Downloaded %s (size unknown)", filepath.Base(renamed))
	}

	// ── 5. Decode non-native formats to WAV ────────────────────────
	ext := filepath.Ext(renamed)
	if ext == ".aac" || ext == ".m4a" {
		log.Printf("[Player] Need to decode %s format → WAV", ext)
		wavPath, err := codec.DecodeToWAV(renamed)
		if err != nil {
			log.Printf("[Player] Decode failed: %v, removing unplayable file", err)
			os.Remove(renamed)
			os.Remove(renamed + ".meta")
			return "", fmt.Errorf("decode failed: %w", err)
		}
		// Remove the uncompressed source file (keep only the WAV)
		os.Remove(renamed)
		os.Remove(renamed + ".meta")
		return wavPath, nil
	}

	return renamed, nil
}

// playableExtensions are formats miniaudio can play natively (no decode needed).
// Listed in priority order — .wav first since it's the final decoded form.
var playableExtensions = []string{
	".wav", ".mp3", ".flac", ".ogg",
}

// decodableExtensions are formats that need extra processing to become playable.
var decodableExtensions = []string{
	".aac", ".m4a",
}

// lookupCachedPlayableFile looks for a previously-cached file that miniaudio
// can play directly (WAV, MP3, FLAC, OGG).
func lookupCachedPlayableFile(cacheDir, safeID string) string {
	for _, ext := range playableExtensions {
		path := filepath.Join(cacheDir, safeID+ext)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

// lookupCachedDecodableFile looks for a cached file that can be decoded to WAV.
func lookupCachedDecodableFile(cacheDir, safeID string) string {
	for _, ext := range decodableExtensions {
		path := filepath.Join(cacheDir, safeID+ext)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ==========================================================================
// Reporting Control
// ==========================================================================

// SetReportingEnabled enables or disables status reporting to the server.
func (p *Player) SetReportingEnabled(enabled bool) {
	p.reportingMu.Lock()
	defer p.reportingMu.Unlock()
	p.reportingEnabled = enabled
}

// IsReportingEnabled returns whether status reporting is enabled.
func (p *Player) IsReportingEnabled() bool {
	p.reportingMu.RLock()
	defer p.reportingMu.RUnlock()
	return p.reportingEnabled
}

// ==========================================================================
// Status Reporting (1s ticker)
// ==========================================================================

func (p *Player) statusReportLoop() {
	defer p.statusWG.Done()

	for {
		select {
		case <-p.stopStatus:
			return
		case <-p.statusTicker.C:
			p.sendStatusUpdate()
		}
	}
}

func (p *Player) sendStatusUpdate() {
	if !p.ws.IsConnected() {
		return
	}

	// Don't send status updates if no control clients are connected
	if !p.IsReportingEnabled() {
		return
	}

	song := p.state.GetCurrentSong()
	state := p.state.GetState()
	vol := p.state.GetVolume()

	// Build current_song payload
	var currentSongInfo map[string]interface{}
	if song != nil {
		currentSongInfo = map[string]interface{}{
			"id":     song.ID,
			"title":  song.Title,
			"artist": song.Artist,
			"source": song.Source,
			"url":    song.URL,
		}
	}

	// Get current position from audio engine
	progress := audio.GetPosition()
	p.state.SetProgress(progress)

	msg := network.Message{
		Type:   "player",
		Action: ActionStatusUpdate,
		Payload: map[string]interface{}{
			"player_id":    p.state.GetPlayerID(),
			"playing":      state == "playing",
			"progress":     progress,
			"volume":       vol,
			"current_song": currentSongInfo,
		},
	}

	// Best-effort send
	_ = p.ws.Send(msg)
}

// ==========================================================================
// Registration
// ==========================================================================

func (p *Player) register() {
	p.state.SetConnected(true)

	msg := network.Message{
		Type:   "player",
		Action: ActionRegister,
		Payload: map[string]interface{}{
			"player_id": p.state.GetPlayerID(),
			"name":      p.state.Name,
		},
	}

	if err := p.ws.Send(msg); err != nil {
		log.Printf("[Player] Register send error: %v", err)
	}

	log.Printf("[Player] Registered as: %s", p.state.Name)
}

// ==========================================================================
// Helpers
// ==========================================================================

func generatePlayerID() string {
	now := time.Now().UnixNano()
	return fmt.Sprintf("player-%08x-%04x", uint32(now>>32), uint32(now))
}

func decodePayload(payload interface{}, target interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
