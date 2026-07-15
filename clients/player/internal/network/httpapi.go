package network

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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

// APIClient handles REST API calls to the backend server.
type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAPIClient creates an API client pointing at the given backend server.
// serverAddr should be the base URL of the server, e.g. "http://192.168.1.100:8080".
func NewAPIClient(serverAddr string) *APIClient {
	addr := strings.Replace(serverAddr, "ws://", "http://", 1)
	addr = strings.Replace(addr, "wss://", "https://", 1)
	return &APIClient{
		baseURL: addr,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// songURLResponse matches the server's GET /api/song/url response.
type songURLResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
	Via string `json:"via"`
}

// FetchSongURL retrieves the actual playable audio URL for a song.
// It calls: GET /api/song/url?source=<source>&musicId=<id>
//
// Returns the audio URL string, or an error if the request fails or the URL is empty.
func (c *APIClient) FetchSongURL(source, musicID string) (string, error) {
	u, _ := url.Parse(c.baseURL + "/api/song/url")
	q := u.Query()
	q.Set("source", source)
	q.Set("musicId", musicID)
	q.Set("quality", "128k")
	u.RawQuery = q.Encode()

	resp, err := c.httpClient.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("failed to fetch song URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("song URL API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result songURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode song URL response: %w", err)
	}

	if result.URL == "" {
		return "", fmt.Errorf("song URL is empty (id=%s)", musicID)
	}

	return result.URL, nil
}

// PlayerSettings represents the settings for a player (matches server model).
type PlayerSettings struct {
	CacheDir      string `json:"cache_dir,omitempty"`
	InitialVolume int    `json:"initial_volume,omitempty"`
}

// playerResponse matches the server's GET /api/players/:id response.
type playerResponse struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Note     string         `json:"note"`
	Settings PlayerSettings `json:"settings"`
}

// FetchPlayerSettings retrieves the player settings from the server.
// It calls: GET /api/players/:id
func (c *APIClient) FetchPlayerSettings(playerID string) (PlayerSettings, error) {
	u := c.baseURL + "/api/players/" + playerID

	resp, err := c.httpClient.Get(u)
	if err != nil {
		return PlayerSettings{}, fmt.Errorf("failed to fetch player settings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return PlayerSettings{}, fmt.Errorf("player API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result playerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return PlayerSettings{}, fmt.Errorf("failed to decode player response: %w", err)
	}

	return result.Settings, nil
}

// SearchResult matches the server's GET /api/search response.
type SearchResult struct {
	Results []Song `json:"results"`
	Query   string `json:"query"`
}

// Search queries the server's search API.
// It calls: GET /api/search?q=<keyword>&sources=<sources>
func (c *APIClient) Search(keyword, sources string) ([]Song, error) {
	u, _ := url.Parse(c.baseURL + "/api/search")
	q := u.Query()
	q.Set("q", keyword)
	if sources != "" {
		q.Set("sources", sources)
	}
	u.RawQuery = q.Encode()

	resp, err := c.httpClient.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	return result.Results, nil
}
