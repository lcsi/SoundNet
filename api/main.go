package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"music-player/alarm"
	"music-player/channel"
	"music-player/config"
	"music-player/db"
	"music-player/models"
	redislib "music-player/redis"
	ws "music-player/ws"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for development
		},
	}
	globalDB      *db.DB
	globalRedis   *redislib.Client
	globalHub     *ws.Hub
	channelMgr    *channel.Manager
	alarmEngine   *alarm.Engine
    cfg           *config.Config
)

func main() {
	cfg = config.Load()

	// Initialize SQLite
	var err error
	globalDB, err = db.New(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer globalDB.Close()
	log.Println("[DB] SQLite initialized")

	// Initialize Redis
	globalRedis, err = redislib.New(cfg.RedisAddr)
	if err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}
	defer globalRedis.Close()
	log.Println("[Redis] Connected to Redis")

	// Initialize Hub
	globalHub = ws.NewHub()

	// Initialize Channel Manager
	channelMgr = channel.NewManager(globalRedis, globalDB)

	// Set global redis, db, and channel manager references for ws and channel packages
	ws.SetRedis(globalRedis)
	ws.SetDB(globalDB)
	ws.SetChannelManager(channelMgr)
	channel.SetRedis(globalRedis)

	// Initialize Alarm Engine
	alarmEngine = alarm.New(globalRedis, globalDB, channelMgr)
	alarmEngine.SetCallbacks(onAlarmStart, onSleepTimer)
	alarmEngine.Start()

	ws.SetAlarmEngine(alarmEngine)

	// Setup HTTP routes
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/ws", handleWebSocket)

	// REST API endpoints
	mux.HandleFunc("/api/players", handlePlayers)
	mux.HandleFunc("/api/players/", handlePlayerByID)
	mux.HandleFunc("/api/channels", handleChannels)
	mux.HandleFunc("/api/channels/", handleChannelByName)
	mux.HandleFunc("/api/alarms/channel/", handleChannelAlarms)
	mux.HandleFunc("/api/alarms/", handleAlarmByID)
	mux.HandleFunc("/api/search", handleSearch)
	mux.HandleFunc("/api/playlist", handlePlaylistList)
	mux.HandleFunc("/api/playlist/detail", handlePlaylistDetail)
	mux.HandleFunc("/api/song/", handleSongURL)
	mux.HandleFunc("/api/lyric", handleLyric)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("[Server] Starting on %s", addr)
	server := &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WebSocket handler - routes connections to either control or player based on URL query
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}

	// Determine client type from query parameter (?type=control or ?type=player)
	clientType := r.URL.Query().Get("type")
	if clientType == "" {
		// Default to control
		clientType = "control"
	}

	switch clientType {
	case "player":
		playerClient := ws.NewPlayerClient(globalHub, conn)
		go playerClient.WritePump()
		go playerClient.ReadPump()
		log.Printf("[WS] Player client connected")

	case "control":
		controlClient := ws.NewControlClient(globalHub, conn, channelMgr)
		globalHub.RegisterControl(controlClient)
		go controlClient.WritePump()
		go controlClient.ReadPump()
		log.Printf("[WS] Control client connected")

	default:
		log.Printf("[WS] Unknown client type: %s", clientType)
		conn.Close()
	}
}

// HTTP Handlers

