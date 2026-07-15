// Command gbr-agent is the Grok Build Remote PC agent.
//
//	gbr-agent run              — start relay poll loop + session scanner
//	gbr-agent version          — print version
//	gbr-agent pair -code CODE  — complete mobile pairing
//	gbr-agent rename -name N   — set device display name
//	gbr-agent sessions         — list known sessions
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/core"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/doctor"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/grok"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/relay"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/service"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/session"
	"github.com/google/uuid"
)

var (
	version = "0.3.0"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	logLevel := "info"
	var cmdArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-log" || a == "--log":
			if i+1 < len(args) {
				logLevel = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "-log="):
			logLevel = strings.TrimPrefix(a, "-log=")
		case strings.HasPrefix(a, "--log="):
			logLevel = strings.TrimPrefix(a, "--log=")
		default:
			cmdArgs = args[i:]
			i = len(args)
		}
	}
	if len(cmdArgs) == 0 {
		printUsage()
		return 2
	}

	setupLogger(logLevel)

	cmd := cmdArgs[0]
	subArgs := cmdArgs[1:]

	switch cmd {
	case "version", "-version", "--version":
		fmt.Printf("gbr-agent %s commit=%s date=%s %s/%s\n", version, commit, date, runtime.GOOS, runtime.GOARCH)
		return 0
	case "run":
		return cmdRun(subArgs)
	case "pair":
		return cmdPair(subArgs)
	case "rename":
		return cmdRename(subArgs)
	case "sessions":
		return cmdSessions(subArgs)
	case "status":
		return cmdStatus(subArgs)
	case "doctor":
		return cmdDoctor(subArgs)
	case "service":
		return cmdService(subArgs)
	case "help", "-h", "--help":
		printUsage()
		return 0
	default:
		slog.Error("unknown command", "cmd", cmd)
		printUsage()
		return 2
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `gbr-agent — Grok Build Remote agent (Windows / Mac / Linux)

Usage:
  gbr-agent [-log=info] version
  gbr-agent [-log=info] doctor
  gbr-agent [-log=info] status
  gbr-agent [-log=info] run [-session ID] [-conv MAILBOX_ID] [-relay URL]
  gbr-agent [-log=info] pair -code PAIRING_CODE [-name DEVICE_NAME] [-conv MAILBOX_ID] [-relay URL]
  gbr-agent [-log=info] rename -name DEVICE_NAME [-session SESSION_ID]
  gbr-agent [-log=info] sessions
  gbr-agent [-log=info] service install|uninstall|status

Environment:
  GBR_API_KEY / XAI_API_KEY     xAI API key (optional if relay-only)
  GBR_RELAY_URL                 durable mailbox relay (default production worker)
  GBR_BASE_URL / XAI_BASE_URL   xAI base (legacy Mode B)

Device identity: %%USERPROFILE%%\.gbr\device.json
Sessions rename: %%USERPROFILE%%\.gbr\sessions.json
Inject dedup:    %%USERPROFILE%%\.gbr\seen.json

Platforms:
  windows  SendInput + managed pwsh; Task Scheduler user logon service
  darwin   AppleScript Terminal/iTerm + managed bash; LaunchAgent
  linux    xdotool (X11) + managed bash; systemd --user

`)
}

func setupLogger(level string) {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv})
	slog.SetDefault(slog.New(h))
}

