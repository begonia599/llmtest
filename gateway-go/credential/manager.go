package credential

import (
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"
	"encoding/json"
	"io"
)

type Credential struct {
	ID             string            `json:"id"`
	AccessToken    string            `json:"access_token"`
	RefreshToken   string            `json:"refresh_token"`
	Expiry         time.Time         `json:"expiry"`
	Disabled       bool              `json:"disabled"`
	Preview        bool              `json:"preview"`
	ModelCooldowns map[string]time.Time `json:"model_cooldowns"`
	CallCount      int64             `json:"call_count"`
	ErrorCount     int64             `json:"error_count"`
	mu             sync.Mutex
}

type Manager struct {
	mu          sync.RWMutex
	credentials []*Credential
	refreshURL  string
	httpClient  *http.Client
}

func NewManager(count int, refreshURL string) *Manager {
	creds := make([]*Credential, count)
	for i := 0; i < count; i++ {
		creds[i] = &Credential{
			ID:             fmt.Sprintf("cred_%03d", i+1),
			AccessToken:    fmt.Sprintf("mock_token_%03d", i+1),
			RefreshToken:   fmt.Sprintf("mock_refresh_%03d", i+1),
			Expiry:         time.Now().Add(time.Duration(60+rand.Intn(3540)) * time.Second), // 1-60 min
			ModelCooldowns: make(map[string]time.Time),
		}
	}

	return &Manager{
		credentials: creds,
		refreshURL:  refreshURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetCredential returns a random available credential, filtering by disabled/cooldown.
func (m *Manager) GetCredential(model string) (*Credential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	var available []*Credential

	for _, c := range m.credentials {
		c.mu.Lock()
		if c.Disabled {
			c.mu.Unlock()
			continue
		}
		// Check model-level cooldown
		if cooldown, ok := c.ModelCooldowns[model]; ok && now.Before(cooldown) {
			c.mu.Unlock()
			continue
		}
		c.mu.Unlock()
		available = append(available, c)
	}

	if len(available) == 0 {
		return nil, fmt.Errorf("no available credentials for model %s", model)
	}

	chosen := available[rand.Intn(len(available))]

	// Check if token needs refresh
	chosen.mu.Lock()
	defer chosen.mu.Unlock()

	if time.Until(chosen.Expiry) <= 120*time.Second {
		if err := m.refreshToken(chosen); err != nil {
			return nil, fmt.Errorf("token refresh failed for %s: %w", chosen.ID, err)
		}
	}

	chosen.CallCount++
	return chosen, nil
}

// PreWarmCredential gets the next available credential in a non-blocking way.
func (m *Manager) PreWarmCredential(model string, exclude string) (*Credential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	var available []*Credential

	for _, c := range m.credentials {
		c.mu.Lock()
		if c.Disabled || c.ID == exclude {
			c.mu.Unlock()
			continue
		}
		if cooldown, ok := c.ModelCooldowns[model]; ok && now.Before(cooldown) {
			c.mu.Unlock()
			continue
		}
		c.mu.Unlock()
		available = append(available, c)
	}

	if len(available) == 0 {
		return nil, fmt.Errorf("no available credentials for model %s (excluding %s)", model, exclude)
	}

	chosen := available[rand.Intn(len(available))]

	chosen.mu.Lock()
	defer chosen.mu.Unlock()

	if time.Until(chosen.Expiry) <= 120*time.Second {
		if err := m.refreshToken(chosen); err != nil {
			return nil, fmt.Errorf("token refresh failed for %s: %w", chosen.ID, err)
		}
	}

	chosen.CallCount++
	return chosen, nil
}

func (m *Manager) refreshToken(cred *Credential) error {
	resp, err := m.httpClient.Post(m.refreshURL, "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		if isPermanentRefreshError(resp.StatusCode) {
			cred.Disabled = true
			return fmt.Errorf("permanent refresh failure (status %d), credential disabled", resp.StatusCode)
		}
		return fmt.Errorf("temporary refresh failure (status %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}

	if token, ok := result["access_token"].(string); ok {
		cred.AccessToken = token
	}
	if expiresIn, ok := result["expires_in"].(float64); ok {
		cred.Expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}

	return nil
}

func isPermanentRefreshError(statusCode int) bool {
	switch statusCode {
	case 400, 401, 403:
		return true
	default:
		return false
	}
}

// RecordError records an error for a credential and handles cooldowns/disabling.
func (m *Manager) RecordError(cred *Credential, statusCode int, model string, cooldownSeconds int) {
	cred.mu.Lock()
	defer cred.mu.Unlock()

	cred.ErrorCount++

	switch statusCode {
	case 429, 503:
		if cooldownSeconds > 0 {
			cred.ModelCooldowns[model] = time.Now().Add(time.Duration(cooldownSeconds) * time.Second)
		} else {
			cred.ModelCooldowns[model] = time.Now().Add(30 * time.Second) // default 30s
		}
	case 400, 403:
		cred.Disabled = true
	}
}

// GetStats returns credential statistics.
func (m *Manager) GetStats() []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]map[string]any, len(m.credentials))
	for i, c := range m.credentials {
		c.mu.Lock()
		stats[i] = map[string]any{
			"id":         c.ID,
			"disabled":   c.Disabled,
			"call_count": c.CallCount,
			"error_count": c.ErrorCount,
			"expiry":     c.Expiry.Format(time.RFC3339),
			"cooldowns":  len(c.ModelCooldowns),
		}
		c.mu.Unlock()
	}
	return stats
}
