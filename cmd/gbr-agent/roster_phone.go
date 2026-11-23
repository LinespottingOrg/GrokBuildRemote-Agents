package main

import (
	"context"
	"log/slog"
	"strings"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/session"
)

func (rt *agentRuntime) phoneSessionHidden(id string) bool {
	if rt == nil || rt.vis == nil {
		return false
	}
	return rt.vis.Hidden(strings.TrimSpace(id))
}

// phoneSessionPayloads is the roster pushed to the phone.
// Hidden sessions are omitted. The local Bot API still lists them.
func (rt *agentRuntime) phoneSessionPayloads() []map[string]any {
	raw := rt.listSessionPayloads()
	if rt == nil || rt.vis == nil {
		return raw
	}
	out := make([]map[string]any, 0, len(raw))
	for _, row := range raw {
		id, _ := row["session_id"].(string)
		if rt.vis.Hidden(id) {
			continue
		}
		out = append(out, row)
	}
	return out
}

// refuseHiddenPhone reports whether a phone-originated action on id must stop.
// It does not type into the terminal. Local loopback inject is a different path.
func (rt *agentRuntime) refuseHiddenPhone(ctx context.Context, mailboxID, sessionID, commandID, action string) bool {
	if !rt.phoneSessionHidden(sessionID) {
		return false
	}
	slog.Info("phone action refused; session hidden", "action", action, "session_id", sessionID)
	if rt != nil && rt.relay != nil && rt.dev != nil {
		_ = rt.pushOutputFull(ctx, mailboxID, sessionID, commandID, "system",
			"session is hidden from the phone roster", true, "control", action)
	}
	return true
}

func filterVisibleIDs(ids []string, vis *session.Visibility) []string {
	if vis == nil || len(ids) == 0 {
		return ids
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if vis.Hidden(id) {
			continue
		}
		out = append(out, id)
	}
	return out
}