// agentRuntime holds live state for the run loop.
type agentRuntime struct {
	dev     *core.Device
	relay   *relay.Client
	hybrid  *inject.Hybrid
	scanner *session.Scanner
	store   *session.Store
	seen    *core.SeenStore
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	sessionFlag := fs.String("session", "", "also force-track this session_id at cwd")
	conv := fs.String("conv", "", "mailbox conversation id (else from device.json)")
	relayURL := fs.String("relay", "", "relay base URL (else GBR_RELAY_URL / default)")
	_ = fs.Parse(args)

	// Config: API key optional when using durable relay only.
	cfg, cfgErr := core.LoadConfig()
	if cfgErr != nil {
		slog.Warn("xAI config unavailable (relay-only mode ok)", "err", cfgErr)
		cfg = &core.Config{PollIntervalSec: 2, HTTPTimeoutSec: 30, BaseURL: core.DefaultBaseURL}
	}

	dev, err := core.LoadOrCreateDevice()
	if err != nil {
		slog.Error("device", "err", err)
		return 1
	}

	mailboxID := firstNonEmpty(*conv, dev.MailboxConversationID)
	if mailboxID == "" {
		slog.Error("no mailbox; run: gbr-agent pair -code ... first")
		return 1
	}

	rc := relay.New(firstNonEmpty(*relayURL, os.Getenv("GBR_RELAY_URL")), time.Duration(cfg.HTTPTimeoutSec)*time.Second)
	ctxHealth, cancelH := context.WithTimeout(context.Background(), 10*time.Second)
	if err := rc.Health(ctxHealth); err != nil {
		slog.Warn("relay health check failed (will still try)", "relay", rc.Base(), "err", err)
	} else {
		slog.Info("relay ok", "relay", rc.Base())
	}
	cancelH()

	store, err := session.OpenStore("")
	if err != nil {
		slog.Warn("session store", "err", err)
		store = nil
	}
	reg := session.NewRegistry()
	ui := inject.Default()
	pty := inject.NewManager(nil)
	hybrid := inject.NewHybrid(ui, pty)

	discover := func(ctx context.Context) ([]session.Candidate, error) {
		wins, err := hybrid.Discover()
		if err != nil {
			return nil, err
		}
		var out []session.Candidate
		cwd, _ := os.Getwd()
		for _, w := range wins {
			out = append(out, session.Candidate{
				CWD:   cwd, // best-effort; UI discover often lacks cwd
				Shell: string(w.Kind),
				PID:   int(w.PID),
				HWND:  w.HWND,
				Title: w.Title,
			})
		}
		return out, nil
	}
	sc := session.NewScanner(store, reg, discover)
	// Always track agent working directory as a session.
	cwd, _ := os.Getwd()
	sc.Track(session.Candidate{CWD: cwd, Shell: defaultShellName(), Title: "gbr-agent"})
	if *sessionFlag != "" {
		// Pin explicit session id via rename map if possible
		if store != nil && grok.ValidSessionID(*sessionFlag) {
			_ = store.Rename(cwd, *sessionFlag)
		}
	}

	seenStore, err := core.OpenSeen()
	if err != nil {
		slog.Warn("seen store", "err", err)
		seenStore, _ = core.OpenSeen() // empty fallback
	}
	if seenStore == nil {
		seenStore, _ = core.OpenSeen()
	}

	rt := &agentRuntime{
		dev:     dev,
		relay:   rc,
		hybrid:  hybrid,
		scanner: sc,
		store:   store,
		seen:    seenStore,
	}

	slog.Info("gbr-agent starting",
		"version", version,
		"device_id", dev.DeviceID,
		"device_name", dev.DeviceName,
		"mailbox", mailboxID,
		"relay", rc.Base(),
		"os", runtime.GOOS,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer func() { _ = hybrid.Close() }()

	// Background: session scanner
	go func() {
		if err := sc.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("scanner", "err", err)
		}
	}()

	// Publish registers periodically
	go rt.registerLoop(ctx, mailboxID)

	// Heartbeat
	go rt.heartbeatLoop(ctx, mailboxID)

	// Main poll loop
	interval := time.Duration(cfg.PollIntervalSec) * time.Second
	if interval < 2*time.Second {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Immediate poll
	rt.pollOnce(ctx, mailboxID)

	for {
		select {
		case <-ctx.Done():
			slog.Info("shutdown")
			return 0
		case <-ticker.C:
			rt.pollOnce(ctx, mailboxID)
		}
	}
}

func (rt *agentRuntime) pollOnce(ctx context.Context, mailboxID string) {
	pctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	raws, err := rt.relay.Poll(pctx, mailboxID, rt.dev.DeviceID, "agent")
	if err != nil {
		slog.Warn("poll", "err", err)
		return
	}
	for _, raw := range raws {
		env, err := grok.ParseEnvelope(raw)
		if err != nil {
			slog.Debug("skip bad envelope", "err", err)
			continue
		}
		// Prefer command_id for inject/list; include type so pair/output don't collide.
		fp := env.Type + ":" + env.CommandID
		if env.CommandID == "" {
			fp = env.Type + ":" + env.DeviceID + ":" + env.TS.UTC().Format(time.RFC3339Nano)
		}
		if rt.seen != nil && rt.seen.Has(fp) {
			continue
		}
		if err := rt.handle(ctx, mailboxID, env); err != nil {
			slog.Error("handle", "type", env.Type, "err", err)
			continue
		}
		if rt.seen != nil {
			rt.seen.Add(fp)
		}
		// Best-effort: drop inject/list from relay queue so restarts stay clean
		if env.CommandID != "" && (env.Type == grok.TypeInject || env.Type == grok.TypeList) {
			actx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_ = rt.relay.Ack(actx, mailboxID, []string{env.CommandID})
			cancel()
		}
	}
}

func (rt *agentRuntime) handle(ctx context.Context, mailboxID string, env *grok.Envelope) error {
	slog.Info("envelope", "type", env.Type, "session_id", env.SessionID, "command_id", env.CommandID)

	switch env.Type {
	case grok.TypeInject:
		var p grok.InjectPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			return err
		}
		text := p.Text
		if p.Mode == "nl" && p.NLPrompt != "" && text == "" {
			text = p.NLPrompt
		}
		req := inject.InjectRequest{
			SessionID: env.SessionID,
			CommandID: env.CommandID,
			Text:      text,
			Submit:    p.Submit,
		}
		// Prefer binding to known session if we have HWND
		if sess, ok := rt.scanner.Registry.Get(env.SessionID); ok && sess != nil && sess.HWND != 0 {
			_ = rt.hybrid.Bind(env.SessionID, inject.TerminalWindow{
				HWND:  sess.HWND,
				PID:   uint32(sess.PID),
				Title: sess.Title,
			})
		}
		injErr := rt.hybrid.Inject(env.SessionID, req)
		// Capture output after short settle
		time.Sleep(400 * time.Millisecond)
		cap, _ := rt.hybrid.Capture(env.SessionID)
		chunk := cap.Text
		if chunk == "" {
			if injErr != nil {
				chunk = "inject error: " + injErr.Error()
			} else {
				chunk = "ok (no capture buffer yet — managed shell may still be starting)"
			}
		}
		stream := "stdout"
		if injErr != nil && cap.Text == "" {
			stream = "system"
		}
		return rt.pushOutput(ctx, mailboxID, env.SessionID, env.CommandID, stream, chunk, true)

	case grok.TypeList:
		sessions := rt.listSessionPayloads()
		out, err := grok.NewEnvelope(grok.TypeList, rt.dev.DeviceID, "", env.CommandID, map[string]any{
			"sessions": sessions,
		})
		if err != nil {
			return err
		}
		return rt.pushEnv(ctx, mailboxID, out)

	case grok.TypePair:
		slog.Info("pair envelope seen", "from_device", env.DeviceID)
		return nil

	case grok.TypeHeartbeat, grok.TypeOutput, grok.TypeRegister:
		return nil

	default:
		slog.Warn("unhandled type", "type", env.Type)
		return nil
	}
}

