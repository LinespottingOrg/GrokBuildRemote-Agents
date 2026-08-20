// Session open / idle-wait / result / lock / tasks — the 0.5.4 chain
// shared by Grok bot (HTTP) and Claude Cowork (MCP).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/core"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/session"
	"github.com/google/uuid"
)

func (s *botServer) handleOpen(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBotBody+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read_failed"})
		return
	}
	var body struct {
		Device   string `json:"device"`
		DeviceID string `json:"device_id"`
		Session  string `json:"session_id"`
		CWD      string `json:"cwd"`
		Resume   string `json:"resume"`
		Command  string `json:"command"`
		Title    string `json:"title"`
		Holder   string `json:"holder"`
		Goal     string `json:"goal"`
		TTLSec   int    `json:"ttl_s"`
		Steal    bool   `json:"steal"`
		Notify   *bool  `json:"notify_phone"`
		Attach   *bool  `json:"attach"`
	}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
			return
		}
	}
	want := firstFilled(body.Device, body.DeviceID, r.URL.Query().Get("device"))
	if f, _ := core.LoadFleet(); f != nil {
		if d, ok := f.Get(want); ok && d.Kind == "relay" {
			s.proxyRemote(w, d, http.MethodPost, "/sessions/open", body)
			return
		}
	}
	if s.rt == nil || s.rt.hybrid == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "agent_not_ready"})
		return
	}

	sid := strings.TrimSpace(body.Session)
	attachOnly := body.Attach != nil && *body.Attach
	if sid != "" && (attachOnly || s.sessionExists(sid)) {
		s.finishOpen(w, inject.OpenResult{
			SessionID: sid,
			Attached:  true,
			Method:    "existing",
			CWD:       body.CWD,
			Note:      "attached existing session — did not spawn",
		}, body.Holder, body.Goal, body.TTLSec, body.Steal, body.Notify)
		return
	}

	res, err := s.rt.hybrid.OpenOrAttach(inject.OpenRequest{
		SessionID: sid,
		CWD:       body.CWD,
		Resume:    body.Resume,
		Command:   body.Command,
		Title:     body.Title,
		Holder:    body.Holder,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.registerOpened(res, firstFilled(body.Title, "Grok Build"))
	s.finishOpen(w, res, body.Holder, body.Goal, body.TTLSec, body.Steal, body.Notify)
}

func (s *botServer) sessionExists(id string) bool {
	if id == "" || id == "session" {
		return false
	}
	if s.rt.scanner != nil && s.rt.scanner.Registry != nil {
		if _, ok := s.rt.scanner.Registry.Get(id); ok {
			return true
		}
	}
	if s.rt.hybrid != nil && s.rt.hybrid.PTY != nil && s.rt.hybrid.PTY.Get(id) != nil {
		return true
	}
	return false
}

func (s *botServer) registerOpened(res inject.OpenResult, title string) {
	if s.rt == nil || s.rt.scanner == nil {
		return
	}
	cwd := res.CWD
	if cwd == "" {
		cwd = "gbr-open-" + res.SessionID
	}
	s.rt.scanner.Track(session.Candidate{
		CWD:      cwd,
		Shell:    firstFilled(res.Command, "grok-build"),
		PID:      res.PID,
		Title:    title,
		PreferID: res.SessionID,
	})
}

func (s *botServer) finishOpen(w http.ResponseWriter, res inject.OpenResult, holder, goal string, ttlS int, steal bool, notify *bool) {
	ttl := time.Duration(ttlS) * time.Second
	lease, lerr := core.AcquireLease(res.SessionID, holder, goal, ttl, steal)
	payload := map[string]any{
		"ok":         lerr == nil || errors.Is(lerr, core.ErrLeaseHeld),
		"session_id": res.SessionID,
		"opened":     res.Opened,
		"attached":   res.Attached,
		"method":     res.Method,
		"command":    res.Command,
		"cwd":        res.CWD,
		"resume":     res.Resume,
		"pid":        res.PID,
		"note":       res.Note,
		"device":     map[string]any{"id": "local", "kind": "local", "mailbox_id": s.mailboxID, "os": runtime.GOOS},
	}
	if lerr != nil && !errors.Is(lerr, core.ErrLeaseHeld) {
		payload["ok"] = true // session is usable; lease is advisory
		payload["lease_error"] = lerr.Error()
	} else if lerr != nil {
		payload["ok"] = false
		payload["error"] = "locked"
		payload["hint"] = core.LeaseConflictHint(lease)
		payload["lock"] = lease.Public(false)
		writeJSON(w, http.StatusConflict, payload)
		return
	} else {
		payload["lock"] = lease.Public(true)
	}
	if strings.TrimSpace(goal) != "" {
		t, err := core.UpsertTask(core.Task{
			SessionID: res.SessionID,
			Holder:    holder,
			Goal:      goal,
			Status:    core.TaskOpen,
			Device:    "local",
		})
		if err == nil {
			payload["task"] = t
		}
	}
	doNotify := notify == nil || *notify
	if doNotify {
		kind := "open"
		if res.Attached {
			kind = "attach"
		}
		s.notifyPhone("bot · local · " + kind + " · session " + res.SessionID)
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *botServer) handleResult(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	device := firstFilled(q.Get("device"), "")
	if f, _ := core.LoadFleet(); f != nil {
		if d, ok := f.Get(device); ok && d.Kind == "relay" {
			path := "/result?session_id=" + q.Get("session_id") +
				"&command_id=" + q.Get("command_id") +
				"&wait_ms=" + q.Get("wait_ms") +
				"&idle_ms=" + q.Get("idle_ms") +
				"&excerpt_bytes=" + q.Get("excerpt_bytes")
			s.proxyRemote(w, d, http.MethodGet, path, nil)
			return
		}
	}
	sid := strings.TrimSpace(q.Get("session_id"))
	if sid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "session_id_required"})
		return
	}
	waitMS, _ := strconv.Atoi(q.Get("wait_ms"))
	idleMS, _ := strconv.Atoi(q.Get("idle_ms"))
	exN, _ := strconv.Atoi(q.Get("excerpt_bytes"))
	if exN <= 0 {
		exN = 4000
	}
	res := s.collectResult(sid, q.Get("command_id"), waitMS, idleMS, exN)
	writeJSON(w, http.StatusOK, res)
}

