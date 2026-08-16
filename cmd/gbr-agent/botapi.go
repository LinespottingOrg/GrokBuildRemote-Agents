// Localhost bot HTTP API (v0.5.2).
//
// Binds 127.0.0.1 only so a Grok Build / coding agent on the same PC can
// list sessions, inject text, and read output without speaking gbr/1 envelopes.
// Remote access stays on the relay: GET/POST /v1/mb/:id/bot/* with X-GBR-Key.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
	"github.com/google/uuid"
)

const defaultBotPort = 8788
const maxBotOut = 200
const maxBotBody = 32 * 1024

type botOutputItem struct {
	TS        string `json:"ts"`
	SessionID string `json:"session_id"`
	CommandID string `json:"command_id"`
	Stream    string `json:"stream"`
	Chunk     string `json:"chunk"`
	EOF       bool   `json:"eof"`
	Reason    string `json:"reason,omitempty"`
	Method    string `json:"method,omitempty"`
}

type botServer struct {
	rt        *agentRuntime
	mailboxID string
	key       string
	started   time.Time
	port      int
}

func botPortFromEnv() int {
	s := strings.TrimSpace(os.Getenv("GBR_BOT_PORT"))
	if s == "" {
		return defaultBotPort
	}
	if s == "off" || s == "none" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return defaultBotPort
	}
	return n
}

func (rt *agentRuntime) recordBotOutput(item botOutputItem) {
	if rt == nil {
		return
	}
	if item.TS == "" {
		item.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	rt.outMu.Lock()
	defer rt.outMu.Unlock()
	rt.outLog = append(rt.outLog, item)
	if len(rt.outLog) > maxBotOut {
		rt.outLog = rt.outLog[len(rt.outLog)-maxBotOut:]
	}
}

func (rt *agentRuntime) botOutputs(sessionID, commandID, after string, limit int) []botOutputItem {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	afterMs := int64(0)
	if after != "" {
		if t, err := time.Parse(time.RFC3339Nano, after); err == nil {
			afterMs = t.UnixMilli()
		} else if t, err := time.Parse(time.RFC3339, after); err == nil {
			afterMs = t.UnixMilli()
		}
	}
	rt.outMu.Lock()
	defer rt.outMu.Unlock()
	out := make([]botOutputItem, 0, len(rt.outLog))
	for _, it := range rt.outLog {
		if sessionID != "" && it.SessionID != sessionID {
			continue
		}
		if commandID != "" && it.CommandID != commandID {
			continue
		}
		if afterMs > 0 {
			if t, err := time.Parse(time.RFC3339Nano, it.TS); err == nil && !t.After(time.UnixMilli(afterMs)) {
				continue
			} else if t, err := time.Parse(time.RFC3339, it.TS); err == nil && !t.After(time.UnixMilli(afterMs)) {
				continue
			}
		}
		out = append(out, it)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func (rt *agentRuntime) startBotAPI(ctx context.Context, mailboxID string, port int) {
	if port <= 0 {
		slog.Info("bot api disabled", "hint", "pass -bot-port 8788 or GBR_BOT_PORT=8788")
		return
	}
	s := &botServer{
		rt:        rt,
		mailboxID: mailboxID,
		key:       "",
		started:   time.Now(),
		port:      port,
	}
	if rt.dev != nil {
		s.key = rt.dev.MailboxKey
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	mux.HandleFunc("/health", s.handle)
	mux.HandleFunc("/v1", s.handle)
	mux.HandleFunc("/v1/", s.handle)

	srv := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 8 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		slog.Error("bot api listen failed", "addr", srv.Addr, "err", err)
		return
	}
	slog.Info("bot api listening",
		"url", "http://"+srv.Addr,
		"bind", "127.0.0.1",
		"mailbox", mailboxID,
		"docs", "GET /  ·  GET /v1/sessions  ·  POST /v1/inject",
	)

	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		slog.Error("bot api", "err", err)
	}
}

func (s *botServer) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !isLoopback(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "loopback_only"})
		return
	}
	if !s.authorize(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized", "hint": "X-GBR-Key or Authorization: Bearer"})
		return
	}

	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}

	switch {
	case path == "/" || path == "/health" || path == "/v1":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
			return
		}
		s.writeDiscovery(w)
	case path == "/v1/sessions":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
			return
		}
		s.writeSessions(w)
	case path == "/v1/inject":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
			return
		}
		s.handleInject(w, r)
	case path == "/v1/output":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
			return
		}
		s.writeOutput(w, r)
	case path == "/v1/status":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
			return
		}
		s.writeStatus(w)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
	}
}

func (s *botServer) writeDiscovery(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"service":    "gbr-agent-bot",
		"proto":      "gbr/1",
		"version":    version,
		"mailbox_id": s.mailboxID,
		"bind":       "127.0.0.1",
		"port":       s.port,
		"uptime_s":   int(time.Since(s.started).Seconds()),
		"auth":       []string{"loopback", "optional X-GBR-Key", "Authorization: Bearer <mailbox_key>"},
		"require_key": os.Getenv("GBR_BOT_REQUIRE_KEY") == "1",
		"endpoints": map[string]string{
			"discovery": "GET /  or  GET /v1",
			"sessions":  "GET /v1/sessions",
			"inject":    "POST /v1/inject",
			"output":    "GET /v1/output?session_id=&command_id=&after=&limit=",
			"status":    "GET /v1/status",
		},
		"inject_body": map[string]any{
			"session_id": "grok-build-…",
			"text":       "the prompt to type",
			"submit":     true,
		},
		"relay_bot": s.relayBotURL(),
		"note":      "This port is loopback-only. Remote bots use the relay Bot API with the mailbox key from the phone Settings.",
	})
}