func (rt *agentRuntime) pushOutput(ctx context.Context, mailboxID, sessionID, commandID, stream, chunk string, eof bool) error {
	out, err := grok.NewEnvelope(grok.TypeOutput, rt.dev.DeviceID, sessionID, commandID, grok.OutputPayload{
		Stream: stream,
		Chunk:  chunk,
		EOF:    eof,
	})
	if err != nil {
		return err
	}
	return rt.pushEnv(ctx, mailboxID, out)
}

func (rt *agentRuntime) pushEnv(ctx context.Context, mailboxID string, env *grok.Envelope) error {
	// Relay expects plain JSON object
	var wire map[string]any
	b, err := env.Serialize()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		return err
	}
	pctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	return rt.relay.Push(pctx, mailboxID, wire)
}

func (rt *agentRuntime) listSessionPayloads() []map[string]any {
	var out []map[string]any
	if rt.scanner != nil && rt.scanner.Registry != nil {
		for _, s := range rt.scanner.Registry.List() {
			out = append(out, map[string]any{
				"session_id": s.ID,
				"cwd":        s.CWD,
				"shell":      s.Shell,
				"title":      s.Title,
				"os":         runtime.GOOS,
				"git_remote": s.GitRemote,
			})
		}
	}
	// Include managed PTY sessions not already listed
	seen := map[string]bool{}
	for _, m := range out {
		if id, ok := m["session_id"].(string); ok {
			seen[id] = true
		}
	}
	for _, id := range rt.hybrid.ManagedIDs() {
		if seen[id] {
			continue
		}
		out = append(out, map[string]any{
			"session_id": id,
			"shell":      "managed",
			"title":      "gbr managed shell",
			"os":         runtime.GOOS,
		})
	}
	if len(out) == 0 {
		cwd, _ := os.Getwd()
		out = append(out, map[string]any{
			"session_id": session.Slugify(filepath.Base(cwd)),
			"cwd":        cwd,
			"shell":      defaultShellName(),
			"title":      "gbr-agent",
			"os":         runtime.GOOS,
		})
	}
	return out
}

