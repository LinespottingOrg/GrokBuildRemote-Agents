package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const settingsPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Grok Build Remote — Settings</title>
<style>
  body { font: 15px/1.4 -apple-system, Segoe UI, sans-serif; margin: 24px; max-width: 640px; color: #142; background: #f6f7f4; }
  h1 { font-size: 22px; margin: 0 0 8px; }
  p { color: #345; }
  ul { list-style: none; padding: 0; }
  li { background: #fff; border: 1px solid #d5ddd0; border-radius: 8px; padding: 10px 12px; margin: 8px 0; display: flex; gap: 10px; align-items: flex-start; }
  .meta { color: #567; font-size: 13px; }
  .err { color: #a20; }
</style>
</head>
<body>
<h1>Grok Build Remote</h1>
<p>Choose which terminals this computer shows on the phone. Hidden terminals stay on this machine. The phone cannot see them or type into them. Nothing is hidden until you check it.</p>
<p id="status" class="meta">Loading…</p>
<ul id="rows"></ul>
<script>
async function load() {
  const status = document.getElementById('status');
  const rows = document.getElementById('rows');
  try {
    const res = await fetch('/v1/settings/roster');
    const data = await res.json();
    rows.textContent = '';
    const sessions = (data && data.sessions) || [];
    if (!sessions.length) {
      status.textContent = 'No terminals discovered yet.';
      return;
    }
    status.textContent = 'Checked rows are hidden from the phone.';
    for (const s of sessions) {
      const li = document.createElement('li');
      const box = document.createElement('input');
      box.type = 'checkbox';
      box.checked = !!s.hidden;
      box.addEventListener('change', () => save(s.session_id, box.checked, status));
      const text = document.createElement('div');
      const title = document.createElement('div');
      title.textContent = s.title || s.session_id;
      const meta = document.createElement('div');
      meta.className = 'meta';
      meta.textContent = s.session_id + (s.shell ? ' · ' + s.shell : '') + (s.online ? '' : ' · not open');
      text.appendChild(title);
      text.appendChild(meta);
      li.appendChild(box);
      li.appendChild(text);
      rows.appendChild(li);
    }
  } catch (err) {
    status.className = 'err';
    status.textContent = 'Could not load settings.';
  }
}
async function save(id, hidden, status) {
  status.className = 'meta';
  status.textContent = 'Saving…';
  const res = await fetch('/v1/settings/roster', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: id, hidden: hidden })
  });
  if (!res.ok) {
    status.className = 'err';
    status.textContent = 'Save failed.';
    return;
  }
  status.textContent = hidden ? 'Hidden from the phone.' : 'Visible on the phone.';
}
load();
</script>
</body>
</html>
`

func (rt *agentRuntime) republishRoster() {
	if rt == nil || rt.dev == nil || rt.relay == nil {
		return
	}
	mailboxID := strings.TrimSpace(rt.dev.MailboxConversationID)
	if mailboxID == "" {
		return
	}
	go rt.publishSessionSnapshot(context.Background(), mailboxID)
}

func (s *botServer) writeSettingsPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, settingsPageHTML)
}

func (s *botServer) rosterSettingsRows() []map[string]any {
	if s == nil || s.rt == nil {
		return []map[string]any{}
	}
	live := s.rt.listSessionPayloads()
	seen := map[string]bool{}
	out := make([]map[string]any, 0, len(live))
	for _, row := range live {
		id, _ := row["session_id"].(string)
		if id == "" {
			continue
		}
		seen[id] = true
		title, _ := row["title"].(string)
		shell, _ := row["shell"].(string)
		out = append(out, map[string]any{
			"session_id": id,
			"title":      title,
			"shell":      shell,
			"hidden":     s.rt.phoneSessionHidden(id),
			"online":     true,
		})
	}
	if s.rt.vis != nil {
		for _, h := range s.rt.vis.List() {
			if seen[h.SessionID] {
				continue
			}
			out = append(out, map[string]any{
				"session_id": h.SessionID,
				"title":      h.Title,
				"shell":      h.Shell,
				"hidden":     true,
				"online":     false,
			})
		}
	}
	return out
}

func (s *botServer) writeRosterSettings(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"sessions": s.rosterSettingsRows(),
	})
}

func (s *botServer) handleRosterSettings(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.rt == nil || s.rt.vis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "visibility_unavailable"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8192))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_body"})
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		Hidden    bool   `json:"hidden"`
		Title     string `json:"title"`
		Shell     string `json:"shell"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	title := strings.TrimSpace(req.Title)
	shell := strings.TrimSpace(req.Shell)
	if s.rt.scanner != nil && s.rt.scanner.Registry != nil {
		if sess, ok := s.rt.scanner.Registry.Get(req.SessionID); ok && sess != nil {
			if title == "" {
				title = sess.Title
			}
			if shell == "" {
				shell = sess.Shell
			}
		}
	}
	if err := s.rt.vis.Set(req.SessionID, req.Hidden, title, shell); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.rt.republishRoster()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"session_id": req.SessionID,
		"hidden":     s.rt.vis.Hidden(req.SessionID),
	})
}
