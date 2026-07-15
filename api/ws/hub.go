package ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"music-player/channel"
	"music-player/models"
)

// Hub manages all WebSocket connections.
type Hub struct {
	players      map[string]*PlayerClient
	controls     map[*ControlClient]bool
	unregPlayer  chan *PlayerClient

	mu     sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		players:     make(map[string]*PlayerClient),
		controls:    make(map[*ControlClient]bool),
		unregPlayer: make(chan *PlayerClient, 100),
	}
}

func (h *Hub) RegisterPlayer(pc *PlayerClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.players[pc.id] = pc
	log.Printf("[Hub] Player registered: %s (%s)", pc.id, pc.name)
}

func (h *Hub) UnregisterPlayer(pc *PlayerClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Only delete if the stored pointer matches this exact client.
	// This prevents old connections from deleting the new connection's entry
	// during a reconnect race (old ReadPump defer runs after new handleRegister).
	if existing, ok := h.players[pc.id]; ok && existing == pc {
		delete(h.players, pc.id)
		log.Printf("[Hub] Player unregistered: %s", pc.id)
	}
}

func (h *Hub) GetPlayer(id string) *PlayerClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.players[id]
}

func (h *Hub) GetAllPlayers() []*PlayerClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	players := make([]*PlayerClient, 0, len(h.players))
	for _, p := range h.players {
		players = append(players, p)
	}
	return players
}

// BroadcastToChannelPlayers sends a command to all players in a channel.
func BroadcastToChannelPlayers(ch *channel.Channel, action string, payload interface{}) {
	ch.BroadcastToPlayers(action, payload)
}

// BroadcastToChannelControls sends a state update to all control clients in a channel.
func BroadcastToChannelControls(ch *channel.Channel, update *models.StateUpdate) {
	ch.BroadcastToControls(update)
}

// --- Control client tracking ---

func (h *Hub) RegisterControl(cc *ControlClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.controls[cc] = true
	log.Printf("[Hub] Control registered (%d controls)", len(h.controls))
}

func (h *Hub) UnregisterControl(cc *ControlClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.controls, cc)
	log.Printf("[Hub] Control unregistered (%d controls)", len(h.controls))
}

// BroadcastToAllControls sends a message to every connected control client.
func (h *Hub) BroadcastToAllControls(v interface{}) {
	h.mu.RLock()
	clients := make([]*ControlClient, 0, len(h.controls))
	for cc := range h.controls {
		clients = append(clients, cc)
	}
	h.mu.RUnlock()

	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("[Hub] Broadcast marshal error: %v", err)
		return
	}

	for _, cc := range clients {
		select {
		case cc.send <- data:
		default:
			// Buffer full—use a goroutine to block-send so one slow client
			// doesn't cause other clients to miss system-level messages.
			// Use stopped channel to prevent goroutine leak when WritePump exits.
			go func(cc *ControlClient) {
				select {
				case cc.send <- data:
				case <-cc.stopped:
				}
			}(cc)
		}
	}
}

// BroadcastPlayersUpdate fetches the current player list and pushes it to all controls.
func (h *Hub) BroadcastPlayersUpdate() {
	if globalDB == nil {
		return
	}

	players, err := globalDB.ListPlayers()
	if err != nil {
		log.Printf("[Hub] Failed to list players for broadcast: %v", err)
		return
	}

	type playerResp struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Note      string `json:"note"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Online    bool   `json:"online"`
		Channel   string `json:"channel,omitempty"`
	}

	resp := make([]playerResp, 0, len(players))
	for _, p := range players {
		// Skip local players — they should be invisible to other controls
		if len(p.ID) > 6 && p.ID[:6] == "local-" {
			continue
		}
		pr := playerResp{
			ID:        p.ID,
			Name:      p.Name,
			Note:      p.Note,
			CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if pc := h.GetPlayer(p.ID); pc != nil {
			pr.Online = true
		}
		if globalRedis != nil {
			if ch, _ := globalRedis.GetPlayerChannel(context.Background(), p.ID); ch != "" {
				pr.Channel = ch
			}
		}
		resp = append(resp, pr)
	}

	msg := map[string]interface{}{
		"type":    models.MsgTypeSystem,
		"action":  models.ActionPlayersUpdate,
		"payload": resp,
	}

	h.BroadcastToAllControls(msg)
}

// BroadcastChannelsUpdate fetches the current channel list and pushes it to all controls.
func (h *Hub) BroadcastChannelsUpdate() {
	if globalChannelMgr == nil {
		return
	}

	items := globalChannelMgr.ListAllChannels()

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

	msg := map[string]interface{}{
		"type":    models.MsgTypeSystem,
		"action":  models.ActionChannelsUpdate,
		"payload": resp,
	}

	h.BroadcastToAllControls(msg)
}

// WriteJSON is a helper to write JSON to a WebSocket connection.
func WriteJSON(conn *websocket.Conn, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}
