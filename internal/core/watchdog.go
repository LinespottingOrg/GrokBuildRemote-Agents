package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Health quality tokens — same vocabulary as ChrisP grok-remote-hub
// (ok / stale / zombie) so Grok Bot and the website can talk about both.
const (
	HealthOK     = "ok"
	HealthStale  = "stale"
	HealthZombie = "zombie"
	HealthDown   = "down"
)

// CompanionRemote is a LAN/Tailscale Grok-remote that may share this PC.
// GBR does not replace them; the watchdog only reports whether they are up
// so the phone app + Grok Bot can see the whole desk.
type CompanionRemote struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Repo    string `json:"repo"`
	Impl    string `json:"impl"`
	Port    int    `json:"port"`
	Path    string `json:"path"`
	Quality string `json:"quality"`
	Detail  string `json:"detail,omitempty"`
	URL     string `json:"url,omitempty"`
	Online  bool   `json:"online"`
}

// DefaultCompanions are the three community remotes the site FAQ markets.
// Probe is loopback-only. Timeouts stay short so /health never blocks Grok Bot.
func DefaultCompanions() []CompanionRemote {
	return []CompanionRemote{
		{
			ID:   "amnibro",
			Name: "Amnibro grok-remote",
			Repo: "https://github.com/Amnibro/grok-remote",
			Impl: "amnibro",
			Port: 2421,
			Path: "/",
		},
		{
			ID:   "farina",
			Name: "daniel-farina/grok-remote",
			Repo: "https://github.com/daniel-farina/grok-remote",
			Impl: "farina",
			Port: 7910,
			Path: "/api/health",
		},
		{
			ID:   "chrisp",
			Name: "ChrisP-Builds grok-remote-hub",
			Repo: "https://github.com/ChrisP-Builds/grok-remote-hub",
			Impl: "chrisp",
			Port: 8787,
			Path: "/health",
		},
	}
}

// WatchdogSnapshot is what GET /health and GET /v1/health return.
type WatchdogSnapshot struct {
	Quality     string            `json:"quality"`
	Relay       string            `json:"relay_quality"`
	RosterAgeS  int               `json:"roster_age_s"`
	LastRoster  string            `json:"last_roster,omitempty"`
	Class       string            `json:"class"`
	Hostname    string            `json:"hostname,omitempty"`
	Companions  []CompanionRemote `json:"companions"`
	Note        string            `json:"note"`
	CheckedAt   string            `json:"checked_at"`
}

// Watchdog tracks last roster publish so Grok Bot can tell ok vs stale vs zombie.
type Watchdog struct {
	mu         sync.Mutex
	lastRoster time.Time
	lastRelay  time.Time
	relayOK    bool
	probe      func(ctx context.Context, port int, path string) (bool, string)
}

// GlobalWatchdog is the process-wide instance used by gbr-agent run.
var GlobalWatchdog = NewWatchdog()

// NewWatchdog returns a watchdog with the default loopback HTTP probe.
func NewWatchdog() *Watchdog {
	return &Watchdog{probe: defaultProbe}
}

// TouchRoster records a successful session roster publish.
func (w *Watchdog) TouchRoster() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.lastRoster = time.Now()
	w.mu.Unlock()
}

// TouchRelay records a relay health/heartbeat result.
func (w *Watchdog) TouchRelay(ok bool) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.lastRelay = time.Now()
	w.relayOK = ok
	w.mu.Unlock()
}

// Snapshot builds the health document. Companion probes are optional
// (skipCompanions=true for cheap CLI status).
func (w *Watchdog) Snapshot(skipCompanions bool) WatchdogSnapshot {
	if w == nil {
		w = NewWatchdog()
	}
	now := time.Now()
	w.mu.Lock()
	lastRoster := w.lastRoster
	lastRelay := w.lastRelay
	relayOK := w.relayOK
	probe := w.probe
	w.mu.Unlock()

	age := 0
	if !lastRoster.IsZero() {
		age = int(now.Sub(lastRoster).Seconds())
	}

	relayQ := HealthDown
	switch {
	case relayOK && !lastRelay.IsZero() && now.Sub(lastRelay) < 45*time.Second:
		relayQ = HealthOK
	case relayOK:
		relayQ = HealthStale
	case !lastRelay.IsZero():
		relayQ = HealthZombie
	}

	quality := HealthOK
	switch {
	case lastRoster.IsZero():
		quality = HealthStale
	case age > 120:
		quality = HealthZombie
	case age > 30:
		quality = HealthStale
	}
	if relayQ == HealthDown || relayQ == HealthZombie {
		if quality == HealthOK {
			quality = HealthStale
		}
	}

	snap := WatchdogSnapshot{
		Quality:    quality,
		Relay:      relayQ,
		RosterAgeS: age,
		Class:      DetectClass(),
		Hostname:   HostnameBest(),
		Note:       "GitHub HTTPS relay is the control plane. Companion remotes are optional LAN/Tailscale hubs on this PC.",
		CheckedAt:  now.UTC().Format(time.RFC3339),
		Companions: []CompanionRemote{},
	}
	if !lastRoster.IsZero() {
		snap.LastRoster = lastRoster.UTC().Format(time.RFC3339)
	}
	if skipCompanions {
		return snap
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	comps := DefaultCompanions()
	for i := range comps {
		ok, detail := probe(ctx, comps[i].Port, comps[i].Path)
		comps[i].Online = ok
		comps[i].Detail = detail
		comps[i].URL = fmt.Sprintf("http://127.0.0.1:%d%s", comps[i].Port, comps[i].Path)
		if ok {
			comps[i].Quality = HealthOK
		} else {
			comps[i].Quality = HealthDown
		}
	}
	snap.Companions = comps
	return snap
}

func defaultProbe(ctx context.Context, port int, path string) (bool, string) {
	if path == "" {
		path = "/"
	}
	d := net.Dialer{Timeout: 250 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false, "not listening"
	}
	_ = conn.Close()

	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return true, "tcp up"
	}
	client := &http.Client{Timeout: 400 * time.Millisecond}
	res, err := client.Do(req)
	if err != nil {
		return true, "tcp up, http " + err.Error()
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 2048))
	if res.StatusCode >= 200 && res.StatusCode < 500 {
		return true, fmt.Sprintf("http %d", res.StatusCode)
	}
	return true, fmt.Sprintf("http %d", res.StatusCode)
}

// CompanionSummary is a one-line CLI print.
func CompanionSummary(comps []CompanionRemote) string {
	var parts []string
	for _, c := range comps {
		state := "down"
		if c.Online {
			state = "up"
		}
		parts = append(parts, fmt.Sprintf("%s:%d=%s", c.ID, c.Port, state))
	}
	return strings.Join(parts, " ")
}
