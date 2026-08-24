package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/core"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/relay"
	"github.com/google/uuid"
)

func cmdFleet(args []string) int {
	if len(args) == 0 {
		return fleetPrint()
	}
	switch strings.ToLower(args[0]) {
	case "list", "ls", "status":
		return fleetPrint()
	case "add":
		return fleetAdd(args[1:])
	case "rm", "remove", "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: gbr-agent fleet rm DEVICE_ID")
			return 2
		}
		return fleetRemove(args[1])
	case "sync":
		return fleetSync()
	default:
		fmt.Fprintln(os.Stderr, `usage:
  gbr-agent fleet
  gbr-agent fleet add -name mac -os darwin
  gbr-agent fleet add -name studio-linux -mailbox gbr-xxxx -key KEY [-os linux]
  gbr-agent fleet rm studio-linux
  gbr-agent fleet sync`)
		return 2
	}
}

func fleetPrint() int {
	f, err := core.LoadFleet()
	if err != nil {
		slog.Error("fleet", "err", err)
		return 1
	}
	fmt.Println("local    this PC    (gbr-agent run · loopback + this mailbox)")
	for _, d := range f.PublicDevices() {
		fmt.Printf("%-12s  %-10s  mailbox=%s  os=%s  key=%v\n",
			d["id"], d["kind"], d["mailbox_id"], d["os"], d["has_key"])
	}
	return 0
}

func fleetAdd(args []string) int {
	fs := flag.NewFlagSet("fleet add", flag.ExitOnError)
	name := fs.String("name", "", "device slug / display name")
	id := fs.String("id", "", "optional id (default: slug of -name)")
	mailbox := fs.String("mailbox", "", "remote mailbox id (gbr-…)")
	key := fs.String("key", "", "remote mailbox key (X-GBR-Key)")
	osName := fs.String("os", "", "linux / darwin / windows")
	className := fs.String("class", "", "phone | linux | pc | laptop | mac_mini")
	_ = fs.Parse(args)
	f, err := core.LoadFleet()
	if err != nil {
		slog.Error("fleet", "err", err)
		return 1
	}
	dev := core.FleetDevice{
		ID:         firstNonEmpty(*id, *name),
		Name:       firstNonEmpty(*name, *id),
		Kind:       "relay",
		Class:      strings.TrimSpace(*className),
		MailboxID:  strings.TrimSpace(*mailbox),
		MailboxKey: strings.TrimSpace(*key),
		OS:         strings.TrimSpace(*osName),
		Impl:       "gbr",
	}
	if err := applyFleetOffer(&dev); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := f.Upsert(dev); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	saved, _ := f.Get(firstNonEmpty(*id, *name))
	_ = fleetSyncOne(saved)
	fmt.Printf("ok fleet + %s  mailbox=%s\n", saved.ID, saved.MailboxID)
	fmt.Println("Grok bot: POST /v1/inject  {\"device\":\"" + saved.ID + "\",\"text\":\"…\"}")
	return 0
}

// applyFleetOffer fills mailbox id/key/os from a pair-as-mailbox offer when
// the flags omitted them. Never prints the key.
func applyFleetOffer(dev *core.FleetDevice) error {
	if strings.TrimSpace(dev.MailboxID) != "" && strings.TrimSpace(dev.MailboxKey) != "" {
		return nil
	}
	slug := firstNonEmpty(dev.ID, dev.Name)
	if slug == "" {
		return fmt.Errorf("usage: gbr-agent fleet add -name mac -os darwin")
	}
	offer, _, err := core.LoadFleetOffer(slug)
	if err != nil {
		if strings.TrimSpace(dev.MailboxID) == "" || strings.TrimSpace(dev.MailboxKey) == "" {
			return fmt.Errorf("mailbox_id and mailbox_key required (or a pair-as-mailbox offer for %q)", slug)
		}
		return nil
	}
	if strings.TrimSpace(dev.MailboxID) == "" {
		dev.MailboxID = offer.MailboxID
	}
	if strings.TrimSpace(dev.MailboxKey) == "" {
		dev.MailboxKey = offer.MailboxKey
	}
	if strings.TrimSpace(dev.OS) == "" {
		dev.OS = offer.OS
	}
	return nil
}

func fleetRemove(id string) int {
	f, err := core.LoadFleet()
	if err != nil {
		slog.Error("fleet", "err", err)
		return 1
	}
	if err := f.Remove(id); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_ = fleetSync()
	fmt.Printf("ok fleet − %s\n", id)
	return 0
}

