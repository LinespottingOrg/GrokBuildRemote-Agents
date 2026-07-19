// Package trace emits correlated, structured hop events for Grok Build Remote.
//
// Every command that travels phone → relay → agent → terminal → relay → phone
// carries the same command_id. Each participant stamps a hop event with that id,
// so a single command can be reconstructed end-to-end from either side.
//
// Events are written as JSONL to ~/.gbr/logs/agent-YYYY-MM-DD.jsonl and, unless
// disabled, mirrored to the relay trace ring buffer so the live viewer can show
// all three hops in one timeline.
//
// Environment:
//
//	GBR_TRACE=0         disable tracing entirely
//	GBR_TRACE_REMOTE=0  local file only, never push to relay
//	GBR_LOG_DIR=<path>  override log directory
package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Hop names — keep stable, the viewer groups on these.
const (
	HopAgentPoll       = "agent.poll"
	HopAgentRecv       = "agent.recv"
	HopAgentInject     = "agent.inject"
	HopAgentCapture    = "agent.capture"
	HopAgentPushOutput = "agent.push_output"
	HopAgentAck        = "agent.ack"
	HopAgentRegister   = "agent.register"
	HopAgentHeartbeat  = "agent.heartbeat"
	HopAgentStart      = "agent.start"
	HopAgentStop       = "agent.stop"
	HopAgentError      = "agent.error"
)

