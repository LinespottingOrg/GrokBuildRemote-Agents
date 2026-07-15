// Package relay is the durable GBR mailbox transport (Cloudflare Worker + KV).
// Phone and PC never connect; both push/poll envelopes by mailbox id.
package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// DefaultBase is overridden by GBR_RELAY_URL.
const DefaultBase = "https://gbr-relay.ekobrott.workers.dev"

// Client talks to the GBR relay Worker.
type Client struct {
	base       string
	httpClient *http.Client
	mu         sync.Mutex
	after      time.Time
}

// New builds a relay client. base empty → env GBR_RELAY_URL or DefaultBase.
func New(base string, timeout time.Duration) *Client {
	if base == "" {
		base = strings.TrimSpace(os.Getenv("GBR_RELAY_URL"))
	}
	if base == "" {
		base = DefaultBase
	}
	base = strings.TrimRight(base, "/")
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		base: base,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Base returns the configured relay origin.
func (c *Client) Base() string { return c.base }

// Push posts one envelope JSON object to the mailbox.
func (c *Client) Push(ctx context.Context, mailboxID string, envelope any) error {
	raw, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/v1/mb/%s/push", c.base, url.PathEscape(mailboxID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("relay push: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("relay push HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

// PollResult is a batch of raw envelope objects.
type PollResult struct {
	OK         bool              `json:"ok"`
	Envelopes  []json.RawMessage `json:"envelopes"`
	Now        string            `json:"now"`
	Error      string            `json:"error,omitempty"`
}

// Poll fetches envelopes newer than the client's cursor.
func (c *Client) Poll(ctx context.Context, mailboxID, deviceID, role string) ([]json.RawMessage, error) {
	c.mu.Lock()
	after := c.after
	c.mu.Unlock()

	q := url.Values{}
	if !after.IsZero() {
		q.Set("after", after.UTC().Format(time.RFC3339Nano))
	}
	if deviceID != "" {
		q.Set("for", deviceID)
	}
	if role == "" {
		role = "agent"
	}
	q.Set("role", role)

	u := fmt.Sprintf("%s/v1/mb/%s/poll?%s", c.base, url.PathEscape(mailboxID), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("relay poll: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("relay poll HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var pr PollResult
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("relay poll decode: %w", err)
	}
	if pr.Now != "" {
		if t, err := time.Parse(time.RFC3339Nano, pr.Now); err == nil {
			c.mu.Lock()
			c.after = t
			c.mu.Unlock()
		} else if t, err := time.Parse(time.RFC3339, pr.Now); err == nil {
			c.mu.Lock()
			c.after = t
			c.mu.Unlock()
		}
	}
	return pr.Envelopes, nil
}

// Pair registers pairing metadata on the relay.
func (c *Client) Pair(ctx context.Context, mailboxID, code, deviceID, deviceName string) error {
	payload := map[string]string{
		"pairing_code": code,
		"device_id":    deviceID,
		"device_name":  deviceName,
	}
	raw, _ := json.Marshal(payload)
	u := fmt.Sprintf("%s/v1/mb/%s/pair", c.base, url.PathEscape(mailboxID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("relay pair: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("relay pair HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

// Ack asks the relay to drop envelopes by command_id (after successful handle).
func (c *Client) Ack(ctx context.Context, mailboxID string, commandIDs []string) error {
	if len(commandIDs) == 0 {
		return nil
	}
	payload := map[string]any{"command_ids": commandIDs}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/v1/mb/%s/ack", c.base, url.PathEscape(mailboxID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("relay ack: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("relay ack HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

// Health checks the relay.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("relay health HTTP %d", resp.StatusCode)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