func fleetSync() int {
	f, err := core.LoadFleet()
	if err != nil {
		slog.Error("fleet", "err", err)
		return 1
	}
	dev, err := core.LoadOrCreateDevice()
	if err != nil || dev.MailboxConversationID == "" || dev.MailboxKey == "" {
		fmt.Fprintln(os.Stderr, "pair this PC first (gbr-agent pair)")
		return 1
	}
	rc := relay.New(os.Getenv("GBR_RELAY_URL"), 20*time.Second)
	rc.SetKey(dev.MailboxKey)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Replace remote fleet with local list (POST each after we cannot bulk easily).
	// Best-effort: POST each remote; hub already has local.
	for _, d := range f.PublicDevices() {
		rawID, _ := d["id"].(string)
		got, ok := f.Get(rawID)
		if !ok || got.Kind == "local" {
			continue
		}
		body := fleetSyncBody(got)
		_, code, err := rc.BotJSON(ctx, dev.MailboxConversationID, dev.MailboxKey, http.MethodPost, "/devices", body)
		if err != nil || code >= 300 {
			slog.Warn("fleet sync", "device", got.ID, "code", code, "err", err)
		}
	}
	fmt.Println("ok fleet synced to relay hub mailbox")
	return 0
}

func fleetSyncOne(d core.FleetDevice) error {
	dev, err := core.LoadOrCreateDevice()
	if err != nil || dev.MailboxConversationID == "" || dev.MailboxKey == "" {
		return err
	}
	rc := relay.New(os.Getenv("GBR_RELAY_URL"), 20*time.Second)
	rc.SetKey(dev.MailboxKey)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	body := fleetSyncBody(d)
	_, code, err := rc.BotJSON(ctx, dev.MailboxConversationID, dev.MailboxKey, http.MethodPost, "/devices", body)
	if err != nil {
		return err
	}
	if code >= 300 {
		return fmt.Errorf("relay fleet sync HTTP %d", code)
	}
	return nil
}

// fleetSyncBody is the POST /devices payload the hub stores for a remote.
// class / hostname / impl must travel with id, name, mailbox, and os so
// Grok Bot can route by unique class after a fleet add/sync.
func fleetSyncBody(d core.FleetDevice) map[string]any {
	impl := d.Impl
	if impl == "" {
		impl = "gbr"
	}
	return map[string]any{
		"id": d.ID, "name": d.Name, "mailbox_id": d.MailboxID,
		"mailbox_key": d.MailboxKey, "os": d.OS,
		"class": d.Class, "hostname": d.Hostname, "impl": impl,
	}
}

// listPublicDevices is local first, then fleet remotes. Keys never appear
// (has_key only). Local is always online; remotes are false when unknown.
func (s *botServer) listPublicDevices() []map[string]any {
	local := core.LocalIdentity(s.mailboxID, s.key != "")
	phone := core.PhoneIdentity(s.mailboxID)
	devs := []map[string]any{local, phone}
	f, _ := core.LoadFleet()
	if f != nil {
		for _, d := range f.PublicDevices() {
			if d == nil {
				continue
			}
			d["online"] = false
			devs = append(devs, d)
		}
	}
	return devs
}

func matchListedDevice(devices []map[string]any, want string) (map[string]any, bool) {
	want = strings.TrimSpace(want)
	if want == "" {
		return nil, false
	}
	norm := strings.ToLower(strings.ReplaceAll(want, "_", "-"))
	var classHits []map[string]any
	for _, d := range devices {
		if d == nil {
			continue
		}
		id, _ := d["id"].(string)
		name, _ := d["name"].(string)
		cls, _ := d["class"].(string)
		if strings.EqualFold(id, want) || strings.EqualFold(name, want) || strings.EqualFold(id, norm) {
			return d, true
		}
		if parsed, ok := core.ParseClass(want); ok && cls == parsed {
			classHits = append(classHits, d)
		}
	}
	if len(classHits) == 1 {
		return classHits[0], true
	}
	return nil, false
}

func (s *botServer) writeDevices(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "mailbox_id": s.mailboxID, "devices": s.listPublicDevices(),
		"note": "One Grok Bot instance. device=local is this machine; class=phone is the app (spectator). Remotes via the GitHub HTTPS relay. Route by id, name, or unique class.",
	})
}

func printStatusFleet(mailboxID string, hasKey bool) {
	fmt.Println("fleet:")
	hk := "no"
	if hasKey {
		hk = "yes"
	}
	local := core.LocalIdentity(mailboxID, hasKey)
	cls, _ := local["class"].(string)
	hn, _ := local["hostname"].(string)
	fmt.Printf("  %-12s  %-10s  kind=local  class=%s  os=%s  host=%s  mailbox=%s  has_key=%s  online=true\n",
		"local", local["name"], cls, runtime.GOOS, hn, mailboxID, hk)
	fmt.Printf("  %-12s  %-10s  kind=app    class=phone  role=spectator\n", "phone", "paired app")
	f, err := core.LoadFleet()
	if err != nil || f == nil {
		return
	}
	for _, d := range f.PublicDevices() {
		remoteHK := "no"
		if v, _ := d["has_key"].(bool); v {
			remoteHK = "yes"
		}
		id, _ := d["id"].(string)
		name, _ := d["name"].(string)
		kind, _ := d["kind"].(string)
		cls, _ := d["class"].(string)
		osName, _ := d["os"].(string)
		mb, _ := d["mailbox_id"].(string)
		fmt.Printf("  %-12s  %-10s  kind=%s  class=%s  os=%s  mailbox=%s  has_key=%s\n",
			id, name, kind, cls, osName, mb, remoteHK)
	}
}

