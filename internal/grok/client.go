package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client talks to the xAI OpenAI-compatible Chat Completions API and
// implements Mode B mailbox (post/poll [GBR] JSON envelopes).
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	log        *slog.Logger
	model      string

	mu            sync.Mutex
	conversation  string // Mode B mailbox conversation id
	seenMsgIDs    map[string]struct{}
	lastPollAt    time.Time
	pollCursor    int // message index high-water when API returns ordered history
}

// ClientOption configures Client.
type ClientOption func(*Client)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(cl *Client) { cl.httpClient = c }
}

// WithLogger sets structured logger (defaults to slog.Default).
func WithLogger(l *slog.Logger) ClientOption {
	return func(cl *Client) { cl.log = l }
}

// WithModel sets chat model (default grok-2-latest or server default via empty).
func WithModel(m string) ClientOption {
	return func(cl *Client) { cl.model = m }
}

// NewClient builds an xAI chat client. baseURL should be like https://api.x.ai/v1.
func NewClient(baseURL, apiKey string, httpTimeout time.Duration, opts ...ClientOption) *Client {
	if httpTimeout <= 0 {
		httpTimeout = 60 * time.Second
	}
	baseURL = strings.TrimRight(baseURL, "/")
	cl := &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
		log:        slog.Default(),
		model:      "grok-2-latest",
		seenMsgIDs: make(map[string]struct{}),
	}
	for _, o := range opts {
		o(cl)
	}
	if cl.log == nil {
		cl.log = slog.Default()
	}
	return cl
}

// SetConversation binds Mode B mailbox conversation id.
func (c *Client) SetConversation(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conversation != id {
		c.conversation = id
		c.seenMsgIDs = make(map[string]struct{})
		c.pollCursor = 0
	}
}

// Conversation returns the current mailbox conversation id.
func (c *Client) Conversation() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conversation
}

// --- OpenAI-compatible wire types ---

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	// Stream false: agent needs full JSON envelopes.
	Stream bool `json:"stream"`
}