func handlePlayers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List all players
		players, err := globalDB.ListPlayers()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Enrich with online status
		type playerResp struct {
			*models.Player
			Online  bool   `json:"online"`
			Channel string `json:"channel,omitempty"`
		}
		resp := make([]playerResp, 0, len(players))
		for _, p := range players {
			// Skip local players — they should not appear in the player list
			if len(p.ID) > 6 && p.ID[:6] == "local-" {
				continue
			}
			pr := playerResp{Player: p}
			pc := globalHub.GetPlayer(p.ID)
			if pc != nil {
				pr.Online = true
			}
			// Get channel from Redis
			if ch, _ := globalRedis.GetPlayerChannel(context.Background(), p.ID); ch != "" {
				pr.Channel = ch
			}
			resp = append(resp, pr)
		}
		writeJSON(w, resp)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handlePlayerByID(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/players/{id} or /api/players/{id}/assign-channel
	path := r.URL.Path[len("/api/players/"):]
	parts := splitPath(path)
	if len(parts) == 0 {
		http.Error(w, "Missing player ID", http.StatusBadRequest)
		return
	}
	id := parts[0]

	// Sub-route: /api/players/{id}/assign-channel
	if len(parts) >= 2 && parts[1] == "assign-channel" {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Channel string `json:"channel"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.Channel == "" {
			http.Error(w, "Missing channel name", http.StatusBadRequest)
			return
		}
		pc := globalHub.GetPlayer(id)
		if pc == nil {
			http.Error(w, "Player not online", http.StatusNotFound)
			return
		}
		ch := channelMgr.GetOrCreateChannel(req.Channel)
		channelMgr.AddPlayerToChannel(ch, pc)
		pc.SetChannel(ch)
		pc.SendCommand("join_channel", map[string]interface{}{
			"channel": req.Channel,
		})

		// Push a player_update so controls see the new player immediately
		pc.BroadcastPlayerUpdate()

		globalHub.BroadcastChannelsUpdate()

		w.WriteHeader(http.StatusOK)
		writeJSON(w, map[string]string{"status": "ok"})
		return
	}

	// Sub-route: /api/players/{id}/refresh-config
	if len(parts) >= 2 && parts[1] == "refresh-config" {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		pc := globalHub.GetPlayer(id)
		if pc == nil {
			http.Error(w, "Player not online", http.StatusNotFound)
			return
		}

		// Send refresh_config command to the player
		pc.SendCommand("refresh_config", map[string]interface{}{})

		w.WriteHeader(http.StatusOK)
		writeJSON(w, map[string]string{"status": "ok"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, err := globalDB.GetPlayer(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if p == nil {
			http.Error(w, "Player not found", http.StatusNotFound)
			return
		}
		writeJSON(w, p)

	case http.MethodPut:
		var req struct {
			Name     string              `json:"name"`
			Note     string              `json:"note"`
			Settings *models.PlayerSettings `json:"settings,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Get current player to preserve existing settings if not provided
		currentPlayer, err := globalDB.GetPlayer(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if currentPlayer == nil {
			http.Error(w, "Player not found", http.StatusNotFound)
			return
		}

		settings := currentPlayer.Settings
		if req.Settings != nil {
			settings = *req.Settings
		}

		if err := globalDB.UpdatePlayer(id, req.Name, req.Note, settings); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if pc := globalHub.GetPlayer(id); pc != nil {
			pc.SendCommand("update_info", map[string]interface{}{
				"name":     req.Name,
				"note":     req.Note,
				"settings": settings,
			})
		}
		writeJSON(w, map[string]string{"status": "ok"})

	case http.MethodDelete:
		// Delete player from database
		if err := globalDB.DeletePlayer(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// If player is online, disconnect it and remove from channel
		if pc := globalHub.GetPlayer(id); pc != nil {
			pc.SendCommand("stop", nil)
			pc.SendCommand("leave_channel", nil)
			// Remove from channel via Redis
			if globalRedis != nil {
				if chName, _ := globalRedis.GetPlayerChannel(context.Background(), id); chName != "" {
					if ch := channelMgr.GetChannel(chName); ch != nil {
						channelMgr.RemovePlayerFromChannel(ch, id)
					}
				}
			}
		}
		// Clean up Redis
		if globalRedis != nil {
			globalRedis.DelPlayerChannel(context.Background(), id)
		}
		// Broadcast updates
		globalHub.BroadcastPlayersUpdate()
		globalHub.BroadcastChannelsUpdate()
		writeJSON(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items := channelMgr.ListAllChannels()
		type channelInfo struct {
			Name         string `json:"name"`
			DisplayName  string `json:"display_name"`
			PlayerCount  int    `json:"player_count"`
			ControlCount int    `json:"control_count"`
		}
		resp := make([]channelInfo, 0, len(items))
		for _, item := range items {
			resp = append(resp, channelInfo{
				Name:         item.Name,
				DisplayName:  item.DisplayName,
				PlayerCount:  item.PlayerCount,
				ControlCount: 0,
			})
		}
		writeJSON(w, resp)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// splitPath splits a URL path string by "/" and returns non-empty parts.
func splitPath(p string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if i > start {
				parts = append(parts, p[start:i])
			}
			start = i + 1
		}
	}
	if start < len(p) {
		parts = append(parts, p[start:])
	}
	return parts
}

func handleChannelByName(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/channels/"):]
	if name == "" {
		http.Error(w, "Missing channel name", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		ch := channelMgr.GetChannel(name)
		if ch == nil {
			http.Error(w, "Channel not found", http.StatusNotFound)
			return
		}
		// Refresh queue from Redis
		ch.RefreshQueue(context.Background(), globalRedis)
		update := ch.BuildStateUpdate()
		writeJSON(w, update)

	case http.MethodPut:
		var req struct {
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.DisplayName == "" {
			http.Error(w, "display_name is required", http.StatusBadRequest)
			return
		}
		if err := channelMgr.UpdateChannelName(name, req.DisplayName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})

	case http.MethodDelete:
		channelMgr.DeleteChannel(name)
		globalHub.BroadcastChannelsUpdate()
		globalHub.BroadcastPlayersUpdate()
		writeJSON(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "Missing query parameter q", http.StatusBadRequest)
		return
	}

    sources := r.URL.Query().Get("sources")
    searchType := r.URL.Query().Get("type")

	// Proxy to external music search API
	apiURL := fmt.Sprintf("%s/api/search?keyword=%s&sources=%s", cfg.MusicApi, url.QueryEscape(q), url.QueryEscape(sources))
	if searchType != "" {
		apiURL += "&type=" + url.QueryEscape(searchType)
	}
	resp, err := http.Get(apiURL)
	if err != nil {
		log.Printf("[Search] API request failed: %v", err)
		http.Error(w, "Search service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Playlist search — forward raw data
	if searchType == "playlist" {
		var playlistResult struct {
			Success bool                     `json:"success"`
			Data    []map[string]interface{} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&playlistResult); err != nil {
			log.Printf("[Search] Failed to decode playlist response: %v", err)
			http.Error(w, "Failed to parse search results", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]interface{}{
			"results": playlistResult.Data,
			"query":   q,
		})
		return
	}

	var extResult struct {
		Success bool `json:"success"`
		Data    []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Artist   string `json:"artist"`
			Album    string `json:"album"`
			Cover    string `json:"cover"`
			Source   string `json:"source"`
			Duration int    `json:"duration"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&extResult); err != nil {
		log.Printf("[Search] Failed to decode API response: %v", err)
		http.Error(w, "Failed to parse search results", http.StatusInternalServerError)
		return
	}

	// Map to our Song model
	songs := make([]models.Song, 0, len(extResult.Data))
	for _, item := range extResult.Data {
		songs = append(songs, models.Song{
			ID:       item.ID,
			Title:    item.Name,
			Artist:   item.Artist,
			Album:    item.Album,
			Cover:    item.Cover,
			Source:   item.Source,
			Duration: item.Duration,
		})
	}

	writeJSON(w, map[string]interface{}{
		"results": songs,
		"query":   q,
	})
}

func handlePlaylistList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		http.Error(w, "Missing query parameter keyword", http.StatusBadRequest)
		return
	}
	sources := r.URL.Query().Get("sources")

	apiURL := fmt.Sprintf("%s/api/search?keyword=%s&type=playlist&sources=%s", cfg.MusicApi, url.QueryEscape(keyword), url.QueryEscape(sources))

	resp, err := http.Get(apiURL)
	if err != nil {
		log.Printf("[PlaylistList] API request failed: %v", err)
		http.Error(w, "Playlist search service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var extResult struct {
		Success bool                     `json:"success"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&extResult); err != nil {
		log.Printf("[PlaylistList] Failed to decode API response: %v", err)
		http.Error(w, "Failed to parse playlist results", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"results": extResult.Data,
	})
}

func handlePlaylistDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	source := r.URL.Query().Get("source")
	if id == "" || source == "" {
		http.Error(w, "Missing parameters: need id and source", http.StatusBadRequest)
		return
	}

	// Proxy to external playlist detail API
	apiURL := fmt.Sprintf("%s/api/playlist/detail?id=%s&source=%s", cfg.MusicApi, 
		url.QueryEscape(id), url.QueryEscape(source))

	resp, err := http.Get(apiURL)
	if err != nil {
		log.Printf("[PlaylistDetail] API request failed: %v", err)
		http.Error(w, "Playlist detail service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var extResult struct {
		Success bool `json:"success"`
		Data    []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Artist   string `json:"artist"`
			Album    string `json:"album"`
			Cover    string `json:"cover"`
			Source   string `json:"source"`
			Duration int    `json:"duration"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&extResult); err != nil {
		log.Printf("[PlaylistDetail] Failed to decode API response: %v", err)
		http.Error(w, "Failed to parse playlist detail", http.StatusInternalServerError)
		return
	}

	// Map to our Song model
	songs := make([]models.Song, 0, len(extResult.Data))
	for _, item := range extResult.Data {
		songs = append(songs, models.Song{
			ID:       item.ID,
			Title:    item.Name,
			Artist:   item.Artist,
			Album:    item.Album,
			Cover:    item.Cover,
			Source:   item.Source,
			Duration: item.Duration,
		})
	}

	writeJSON(w, map[string]interface{}{
		"results": songs,
	})
}

func handleSongURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	source := r.URL.Query().Get("source")
	musicID := r.URL.Query().Get("musicId")
	quality := r.URL.Query().Get("quality")
	if quality == "" {
		quality = "128k"
	}

	if source == "" || musicID == "" {
		// Try URL path format: /api/song/{source}/{id}
		path := strings.TrimPrefix(r.URL.Path, "/api/song/")
		parts := splitPath(path)
		if len(parts) >= 2 {
			source = parts[0]
			musicID = parts[1]
		} else if len(parts) == 1 {
			musicID = parts[0]
		}
	}

	if source == "" || musicID == "" {
		http.Error(w, "Missing parameters: need source and musicId", http.StatusBadRequest)
		return
	}

	// Proxy to external music URL API
	apiURL := fmt.Sprintf("%s/api/url?source=%s&musicId=%s&quality=%s",
		cfg.MusicApi, url.QueryEscape(source), url.QueryEscape(musicID), url.QueryEscape(quality))

	resp, err := http.Get(apiURL)
	if err != nil {
		log.Printf("[SongURL] API request failed: %v", err)
		http.Error(w, "Song URL service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var urlResult struct {
		URL string `json:"url"`
		Via string `json:"via"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&urlResult); err != nil {
		log.Printf("[SongURL] Failed to decode API response: %v", err)
		http.Error(w, "Failed to parse song URL", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"id":   musicID,
		"url":  urlResult.URL,
		"via":  urlResult.Via,
	})
}

func handleLyric(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	source := r.URL.Query().Get("source")
	if id == "" || source == "" {
		http.Error(w, "Missing parameters: need id and source", http.StatusBadRequest)
		return
	}

	// Proxy to external lyrics API
	apiURL := fmt.Sprintf("%s/api/lyric?id=%s&source=%s",
		cfg.MusicApi, url.QueryEscape(id), url.QueryEscape(source))

	resp, err := http.Get(apiURL)
	if err != nil {
		log.Printf("[Lyric] API request failed: %v", err)
		http.Error(w, "Lyric service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward the response as-is
	var lyricResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&lyricResult); err != nil {
		log.Printf("[Lyric] Failed to decode API response: %v", err)
		http.Error(w, "Failed to parse lyric response", http.StatusInternalServerError)
		return
	}

	writeJSON(w, lyricResult)
}

// ─── Alarm Callbacks ──────────────────────────────────────

// onAlarmStart is called when an alarm_start timer triggers.
func onAlarmStart(alarm *models.Alarm, ch *channel.Channel) {
	log.Printf("[Alarm] onAlarmStart: alarm=%s channel=%s strategy=%s content=%s", alarm.ID, alarm.Channel, alarm.ConflictStrategy, alarm.ContentMode)

	// Helper: start playback from the first song in the queue
	playFromQueueStart := func() {
		if globalRedis != nil {
			globalRedis.SetCurrentIndex(context.Background(), alarm.Channel, 0)
		}
		ch.RefreshQueue(nil, globalRedis)
		if len(ch.Queue) > 0 {
			ch.BroadcastToPlayers(models.CmdPlaySong, ch.Queue[0])
		} else {
			ch.BroadcastToPlayers(models.CmdPlay, nil)
		}
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
		log.Printf("[Alarm] Started playback from queue start")
	}

	// Helper: continue from current position without modifying queue
	continueCurrent := func() {
		ch.RefreshQueue(nil, globalRedis)
		if len(ch.Queue) > 0 {
			ch.BroadcastToPlayers(models.CmdPlay, nil)
		} else {
			ch.BroadcastToPlayers(models.CmdPlay, nil)
		}
		ch.BroadcastJSON(map[string]interface{}{
			"type":   models.MsgTypeStateUpdate,
			"action": models.ActionQueueRefresh,
		})
		log.Printf("[Alarm] Continuing current queue playback")
	}

	switch alarm.ConflictStrategy {
	case models.AlarmConflictSkip:
		log.Printf("[Alarm] Conflict strategy=skip, skipping alarm %s", alarm.ID)
		return

	case models.AlarmConflictQueue:
		log.Printf("[Alarm] Conflict strategy=queue, playing content for alarm %s", alarm.ID)

		switch alarm.ContentMode {
		case models.AlarmContentContinueCurrent:
			continueCurrent()
		case models.AlarmContentQueueStart:
			playFromQueueStart()
		case models.AlarmContentSpecificSongs:
			log.Printf("[Alarm] specific_songs not fully implemented — playing from queue start")
			playFromQueueStart()
		case models.AlarmContentShuffle:
			log.Printf("[Alarm] shuffle not fully implemented — playing from queue start")
			playFromQueueStart()
		}

	case models.AlarmConflictReplace:
		log.Printf("[Alarm] Conflict strategy=replace, replacing content for alarm %s", alarm.ID)

		switch alarm.ContentMode {
		case models.AlarmContentContinueCurrent:
			// Just stop current and resume — don't touch queue
			ch.BroadcastToPlayers(models.CmdStop, nil)
			continueCurrent()
		case models.AlarmContentQueueStart:
			ch.BroadcastToPlayers(models.CmdStop, nil)
			playFromQueueStart()
		case models.AlarmContentSpecificSongs:
			if globalRedis != nil {
				globalRedis.ClearQueue(context.Background(), alarm.Channel)
			}
			ch.BroadcastToPlayers(models.CmdStop, nil)
			log.Printf("[Alarm] specific_songs not fully implemented — queue cleared")
		case models.AlarmContentShuffle:
			if globalRedis != nil {
				globalRedis.ClearQueue(context.Background(), alarm.Channel)
			}
			ch.BroadcastToPlayers(models.CmdStop, nil)
			log.Printf("[Alarm] shuffle not fully implemented — queue cleared")
		}
	}

	// Broadcast fade-in if configured
	if alarm.FadeInSeconds > 0 {
		if alarm.ConflictStrategy == models.AlarmConflictQueue {
			ch.BroadcastToPlayers(models.CmdVolume, map[string]interface{}{"volume": 0})
		}
		ch.BroadcastToPlayers(models.ActionFadeVolume, map[string]interface{}{
			"from":             0,
			"to":               80,
			"duration_seconds": alarm.FadeInSeconds,
		})
	}
}

// onSleepTimer is called when a sleep_timer triggers.
func onSleepTimer(alarm *models.Alarm, ch *channel.Channel) {
	log.Printf("[Alarm] onSleepTimer: alarm=%s channel=%s action=%s", alarm.ID, alarm.Channel, alarm.StopAction)

	// Execute the stop action
	switch alarm.StopAction {
	case models.AlarmStopActionStop:
		ch.BroadcastToPlayers(models.CmdPause, nil)
	case models.AlarmStopActionPause:
		ch.BroadcastToPlayers(models.CmdPause, nil)
	case models.AlarmStopActionStopAndClear:
		ch.BroadcastToPlayers(models.CmdStop, nil)
		if globalRedis != nil {
			globalRedis.ClearQueue(context.Background(), alarm.Channel)
		}
	}

	// Handle fade-out
	if alarm.FadeOutSeconds > 0 {
		ch.BroadcastToPlayers(models.ActionFadeVolume, map[string]interface{}{
			"from":             80,
			"to":               0,
			"duration_seconds": alarm.FadeOutSeconds,
		})
	}

	// Broadcast queue refresh
	ch.BroadcastJSON(map[string]interface{}{
		"type":   models.MsgTypeStateUpdate,
		"action": models.ActionQueueRefresh,
	})
}

// ─── Alarm HTTP Handlers ─────────────────────────────────

func handleChannelAlarms(w http.ResponseWriter, r *http.Request) {
	// Parse channel name from URL: /api/alarms/channel/{name}
	channelName := strings.TrimPrefix(r.URL.Path, "/api/alarms/channel/")
	channelName = strings.TrimSuffix(channelName, "/")
	if channelName == "" {
		http.Error(w, "Missing channel name", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// GET /api/channels/:name/alarms — list all alarms for channel
		alarms, err := alarmEngine.GetChannelAlarms(channelName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if alarms == nil {
			alarms = []*models.Alarm{}
		}

		// Also get active timers for countdown display
		timers, err := alarmEngine.GetActiveTimers(channelName)
		if err != nil {
			timers = []map[string]interface{}{}
		}

		writeJSON(w, map[string]interface{}{
			"alarms":        alarms,
			"active_timers": timers,
		})

	case http.MethodPost:
		// POST /api/channels/:name/alarms — create a new alarm
		var alarm models.Alarm
		if err := json.NewDecoder(r.Body).Decode(&alarm); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		alarm.Channel = channelName
		if err := alarmEngine.ScheduleAlarm(&alarm); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Broadcast alarm sync to channel controls
		if ch := channelMgr.GetChannel(channelName); ch != nil {
			alarms, _ := alarmEngine.GetChannelAlarms(channelName)
			timers, _ := alarmEngine.GetActiveTimers(channelName)
			ch.BroadcastJSON(map[string]interface{}{
				"type":   models.MsgTypeStateUpdate,
				"action": models.ActionAlarmSync,
				"payload": map[string]interface{}{
					"alarms":        alarms,
					"active_timers": timers,
				},
			})
		}

		writeJSON(w, alarm)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAlarmByID(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/alarms/{id} or /api/alarms/{id}/toggle
	path := strings.TrimPrefix(r.URL.Path, "/api/alarms/")
	parts := splitPath(path)
	if len(parts) == 0 {
		http.Error(w, "Missing alarm ID", http.StatusBadRequest)
		return
	}
	alarmID := parts[0]

	// Sub-route: /api/alarms/{id}/toggle
	if len(parts) >= 2 && parts[1] == "toggle" {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		alarm, err := alarmEngine.ToggleAlarm(alarmID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Broadcast alarm sync after toggle
		if alarm != nil {
			if ch := channelMgr.GetChannel(alarm.Channel); ch != nil {
				alarms, _ := alarmEngine.GetChannelAlarms(alarm.Channel)
				timers, _ := alarmEngine.GetActiveTimers(alarm.Channel)
				ch.BroadcastJSON(map[string]interface{}{
					"type":   models.MsgTypeStateUpdate,
					"action": models.ActionAlarmSync,
					"payload": map[string]interface{}{
						"alarms":        alarms,
						"active_timers": timers,
					},
				})
			}
		}
		writeJSON(w, alarm)
		return
	}

	// Sub-route: /api/alarms/{id}/reset
	if len(parts) >= 2 && parts[1] == "reset" {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		alarm, err := alarmEngine.ResetAlarm(alarmID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Broadcast alarm sync
		if alarm != nil {
			if ch := channelMgr.GetChannel(alarm.Channel); ch != nil {
				alarms, _ := alarmEngine.GetChannelAlarms(alarm.Channel)
				timers, _ := alarmEngine.GetActiveTimers(alarm.Channel)
				ch.BroadcastJSON(map[string]interface{}{
					"type":   models.MsgTypeStateUpdate,
					"action": models.ActionAlarmSync,
					"payload": map[string]interface{}{
						"alarms":        alarms,
						"active_timers": timers,
					},
				})
			}
		}
		writeJSON(w, alarm)
		return
	}

	switch r.Method {
	case http.MethodPut:
		// PUT /api/alarms/{id} — update alarm
		var alarm models.Alarm
		if err := json.NewDecoder(r.Body).Decode(&alarm); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		alarm.ID = alarmID
		if err := alarmEngine.UpdateAlarm(&alarm); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Broadcast alarm sync
		if ch := channelMgr.GetChannel(alarm.Channel); ch != nil {
			alarms, _ := alarmEngine.GetChannelAlarms(alarm.Channel)
			timers, _ := alarmEngine.GetActiveTimers(alarm.Channel)
			ch.BroadcastJSON(map[string]interface{}{
				"type":   models.MsgTypeStateUpdate,
				"action": models.ActionAlarmSync,
				"payload": map[string]interface{}{
					"alarms":        alarms,
					"active_timers": timers,
				},
			})
		}

		writeJSON(w, alarm)

	case http.MethodDelete:
		// DELETE /api/alarms/{id} — delete alarm
		// Load alarm first to get channel for broadcast
		ctx := context.Background()
		alarm, err := globalRedis.GetAlarmHash(ctx, alarmID)
		if err == nil && alarm != nil {
			// Broadcast alarm sync before deletion
			if ch := channelMgr.GetChannel(alarm.Channel); ch != nil {
				alarms, _ := alarmEngine.GetChannelAlarms(alarm.Channel)
				ch.BroadcastJSON(map[string]interface{}{
					"type":   models.MsgTypeStateUpdate,
					"action": models.ActionAlarmSync,
					"payload": map[string]interface{}{
						"alarms":        alarms,
						"active_timers": []map[string]interface{}{},
						"deleted_id":    alarmID,
					},
				})
			}
		}

		if err := alarmEngine.DeleteAlarm(alarmID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