func (s *botServer) handleDeviceWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBotBody))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read_failed"})
		return
	}
	var body core.FleetDevice
	if err := json.Unmarshal(raw, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	f, err := core.LoadFleet()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := f.Upsert(body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	_ = fleetSyncOne(body)
	s.notifyPhone("bot · fleet + " + body.ID)
	saved, _ := f.Get(body.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "device": map[string]any{
		"id": saved.ID, "name": saved.Name, "kind": "relay", "class": saved.Class,
		"mailbox_id": saved.MailboxID, "os": saved.OS, "has_key": true,
	}})
}

func (s *botServer) handleDeviceDelete(w http.ResponseWriter, id string) {
	f, err := core.LoadFleet()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := f.Remove(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.notifyPhone("bot · fleet − " + id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": id})
}

func (s *botServer) notifyPhone(line string) {
	if s.rt == nil || s.rt.dev == nil || s.rt.relay == nil || s.mailboxID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.rt.pushOutputFull(ctx, s.mailboxID, "bot", uuid.NewString(), "system", line, true, "bot", "status")
}

func (s *botServer) injectRemote(w http.ResponseWriter, d core.FleetDevice, sessionID, text string, submit bool, commandID string, notify bool) {
	if s.rt == nil || s.rt.relay == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "no_relay"})
		return
	}
	body := map[string]any{
		"session_id": sessionID, "text": text, "submit": submit,
		"command_id": commandID, "notify_phone": false,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	raw, code, err := s.rt.relay.BotJSON(ctx, d.MailboxID, d.MailboxKey, http.MethodPost, "/inject", body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	if notify {
		s.notifyPhone(fmt.Sprintf("bot · %s · inject · session %s", d.ID, sessionID))
	}
	if code >= 400 {
		w.WriteHeader(code)
		_, _ = w.Write(raw)
		return
	}
	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)
	if parsed == nil {
		parsed = map[string]any{}
	}
	parsed["device"] = map[string]any{"id": d.ID, "kind": "relay", "mailbox_id": d.MailboxID, "os": d.OS}
	parsed["phone_status"] = notify
	writeJSON(w, http.StatusOK, parsed)
}

func (s *botServer) injectLocal(w http.ResponseWriter, sessionID, text string, submit bool, commandID string, notify bool, waitIdle bool, waitMS, idleMS int) {
	if s.rt == nil || s.rt.hybrid == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "agent_not_ready"})
		return
	}
	if sessionID == "session" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false, "error": "inject: empty session_id refused",
			"hint": "session_id \"session\" is the agent pseudo-session. Call GET /v1/sessions or POST /v1/sessions/open.",
		})
		return
	}
	if l, ok := core.GetLease(sessionID); ok {
		// Advisory: still inject, but tell the caller who holds the window.
		_ = l
	}
	if s.rt.scanner != nil && s.rt.scanner.Registry != nil {
		if sess, ok := s.rt.scanner.Registry.Get(sessionID); ok && sess != nil && sess.HWND != 0 {
			_ = s.rt.hybrid.Bind(sessionID, inject.TerminalWindow{
				HWND: sess.HWND, PID: uint32(sess.PID), Title: sess.Title,
			})
		}
	}
	req := inject.InjectRequest{SessionID: sessionID, CommandID: commandID, Text: text, Submit: submit}
	injErr := s.rt.hybrid.Inject(sessionID, req)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = s.rt.captureAndPushAfterInject(ctx, s.mailboxID, sessionID, commandID, injErr)
	}()
	if notify {
		s.notifyPhone(fmt.Sprintf("bot · local · inject · session %s", sessionID))
	}
	out := map[string]any{
		"ok": injErr == nil, "command_id": commandID, "session_id": sessionID,
		"device": map[string]any{"id": "local", "kind": "local", "mailbox_id": s.mailboxID, "os": runtime.GOOS},
		"queued": false, "local": true, "phone_status": notify, "error": errString(injErr),
		"retry": false, // never re-issue this command_id; timed-out / failed injects die
	}
	if l, ok := core.GetLease(sessionID); ok {
		out["lock"] = l.Public(false)
	}
	if waitIdle && injErr == nil {
		if waitMS <= 0 {
			waitMS = 60000
		}
		out["result"] = s.collectResult(sessionID, commandID, waitMS, idleMS, 4000)
	}
	writeJSON(w, http.StatusOK, out)
}