type chatChoice struct {
	Index   int         `json:"index"`
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	ID      string       `json:"id"`
	Choices []chatChoice `json:"choices"`
	Error   *apiError    `json:"error,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func (e *apiError) Error() string {
	if e == nil {
		return "unknown api error"
	}
	return fmt.Sprintf("xai api: %s (%s/%s)", e.Message, e.Type, e.Code)
}

// ChatCompletion posts a one-shot chat completion (no durable mailbox state on server).
func (c *Client) ChatCompletion(ctx context.Context, messages []chatMessage) (*chatResponse, error) {
	reqBody := chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", "gbr-agent/0.1")

	c.log.Debug("chat completion request", "url", url, "model", c.model, "msgs", len(messages))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chat completion: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("chat completion HTTP %d: %s", resp.StatusCode, truncate(string(body), 400))
	}

	var out chatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}
	if out.Error != nil {
		return nil, out.Error
	}
	return &out, nil
}

// PostEnvelope appends a [GBR] envelope into the mailbox conversation via chat completions.
// Mode B: agent (and mobile) send JSON-only messages tagged [GBR].
// conversationID is stored client-side; xAI chat is stateless so we carry a system
// anchor that names the mailbox id for correlation on both sides.
func (c *Client) PostEnvelope(ctx context.Context, env *Envelope) error {
	if env == nil {
		return fmt.Errorf("nil envelope")
	}
	if err := env.Validate(); err != nil {
		return err
	}
	tagged, err := env.SerializeTagged()
	if err != nil {
		return err
	}

	c.mu.Lock()
	conv := c.conversation
	c.mu.Unlock()
	if conv == "" {
		return fmt.Errorf("mailbox conversation not set; pair first")
	}

	messages := []chatMessage{
		{
			Role: "system",
			Content: fmt.Sprintf(
				"GBR mailbox %s. Acknowledge with a single line OK. Do not invent [GBR] envelopes.",
				conv,
			),
		},
		{Role: "user", Content: tagged},
	}

	resp, err := c.ChatCompletion(ctx, messages)
	if err != nil {
		return fmt.Errorf("post envelope: %w", err)
	}
	c.log.Info("posted envelope",
		"type", env.Type,
		"device_id", env.DeviceID,
		"session_id", env.SessionID,
		"command_id", env.CommandID,
		"resp_id", resp.ID,
	)
	return nil
}

// PollEnvelopes performs a Mode B poll for new [GBR] messages addressed to this device.
//
// Day-1 strategy (chat completions only, no durable thread API):
//  1. Ask the model to list any pending [GBR] envelopes for device_id / conversation.
//  2. Parse [GBR] JSON from the response.
//  3. Deduplicate by command_id + type + ts fingerprint.
//
// When a real messages.list API exists, swap the body of this method without changing callers.
func (c *Client) PollEnvelopes(ctx context.Context, deviceID string) ([]*Envelope, error) {
	c.mu.Lock()
	conv := c.conversation
	c.mu.Unlock()
	if conv == "" {
		return nil, fmt.Errorf("mailbox conversation not set; pair first")
	}
	if deviceID == "" {
		return nil, fmt.Errorf("device_id required for poll")
	}

	prompt := fmt.Sprintf(
		"You are the GBR Mode B mailbox relay for conversation %s. "+
			"Return ONLY lines of the form %s {json} for any pending envelopes "+
			"where device_id is %q or type is pair. If none, reply with exactly: NONE",
		conv, GBRTag, deviceID,
	)

	resp, err := c.ChatCompletion(ctx, []chatMessage{
		{Role: "system", Content: "GBR mailbox poll. Output only [GBR] JSON envelopes or NONE."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("poll mailbox: %w", err)
	}

	c.mu.Lock()
	c.lastPollAt = time.Now().UTC()
	c.mu.Unlock()

	if len(resp.Choices) == 0 {
		return nil, nil
	}
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" || strings.EqualFold(content, "NONE") {
		return nil, nil
	}

	envs, err := ExtractTaggedEnvelopes(content)
	if err != nil {
		return nil, err
	}

	var fresh []*Envelope
	for _, e := range envs {
		// Address filter: inject/list/pair for us, or device_id empty (broadcast pair).
		if e.DeviceID != "" && e.DeviceID != deviceID {
			continue
		}
		fp := envelopeFingerprint(e)
		c.mu.Lock()
		_, seen := c.seenMsgIDs[fp]
		if !seen {
			c.seenMsgIDs[fp] = struct{}{}
			fresh = append(fresh, e)
		}
		// Bound seen set growth
		if len(c.seenMsgIDs) > 4096 {
			c.seenMsgIDs = map[string]struct{}{fp: {}}
		}
		c.mu.Unlock()
	}
	return fresh, nil
}

// PostAndPoll is a convenience: post an envelope then poll once for replies.
func (c *Client) PostAndPoll(ctx context.Context, env *Envelope, deviceID string) ([]*Envelope, error) {
	if err := c.PostEnvelope(ctx, env); err != nil {
		return nil, err
	}
	return c.PollEnvelopes(ctx, deviceID)
}

// StartMailboxLoop polls every interval until ctx is cancelled.
// handler is called for each new envelope; errors from handler are logged, not fatal.
func (c *Client) StartMailboxLoop(ctx context.Context, deviceID string, interval time.Duration, handler func(context.Context, *Envelope) error) error {
	if interval < 2*time.Second {
		interval = 2 * time.Second
	}
	if interval > 5*time.Second {
		// Protocol recommends 2–5s; allow slightly higher but cap warn.
		c.log.Warn("poll interval above protocol 5s recommendation", "interval", interval)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Immediate first poll
	if err := c.pollOnce(ctx, deviceID, handler); err != nil {
		c.log.Error("mailbox poll failed", "err", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.pollOnce(ctx, deviceID, handler); err != nil {
				c.log.Error("mailbox poll failed", "err", err)
			}
		}
	}
}

func (c *Client) pollOnce(ctx context.Context, deviceID string, handler func(context.Context, *Envelope) error) error {
	pollCtx, cancel := context.WithTimeout(ctx, c.httpClient.Timeout)
	defer cancel()
	envs, err := c.PollEnvelopes(pollCtx, deviceID)
	if err != nil {
		return err
	}
	for _, e := range envs {
		if handler == nil {
			continue
		}
		hctx, hcancel := context.WithTimeout(ctx, c.httpClient.Timeout)
		err := handler(hctx, e)
		hcancel()
		if err != nil {
			c.log.Error("envelope handler", "type", e.Type, "err", err)
		}
	}
	return nil
}

// CreateMailboxConversation seeds a new Mode B conversation id (local UUID-style).
// The id is correlated via system prompts on both agent and mobile until durable threads exist.
func CreateMailboxConversation() string {
	// Compact time-based + random-ish id without importing uuid here (caller may use google/uuid).
	return fmt.Sprintf("gbr-mb-%d", time.Now().UTC().UnixNano())
}

func envelopeFingerprint(e *Envelope) string {
	if e == nil {
		return ""
	}
	if e.CommandID != "" {
		return e.Type + ":" + e.CommandID
	}
	return fmt.Sprintf("%s:%s:%s:%s", e.Type, e.DeviceID, e.SessionID, e.TS.UTC().Format(time.RFC3339Nano))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
