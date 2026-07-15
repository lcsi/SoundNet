package network

import (
	"encoding/json"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message represents a WebSocket message.
type Message struct {
	Type    string      `json:"type"`
	Action  string      `json:"action"`
	Channel string      `json:"channel,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
}

// WSClient manages a WebSocket connection to the backend server with
// automatic reconnection and heartbeat support.
type WSClient struct {
	serverAddr  string            // Base server address, e.g. "ws://192.168.1.100:8080"
	conn        *websocket.Conn   // Current WebSocket connection
	mu          sync.RWMutex      // Protects conn access across goroutines

	// Channels for delivering received messages to the player
	messageChan chan Message

	// Control signals
	done     chan struct{} // Closed when the client is shutting down
	stopWait sync.WaitGroup

	// Connection state
	connected bool
	muState   sync.RWMutex

	// Heartbeat
	heartbeatInterval time.Duration
	reconnectDelay    time.Duration
}

// NewWSClient creates a new WebSocket client.
// serverAddr is the base address of the server (e.g. "ws://192.168.1.100:8080").
// messageBufSize is the size of the message channel buffer.
func NewWSClient(serverAddr string, messageBufSize int) *WSClient {
	return &WSClient{
		serverAddr:        serverAddr,
		messageChan:       make(chan Message, messageBufSize),
		done:              make(chan struct{}),
		heartbeatInterval: 5 * time.Second,
		reconnectDelay:    3 * time.Second,
	}
}

// IsConnected returns whether the WebSocket is currently connected.
func (c *WSClient) IsConnected() bool {
	c.muState.RLock()
	defer c.muState.RUnlock()
	return c.connected
}

// setConnected updates the connection state.
func (c *WSClient) setConnected(v bool) {
	c.muState.Lock()
	defer c.muState.Unlock()
	c.connected = v
}

// Messages returns a receive-only channel of parsed messages.
func (c *WSClient) Messages() <-chan Message {
	return c.messageChan
}

// Connect dials the WebSocket server and starts read/write pumps.
// It blocks until the connection is established or an error occurs.
// After connection, it automatically reconnects on disconnect.
func (c *WSClient) Connect(clientType string) error {
	wsURL, _ := url.Parse(c.serverAddr + "/ws")
	q := wsURL.Query()
	q.Set("type", clientType)
	wsURL.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		return err
	}

	c.mu.Lock()
	// Close any existing connection
	if c.conn != nil {
		c.conn.Close()
	}
	c.conn = conn
	c.mu.Unlock()

	c.setConnected(true)

	// Start read pump in background
	c.stopWait.Add(1)
	go c.readPump()

	// Start heartbeat in background
	c.stopWait.Add(1)
	go c.heartbeat()

	return nil
}

// reconnectLoop handles automatic reconnection.
// It blocks until the client is shut down.
func (c *WSClient) reconnectLoop(clientType string) {
	defer c.stopWait.Done()

	for {
		select {
		case <-c.done:
			return
		case <-time.After(c.reconnectDelay):
			// Reconnect
			c.mu.Lock()
			if c.conn != nil {
				c.conn.Close()
				c.conn = nil
			}
			c.mu.Unlock()

			wsURL, _ := url.Parse(c.serverAddr + "/ws")
			q := wsURL.Query()
			q.Set("type", clientType)
			wsURL.RawQuery = q.Encode()

			conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
			if err != nil {
				continue
			}

			c.mu.Lock()
			c.conn = conn
			c.mu.Unlock()

			c.setConnected(true)

			// Restart read pump
			c.stopWait.Add(1)
			go c.readPump()

			log.Printf("[WS] Reconnected")

			// Notify the player about reconnection
			c.messageChan <- Message{
				Type:   "system",
				Action: "reconnected",
			}

			return
		}
	}
}

// readPump reads messages from the WebSocket connection.
func (c *WSClient) readPump() {
	defer c.stopWait.Done()

	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[WS] Read error: %v", err)
			c.setConnected(false)
			c.messageChan <- Message{
				Type:   "system",
				Action: "disconnected",
			}
			// Start reconnection loop
			c.stopWait.Add(1)
			go c.reconnectLoop("player")
			return
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[WS] Invalid message: %v", err)
			continue
		}

		// Respond to server ping with a pong
		if msg.Type == "ping" {
			c.Send(Message{Type: "pong"})
			continue
		}

		// Deliver to player
		select {
		case c.messageChan <- msg:
		default:
			log.Printf("[WS] Message channel full, dropping message: %s/%s", msg.Type, msg.Action)
		}
	}
}

// heartbeat sends periodic ping messages to keep the connection alive.
func (c *WSClient) heartbeat() {
	defer c.stopWait.Done()

	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.Send(Message{Type: "ping"})
		}
	}
}

// Send sends a message over the WebSocket connection.
// Returns an error if not connected or the write fails.
func (c *WSClient) Send(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return nil // Silently fail; we'll reconnect later
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return conn.WriteMessage(websocket.TextMessage, data)
}

// Close shuts down the WebSocket client gracefully.
func (c *WSClient) Close() {
	close(c.done)

	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	c.stopWait.Wait()
	close(c.messageChan)
}
