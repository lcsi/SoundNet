package network

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ==========================================================================
// HTTP Caching — saves audio files locally so songs are not re-downloaded.
//
// For each audio file we save a companion ".meta" JSON sidecar that records
// the original URL, ETag and Last-Modified response headers. On subsequent
// requests we send If-None-Match / If-Modified-Since.  If the server
// responds with 304 Not Modified we reuse the local copy.
// ==========================================================================

// cacheMeta is stored as {audioPath}.meta alongside each downloaded file.
type cacheMeta struct {
	URL          string `json:"url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	ContentLen   int64  `json:"content_length,omitempty"`
	DownloadedAt int64  `json:"downloaded_at"` // Unix timestamp
}

func metaPath(audioPath string) string {
	return audioPath + ".meta"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readMeta(path string) (*cacheMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var m cacheMeta
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func writeMeta(path string, m *cacheMeta) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

// CachedDownload downloads url to localPath, using HTTP conditional requests
// so unchanged files are not fetched again.
//
// It creates a {localPath}.meta sidecar to remember ETag / Last-Modified.
// If the server responds 304 Not Modified the existing file is reused.
// If the server returns a new 200 the file and meta are updated.
func CachedDownload(localPath, url string) error {
	mp := metaPath(localPath)
	client := &http.Client{Timeout: 60 * time.Second}

	// ── 1. Build request (possibly conditional) ──────────────────────
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}

	// If we have a cached file AND a matching meta, send conditional headers.
	haveCache := fileExists(localPath)
	if haveCache {
		if m, err := readMeta(mp); err == nil && m.URL == url {
			if m.ETag != "" {
				req.Header.Set("If-None-Match", m.ETag)
			}
			if m.LastModified != "" {
				req.Header.Set("If-Modified-Since", m.LastModified)
			}
		} else {
			// Meta is missing or url changed — ignore stale cache
			haveCache = false
		}
	}

	// ── 2. Execute request ──────────────────────────────────────────
	resp, err := client.Do(req)
	if err != nil {
		// Network error — fall back to cached file if we have one
		if haveCache {
			return nil
		}
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	// ── 3. 304 Not Modified → cache is still valid ──────────────────
	if resp.StatusCode == http.StatusNotModified && haveCache {
		return nil
	}

	// ── 4. Any non-200 status → error (unless we have a fallback) ───
	if resp.StatusCode != http.StatusOK {
		// Retry-able? Just use cache as fallback.
		if haveCache {
			return nil
		}
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	// ── 5. 200 OK → write new file + metadata ───────────────────────
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// Validate that the downloaded file is large enough to be meaningful audio
	if written < 1024 { // Less than 1KB is almost certainly not valid audio
		os.Remove(localPath)
		return fmt.Errorf("downloaded file too small (%d bytes), likely invalid", written)
	}

	// Persist metadata
	meta := &cacheMeta{
		URL:          url,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		ContentLen:   written,
		DownloadedAt: time.Now().Unix(),
	}
	if err := writeMeta(mp, meta); err != nil {
		// Best-effort: remove the downloaded file if we can't save metadata
		os.Remove(localPath)
		return fmt.Errorf("write meta: %w", err)
	}

	return nil
}

// DownloadFile is a simple non-caching download (kept for other uses).
func DownloadFile(localPath, url string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(localPath), err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}