func (rt *agentRuntime) registerLoop(ctx context.Context, mailboxID string) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	// immediate
	rt.publishRegisters(ctx, mailboxID)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rt.publishRegisters(ctx, mailboxID)
		}
	}
}

func (rt *agentRuntime) publishRegisters(ctx context.Context, mailboxID string) {
	for _, msg := range rt.scanner.Registers(rt.dev.DeviceID) {
		// msg is session.RegisterMessage — convert to grok envelope map
		b, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		var wire map[string]any
		if err := json.Unmarshal(b, &wire); err != nil {
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err = rt.relay.Push(pctx, mailboxID, wire)
		cancel()
		if err != nil {
			slog.Debug("register push", "err", err)
		}
	}
}

func (rt *agentRuntime) heartbeatLoop(ctx context.Context, mailboxID string) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n := 0
			if rt.scanner != nil && rt.scanner.Registry != nil {
				n = len(rt.scanner.Registry.List())
			}
			env, err := grok.NewEnvelope(grok.TypeHeartbeat, rt.dev.DeviceID, "", uuid.NewString(), grok.HeartbeatPayload{
				SessionCount: n,
				Status:       "alive",
			})
			if err != nil {
				continue
			}
			if err := rt.pushEnv(ctx, mailboxID, env); err != nil {
				slog.Warn("heartbeat", "err", err)
			}
		}
	}
}