func (s *botServer) collectResult(sessionID, commandID string, waitMS, idleMS, excerptN int) map[string]any {
	capture := func() (string, string, error) {
		if s.rt == nil || s.rt.hybrid == nil {
			return "", "", errors.New("agent_not_ready")
		}
		cap, err := s.rt.hybrid.Capture(sessionID)
		return cap.Text, cap.Method, err
	}
	var idle inject.IdleResult
	if waitMS > 0 {
		idle = inject.WaitIdle(capture, time.Duration(waitMS)*time.Millisecond, time.Duration(idleMS)*time.Millisecond)
	} else {
		text, meth, _ := capture()
		idle = inject.PeekIdle(text, meth)
	}
	if excerptN > 0 && len(idle.Excerpt) > excerptN {
		idle.Excerpt = inject.ExcerptTail(idle.Excerpt, excerptN)
		idle.ExcerptBytes = len(idle.Excerpt)
	}
	// Merge ring-buffer output when capture is empty.
	if strings.TrimSpace(idle.Excerpt) == "" && s.rt != nil {
		items := s.rt.botOutputs(sessionID, commandID, "", 20)
		var b strings.Builder
		for _, it := range items {
			if strings.TrimSpace(it.Chunk) == "" {
				continue
			}
			b.WriteString(it.Chunk)
			if !strings.HasSuffix(it.Chunk, "\n") {
				b.WriteByte('\n')
			}
			if it.Method != "" && idle.Method == "" {
				idle.Method = it.Method
			}
		}
		if b.Len() > 0 {
			idle.Excerpt = inject.ExcerptTail(b.String(), excerptN)
			idle.ExcerptBytes = len(idle.Excerpt)
			idle.Changed = true
			if inject.LooksLikePrompt(idle.Excerpt) {
				idle.Idle = true
				idle.Prompt = true
				idle.State = "idle"
				idle.Reason = inject.IdleReasonPrompt
			}
		}
	}
	out := map[string]any{
		"ok":            true,
		"session_id":    sessionID,
		"command_id":    commandID,
		"state":         idle.State,
		"idle":          idle.Idle,
		"reason":        idle.Reason,
		"excerpt":       idle.Excerpt,
		"excerpt_bytes": idle.ExcerptBytes,
		"method":        idle.Method,
		"waited_ms":     idle.WaitedMS,
		"quiet_ms":      idle.QuietMS,
		"prompt":        idle.Prompt,
		"changed":       idle.Changed,
		"device":        map[string]any{"id": "local", "kind": "local", "mailbox_id": s.mailboxID},
	}
	if l, ok := core.GetLease(sessionID); ok {
		out["lock"] = l.Public(false)
	}
	return out
}