// Event is one hop in a command's journey.
type Event struct {
	TraceID   string `json:"trace_id,omitempty"`
	TS        string `json:"ts"`
	Hop       string `json:"hop"`
	Actor     string `json:"actor"`
	Type      string `json:"type,omitempty"`
	DeviceID  string `json:"device_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	CommandID string `json:"command_id,omitempty"`
	OK        bool   `json:"ok"`
	MS        int64  `json:"ms,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// Config configures a Logger.
type Config struct {
	Dir          string // default ~/.gbr/logs
	Actor        string // default "agent"
	DeviceID     string
	MailboxID    string
	RelayBase    string
	MaxFileBytes int64 // default 10 MiB
}

// Logger writes trace events to disk and (optionally) mirrors them to the relay.
type Logger struct {
	cfg      Config
	mu       sync.Mutex
	file     *os.File
	fileDay  string
	written  int64
	enabled  bool
	remote   bool
	ch       chan Event
	done     chan struct{}
	closeOne sync.Once
	http     *http.Client
}

var (
	defMu   sync.RWMutex
	defLog  *Logger
	maxFile = int64(10 << 20)
)

// DefaultDir is ~/.gbr/logs (honours GBR_LOG_DIR).
func DefaultDir() string {
	if d := strings.TrimSpace(os.Getenv("GBR_LOG_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "gbr-logs")
	}
	return filepath.Join(home, ".gbr", "logs")
}

func envOff(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "0" || v == "false" || v == "off" || v == "no"
}

// New builds a Logger. It never returns nil; on failure it returns a
// disabled logger so callers can emit unconditionally.
func New(cfg Config) *Logger {
	if cfg.Actor == "" {
		cfg.Actor = "agent"
	}
	if cfg.Dir == "" {
		cfg.Dir = DefaultDir()
	}
	if cfg.MaxFileBytes <= 0 {
		cfg.MaxFileBytes = maxFile
	}
	l := &Logger{
		cfg:     cfg,
		enabled: !envOff("GBR_TRACE"),
		remote:  !envOff("GBR_TRACE_REMOTE") && cfg.MailboxID != "" && cfg.RelayBase != "",
		ch:      make(chan Event, 512),
		done:    make(chan struct{}),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
	if !l.enabled {
		close(l.done)
		return l
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		l.enabled = false
		close(l.done)
		return l
	}
	go l.loop()
	return l
}

// Init creates the process-wide default logger.
func Init(cfg Config) *Logger {
	l := New(cfg)
	defMu.Lock()
	defLog = l
	defMu.Unlock()
	return l
}

// Default returns the process-wide logger (may be nil before Init).
func Default() *Logger {
	defMu.RLock()
	defer defMu.RUnlock()
	return defLog
}

// Emit records an event on the default logger. Safe when uninitialised.
func Emit(ev Event) {
	if l := Default(); l != nil {
		l.Emit(ev)
	}
}

// Close flushes and stops the default logger.
func Close() {
	if l := Default(); l != nil {
		l.Close()
	}
}

// Emit records one hop. Non-blocking: if the buffer is full the event is
// dropped rather than slowing the agent's hot path.
func (l *Logger) Emit(ev Event) {
	if l == nil || !l.enabled {
		return
	}
	if ev.TS == "" {
		ev.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if ev.Actor == "" {
		ev.Actor = l.cfg.Actor
	}
	if ev.DeviceID == "" {
		ev.DeviceID = l.cfg.DeviceID
	}
	if ev.TraceID == "" {
		ev.TraceID = ev.CommandID
	}
	select {
	case l.ch <- ev:
	default: // drop rather than block
	}
}

// loop drains the channel, writing to disk immediately and batching remote pushes.
func (l *Logger) loop() {
	defer close(l.done)
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	pending := make([]Event, 0, 32)
	flush := func() {
		if len(pending) == 0 || !l.remote {
			pending = pending[:0]
			return
		}
		batch := make([]Event, len(pending))
		copy(batch, pending)
		pending = pending[:0]
		go l.pushRemote(batch)
	}

	for {
		select {
		case ev, ok := <-l.ch:
			if !ok {
				flush()
				l.closeFile()
				return
			}
			l.writeLine(ev)
			pending = append(pending, ev)
			if len(pending) >= 25 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (l *Logger) writeLine(ev Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	b = append(b, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureFileLocked(); err != nil {
		return
	}
	n, err := l.file.Write(b)
	if err == nil {
		l.written += int64(n)
	}
}

// ensureFileLocked opens/rotates the daily log. Caller holds l.mu.
func (l *Logger) ensureFileLocked() error {
	day := time.Now().UTC().Format("2006-01-02")
	if l.file != nil && l.fileDay == day && l.written < l.cfg.MaxFileBytes {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
		// size-based roll within the same day
		if l.fileDay == day && l.written >= l.cfg.MaxFileBytes {
			cur := l.pathFor(day)
			_ = os.Rename(cur, fmt.Sprintf("%s.%d", cur, time.Now().Unix()))
		}
	}
	f, err := os.OpenFile(l.pathFor(day), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	l.file = f
	l.fileDay = day
	if st, err := f.Stat(); err == nil {
		l.written = st.Size()
	} else {
		l.written = 0
	}
	return nil
}

func (l *Logger) pathFor(day string) string {
	return filepath.Join(l.cfg.Dir, fmt.Sprintf("%s-%s.jsonl", l.cfg.Actor, day))
}

// Path returns today's log file path.
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.pathFor(time.Now().UTC().Format("2006-01-02"))
}

func (l *Logger) closeFile() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// pushRemote mirrors a batch to the relay ring buffer. Best-effort only:
// tracing must never break or slow the control path.
func (l *Logger) pushRemote(events []Event) {
	if len(events) == 0 || l.cfg.MailboxID == "" || l.cfg.RelayBase == "" {
		return
	}
	body, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		return
	}
	u := fmt.Sprintf("%s/v1/mb/%s/trace",
		strings.TrimRight(l.cfg.RelayBase, "/"), url.PathEscape(l.cfg.MailboxID))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.http.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// Close flushes buffered events and closes the log file.
func (l *Logger) Close() {
	if l == nil || !l.enabled {
		return
	}
	l.closeOne.Do(func() { close(l.ch) })
	select {
	case <-l.done:
	case <-time.After(3 * time.Second):
	}
}

// Enabled reports whether tracing is active.
func (l *Logger) Enabled() bool { return l != nil && l.enabled }

// RemoteEnabled reports whether relay mirroring is active.
func (l *Logger) RemoteEnabled() bool { return l != nil && l.remote }