func cmdPair(args []string) int {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	code := fs.String("code", "", "8-char pairing code from mobile")
	name := fs.String("name", "", "device display name")
	conv := fs.String("conv", "", "optional mailbox id (default: gbr-<code>)")
	relayURL := fs.String("relay", "", "relay base URL")
	_ = fs.Parse(args)

	if strings.TrimSpace(*code) == "" {
		slog.Error("pair requires -code")
		return 2
	}
	codeNorm := strings.ToUpper(strings.TrimSpace(*code))
	// Strip spaces/dashes
	codeNorm = strings.ReplaceAll(codeNorm, " ", "")
	codeNorm = strings.ReplaceAll(codeNorm, "-", "")

	dev, err := core.LoadOrCreateDevice()
	if err != nil {
		slog.Error("device", "err", err)
		return 1
	}
	if *name != "" {
		if err := dev.SetDeviceName(*name); err != nil {
			slog.Error("rename during pair", "err", err)
			return 1
		}
	}

	// Always bind mailbox to this pairing code unless -conv overrides.
	// (Previously we kept the old mailbox, so pair -code ABCD left gbr-testcode1.)
	mailboxID := strings.TrimSpace(*conv)
	if mailboxID == "" {
		mailboxID = "gbr-" + strings.ToLower(codeNorm)
	}
	if err := dev.SetMailboxConversationID(mailboxID); err != nil {
		slog.Error("save mailbox id", "err", err)
		return 1
	}

	rc := relay.New(firstNonEmpty(*relayURL, os.Getenv("GBR_RELAY_URL")), 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := rc.Pair(ctx, mailboxID, codeNorm, dev.DeviceID, dev.DeviceName); err != nil {
		slog.Error("relay pair", "err", err)
		return 1
	}

	// Also push a pair envelope into the mailbox so mobile can observe.
	payload := grok.PairPayload{PairingCode: codeNorm, DeviceName: dev.DeviceName}
	env, err := grok.NewEnvelope(grok.TypePair, dev.DeviceID, "", uuid.NewString(), payload)
	if err != nil {
		slog.Error("envelope", "err", err)
		return 1
	}
	var wire map[string]any
	b, _ := env.Serialize()
	_ = json.Unmarshal(b, &wire)
	if err := rc.Push(ctx, mailboxID, wire); err != nil {
		slog.Error("pair push", "err", err)
		return 1
	}

	fmt.Printf("paired device_id=%s mailbox=%s name=%s\n", dev.DeviceID, mailboxID, dev.DeviceName)
	fmt.Printf("relay=%s\n", rc.Base())
	fmt.Printf("device file: %s\n", dev.Path())
	fmt.Printf("next: gbr-agent run\n")
	return 0
}

func cmdRename(args []string) int {
	fs := flag.NewFlagSet("rename", flag.ExitOnError)
	name := fs.String("name", "", "new device display name")
	sessionID := fs.String("session", "", "optional: rename current cwd to this session_id")
	_ = fs.Parse(args)

	if *sessionID != "" {
		if !grok.ValidSessionID(*sessionID) {
			slog.Error("invalid session_id", "id", *sessionID)
			return 2
		}
		store, err := session.OpenStore("")
		if err != nil {
			slog.Error("store", "err", err)
			return 1
		}
		cwd, _ := os.Getwd()
		if err := store.Rename(cwd, *sessionID); err != nil {
			slog.Error("session rename", "err", err)
			return 1
		}
		fmt.Printf("session cwd=%s id=%s\n", cwd, *sessionID)
		return 0
	}

	if strings.TrimSpace(*name) == "" {
		slog.Error("rename requires -name or -session")
		return 2
	}
	dev, err := core.LoadOrCreateDevice()
	if err != nil {
		slog.Error("device", "err", err)
		return 1
	}
	if err := dev.SetDeviceName(strings.TrimSpace(*name)); err != nil {
		slog.Error("rename", "err", err)
		return 1
	}
	fmt.Printf("renamed device_id=%s name=%s\n", dev.DeviceID, dev.DeviceName)
	return 0
}

func cmdSessions(args []string) int {
	_ = args
	store, err := session.OpenStore("")
	if err != nil {
		slog.Warn("store", "err", err)
	}
	reg := session.NewRegistry()
	sc := session.NewScanner(store, reg, nil)
	cwd, _ := os.Getwd()
	sc.Track(session.Candidate{CWD: cwd, Shell: defaultShellName(), Title: "gbr-agent"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := sc.ScanOnce(ctx); err != nil {
		slog.Warn("scan", "err", err)
	}
	for _, s := range reg.List() {
		fmt.Printf("%-24s  cwd=%s  shell=%s  title=%s\n", s.ID, s.CWD, s.Shell, s.Title)
	}
	return 0
}

func cmdStatus(args []string) int {
	_ = args
	dev, err := core.LoadOrCreateDevice()
	if err != nil {
		slog.Error("device", "err", err)
		return 1
	}
	seen, _ := core.OpenSeen()
	rc := relay.New(os.Getenv("GBR_RELAY_URL"), 15*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	relayOK := "down"
	if err := rc.Health(ctx); err == nil {
		relayOK = "ok"
	} else {
		relayOK = "error: " + err.Error()
	}
	fmt.Printf("gbr-agent %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	fmt.Printf("device_id:   %s\n", dev.DeviceID)
	fmt.Printf("device_name: %s\n", dev.DeviceName)
	fmt.Printf("mailbox:     %s\n", dev.MailboxConversationID)
	fmt.Printf("relay:       %s (%s)\n", rc.Base(), relayOK)
	fmt.Printf("seen_cmds:   %d\n", seen.Len())
	fmt.Printf("device_file: %s\n", dev.Path())
	if dev.MailboxConversationID == "" {
		fmt.Printf("hint: run  gbr-agent pair -code YOURCODE\n")
	} else {
		fmt.Printf("hint: run  gbr-agent -log=info run\n")
	}
	return 0
}

func cmdDoctor(args []string) int {
	_ = args
	results := doctor.Run()
	fmt.Print(doctor.Format(results))
	for _, r := range results {
		if !r.OK {
			return 1
		}
	}
	return 0
}

func cmdService(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gbr-agent service install|uninstall|status")
		return 2
	}
	switch args[0] {
	case "install":
		if err := service.Install(); err != nil {
			slog.Error("service install", "err", err)
			return 1
		}
		fmt.Println("service installed and started (user session)")
		st, _ := service.Status()
		fmt.Print(st)
		return 0
	case "uninstall":
		if err := service.Uninstall(); err != nil {
			slog.Error("service uninstall", "err", err)
			return 1
		}
		fmt.Println("service uninstalled")
		return 0
	case "status":
		st, err := service.Status()
		if err != nil {
			slog.Error("service status", "err", err)
			return 1
		}
		fmt.Print(st)
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: gbr-agent service install|uninstall|status")
		return 2
	}
}

func defaultShellName() string {
	if runtime.GOOS == "windows" {
		return "pwsh"
	}
	return "bash"
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}