func (s *botServer) handleLock(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sid := r.URL.Query().Get("session_id")
		if sid == "" {
			list := core.ListLeases()
			pub := make([]map[string]any, 0, len(list))
			for _, l := range list {
				pub = append(pub, l.Public(false))
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "locks": pub})
			return
		}
		if l, ok := core.GetLease(sid); ok {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "lock": l.Public(false)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "lock": nil})
	case http.MethodPost:
		s.lockAcquire(w, r)
	case http.MethodDelete:
		s.lockRelease(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
	}
}

func (s *botServer) lockAcquire(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBotBody))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read_failed"})
		return
	}
	var body struct {
		Device    string `json:"device"`
		SessionID string `json:"session_id"`
		Holder    string `json:"holder"`
		Goal      string `json:"goal"`
		TTLSec    int    `json:"ttl_s"`
		Steal     bool   `json:"steal"`
		Notify    *bool  `json:"notify_phone"`
	}
	if err := json.Unmarshal(raw, &body); err != nil && len(strings.TrimSpace(string(raw))) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	if f, _ := core.LoadFleet(); f != nil {
		if d, ok := f.Get(body.Device); ok && d.Kind == "relay" {
			s.proxyRemote(w, d, http.MethodPost, "/lock", body)
			return
		}
	}
	if strings.TrimSpace(body.SessionID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "session_id_required"})
		return
	}
	lease, err := core.AcquireLease(body.SessionID, body.Holder, body.Goal, time.Duration(body.TTLSec)*time.Second, body.Steal)
	if err != nil {
		code := http.StatusConflict
		if !errors.Is(err, core.ErrLeaseHeld) {
			code = http.StatusBadRequest
		}
		writeJSON(w, code, map[string]any{
			"ok": false, "error": "locked", "hint": err.Error(), "lock": lease.Public(false),
		})
		return
	}
	if body.Notify == nil || *body.Notify {
		s.notifyPhone("bot · local · lock · session " + body.SessionID + " · " + lease.Holder)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "lock": lease.Public(true)})
}

func (s *botServer) lockRelease(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("session_id")
	holder := r.URL.Query().Get("holder")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		var body struct {
			SessionID string `json:"session_id"`
			Holder    string `json:"holder"`
			Force     bool   `json:"force"`
		}
		if json.Unmarshal(raw, &body) == nil {
			if sid == "" {
				sid = body.SessionID
			}
			if holder == "" {
				holder = body.Holder
			}
			force = force || body.Force
		}
	}
	if sid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "session_id_required"})
		return
	}
	if err := core.ReleaseLease(sid, holder, force); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "released": sid})
}

func (s *botServer) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sid := r.URL.Query().Get("session_id")
		id := r.URL.Query().Get("id")
		if id != "" {
			t, ok := core.GetTask(id)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task": t})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tasks": core.ListTasks(sid)})
	case http.MethodPost:
		raw, err := io.ReadAll(io.LimitReader(r.Body, maxBotBody))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read_failed"})
			return
		}
		var t core.Task
		if err := json.Unmarshal(raw, &t); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
			return
		}
		got, err := core.UpsertTask(t)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task": got})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
	}
}

func (s *botServer) proxyRemote(w http.ResponseWriter, d core.FleetDevice, method, path string, body any) {
	if s.rt == nil || s.rt.relay == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "no_relay"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	raw, code, err := s.rt.relay.BotJSON(ctx, d.MailboxID, d.MailboxKey, method, path, body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	if code == 0 {
		code = http.StatusOK
	}
	w.WriteHeader(code)
	_, _ = w.Write(raw)
}

func newCommandID() string { return uuid.NewString() }
