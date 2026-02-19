package policy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// BundleResponse matches the API response from GET /api/v1/policies/bundle.
type BundleResponse struct {
	Version  string       `json:"version"`
	Hash     string       `json:"hash"`
	Policies []PolicyFile `json:"policies"`
}

// PolicyFile represents a single Cedar policy file in a bundle.
type PolicyFile struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// Fetcher retrieves policy bundles from the Dome API server.
type Fetcher struct {
	httpClient *http.Client
	baseURL    string
	tenantID   string

	mu   sync.Mutex
	etag string
}

// NewFetcher creates a policy bundle fetcher.
func NewFetcher(httpClient *http.Client, baseURL, tenantID string) *Fetcher {
	return &Fetcher{
		httpClient: httpClient,
		baseURL:    baseURL,
		tenantID:   tenantID,
	}
}

// FetchResult contains the result of a bundle fetch.
type FetchResult struct {
	Bundle  *BundleResponse
	Changed bool
}

// Fetch retrieves the latest policy bundle. Returns Changed=false if the
// server returns 304 Not Modified (ETag match).
func (f *Fetcher) Fetch() (*FetchResult, error) {
	url := f.baseURL + "/api/v1/policies/bundle"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("X-Tenant-ID", f.tenantID)

	f.mu.Lock()
	if f.etag != "" {
		req.Header.Set("If-None-Match", f.etag)
	}
	f.mu.Unlock()

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch bundle: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return &FetchResult{Changed: false}, nil

	case http.StatusNotFound:
		// No bundle for this tenant — not an error.
		return &FetchResult{Changed: false}, nil

	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20)) // 50MB limit
		if err != nil {
			return nil, fmt.Errorf("read bundle body: %w", err)
		}

		var bundle BundleResponse
		if err := json.Unmarshal(body, &bundle); err != nil {
			return nil, fmt.Errorf("decode bundle: %w", err)
		}

		// Update ETag for next request.
		if etag := resp.Header.Get("ETag"); etag != "" {
			f.mu.Lock()
			f.etag = etag
			f.mu.Unlock()
		}

		return &FetchResult{Bundle: &bundle, Changed: true}, nil

	default:
		return nil, fmt.Errorf("unexpected status %d from policy bundle API", resp.StatusCode)
	}
}

// Syncer manages periodic policy synchronization and loading.
type Syncer struct {
	fetcher  *Fetcher
	engine   *Engine
	interval time.Duration
	logger   func(msg string, args ...any) // slog-compatible

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewSyncer creates a policy syncer that periodically fetches and loads bundles.
func NewSyncer(fetcher *Fetcher, engine *Engine, interval time.Duration, logger func(string, ...any)) *Syncer {
	if interval == 0 {
		interval = 5 * time.Minute
	}
	return &Syncer{
		fetcher:  fetcher,
		engine:   engine,
		interval: interval,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the sync loop. It does an initial fetch, then polls at interval.
func (s *Syncer) Start() {
	// Initial fetch.
	if err := s.syncOnce(); err != nil {
		s.logger("initial policy sync failed", "error", err)
	}

	s.wg.Add(1)
	go s.loop()
}

// Stop halts the sync loop.
func (s *Syncer) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Syncer) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if err := s.syncOnce(); err != nil {
				s.logger("policy sync failed", "error", err)
			}
		}
	}
}

func (s *Syncer) syncOnce() error {
	result, err := s.fetcher.Fetch()
	if err != nil {
		return err
	}
	if !result.Changed || result.Bundle == nil {
		return nil
	}

	policies := make(map[string]string, len(result.Bundle.Policies))
	for _, p := range result.Bundle.Policies {
		policies[p.Filename] = p.Content
	}

	if err := s.engine.LoadBundle(policies, result.Bundle.Version); err != nil {
		return fmt.Errorf("load bundle: %w", err)
	}

	s.logger("policy bundle updated",
		"version", result.Bundle.Version,
		"policy_count", s.engine.PolicyCount(),
	)
	return nil
}