func (s *botServer) relayBotURL() string {
	if s.rt == nil || s.rt.relay == nil || s.mailboxID == "" {
		return ""
	}
	return strings.TrimRight(s.rt.relay.Base(), "/") + "/v1/mb/" + s.mailboxID + "/bot"
}

func (s *botServer) writeSessions(w http.ResponseWriter) {
	var sessions []map[string]any
	if s.rt != nil {
		sessions = s.rt.listSessionPayloads()
	}
	if sessions == nil {
		sessions = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"mailbox_id": s.mailboxID,
		"sessions":   sessions,
		"replace":    true,
		"now":        time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *botServer) handleInject(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBotBody+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read_failed"})
		return
	}
	if len(raw) > maxBotBody {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "text_too_large"})
		return
	}
	var body struct {
		SessionID string `json:"session_id"`
		Session   string `json:"session"`
		Text      string `json:"text"`
		Prompt    string `json:"prompt"`
		NLPrompt  string `json:"nl_prompt"`
		Submit    *bool  `json:"submit"`
		Mode      string `json:"mode"`
		CommandID string `json:"command_id"`
	}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
			return
		}
	}
	text := firstFilled(body.Text, body.Prompt, body.NLPrompt)
	if strings.TrimSpace(text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty_text"})
		return
	}
	sessionID := firstFilled(body.SessionID, body.Session)
	submit := true
	if body.Submit != nil {
		submit = *body.Submit
	}
	commandID := strings.TrimSpace(body.CommandID)
	if commandID == "" {
		commandID = uuid.NewString()
	}

	if s.rt == nil || s.rt.hybrid == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "agent_not_ready"})
		return
	}

	if s.rt.scanner != nil && s.rt.scanner.Registry != nil {
		if sess, ok := s.rt.scanner.Registry.Get(sessionID); ok && sess != nil && sess.HWND != 0 {
			_ = s.rt.hybrid.Bind(sessionID, inject.TerminalWindow{
				HWND:  sess.HWND,
				PID:   uint32(sess.PID),
				Title: sess.Title,
			})
		}
	}
	req := inject.InjectRequest{
		SessionID: sessionID,
		CommandID: commandID,
		Text:      text,
		Submit:    submit,
	}
	injErr := s.rt.hybrid.Inject(sessionID, req)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = s.rt.captureAndPushAfterInject(ctx, s.mailboxID, sessionID, commandID, injErr)
	}()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         injErr == nil,
		"command_id": commandID,
		"session_id": sessionID,
		"queued":     false,
		"local":      true,
		"error":      errString(injErr),
	})
}

func (s *botServer) writeOutput(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	items := s.rt.botOutputs(q.Get("session_id"), q.Get("command_id"), q.Get("after"), limit)
	// Optional live capture when a session is named and the ring is empty.
	if q.Get("live") == "1" && q.Get("session_id") != "" && s.rt != nil && s.rt.hybrid != nil {
		cap, _ := s.rt.hybrid.Capture(q.Get("session_id"))
		if strings.TrimSpace(cap.Text) != "" {
			items = append(items, botOutputItem{
				TS:        time.Now().UTC().Format(time.RFC3339Nano),
				SessionID: q.Get("session_id"),
				Stream:    "stdout",
				Chunk:     trimChunk(cap.Text, 12*1024),
				Reason:    "live",
				Method:    cap.Method,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"mailbox_id": s.mailboxID,
		"items":      items,
		"now":        time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *botServer) writeStatus(w http.ResponseWriter) {
	var sessions []map[string]any
	if s.rt != nil {
		sessions = s.rt.listSessionPayloads()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"mailbox_id":    s.mailboxID,
		"agent_online":  true,
		"agent_version": version,
		"os":            runtime.GOOS,
		"session_count": len(sessions),
		"sessions":      sessions,
		"bot":           "http://127.0.0.1:" + strconv.Itoa(s.port),
		"relay_bot":     s.relayBotURL(),
		"uptime_s":      int(time.Since(s.started).Seconds()),
		"now":           time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *botServer) authorize(r *http.Request) bool {
	presented := strings.TrimSpace(r.Header.Get("X-GBR-Key"))
	if presented == "" {
		auth := r.Header.Get("Authorization")
		if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
			presented = strings.TrimSpace(auth[7:])
		}
	}
	require := os.Getenv("GBR_BOT_REQUIRE_KEY") == "1"
	if presented == "" {
		return !require
	}
	if s.key == "" {
		return true
	}
	return presented == s.key
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func firstFilled(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func cmdBot(args []string) int {
	_ = args
	fmt.Print(`gbr-agent bot API (localhost, loopback only)

While gbr-agent run is up (default port 8788):

  curl -sS http://127.0.0.1:8788/
  curl -sS http://127.0.0.1:8788/v1/sessions
  curl -sS -X POST http://127.0.0.1:8788/v1/inject \
    -H 'Content-Type: application/json' \
    -d '{"session_id":"SESSION","text":"hello from bot","submit":true}'
  curl -sS 'http://127.0.0.1:8788/v1/output?session_id=SESSION&live=1'
  curl -sS http://127.0.0.1:8788/v1/status

Flags / env:
  gbr-agent run -bot-port 8788     default
  gbr-agent run -bot-port 0        disable
  GBR_BOT_PORT=8788
  GBR_BOT_REQUIRE_KEY=1            require X-GBR-Key even on loopback

Remote (phone Settings → Bot API for mailbox id + key):

  curl -sS -H "X-GBR-Key: KEY" \
    https://gbr-relay.ekobrott.workers.dev/v1/mb/MAILBOX/bot/sessions

`)
	return 0
}
