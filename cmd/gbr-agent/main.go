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
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/trace"
	"github.com/google/uuid"
)

var (
	version = "0.4.0"
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
	case "logs":
		return cmdLogs(subArgs)
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
  gbr-agent [-log=info] run [-session ID] [-conv MAILBOX_ID] [-relay URL] [-force]
  gbr-agent [-log=info] pair -code PAIRING_CODE [-name DEVICE_NAME] [-conv MAILBOX_ID] [-relay URL]
  gbr-agent [-log=info] rename -name DEVICE_NAME [-session SESSION_ID]
  gbr-agent [-log=info] sessions
  gbr-agent [-log=info] logs [-f] [-n 50] [-command COMMAND_ID]
  gbr-agent [-log=info] service install|uninstall|status

Environment:
  GBR_API_KEY / XAI_API_KEY     xAI API key (optional if relay-only)
  GBR_RELAY_URL                 durable mailbox relay (default production worker)
  GBR_BASE_URL / XAI_BASE_URL   xAI base (legacy Mode B)
  GBR_TRACE=0                   disable hop tracing entirely
  GBR_TRACE_REMOTE=0            trace to local file only (no relay mirror)
  GBR_LOG_DIR                   override log directory

Device identity: %%USERPROFILE%%\.gbr\device.json
Sessions rename: %%USERPROFILE%%\.gbr\sessions.json
Inject dedup:    %%USERPROFILE%%\.gbr\seen.json
Trace log:       %%USERPROFILE%%\.gbr\logs\agent-YYYY-MM-DD.jsonl

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
	force := fs.Bool("force", false, "start even if another agent holds the lock (unsafe)")
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

	// Single instance per machine. One agent already serves MANY sessions —
	// the scanner tracks every terminal and injects are routed by session_id.
	// Two agents on one mailbox both poll, both inject and both ack, so they
	// consume each other's commands; three were found running concurrently
	// during service install, which produced injects that appeared to vanish.
	lock, lockErr := core.AcquireLock(mailboxID)
	if lockErr != nil {
		if !*force {
			slog.Error("refusing to start", "err", lockErr)
			fmt.Fprintf(os.Stderr, `
One gbr-agent per mailbox — it already handles all your sessions.

  gbr-agent sessions          list the sessions this machine exposes
  gbr-agent logs -f           watch what the running agent is doing
  gbr-agent run -force        start anyway (only if you know the lock is wrong)

Mailbox:   %s
Lock file: %s
`, mailboxID, core.LockPath(mailboxID))
			return 1
		}
		slog.Warn("lock held but -force given; running a second agent is unsafe", "err", lockErr)
	}
	defer lock.Release()

	rc := relay.New(firstNonEmpty(*relayURL, os.Getenv("GBR_RELAY_URL")), time.Duration(cfg.HTTPTimeoutSec)*time.Second)
	rc.SetKey(dev.MailboxKey) // no-op when unpaired or paired against a legacy relay
	if dev.MailboxKey == "" {
		slog.Warn("no mailbox key on file — running unauthenticated; re-pair to obtain one",
			"hint", "gbr-agent pair -code YOURCODE")
	}
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

	tl := trace.Init(trace.Config{
		Actor:     "agent",
		DeviceID:  dev.DeviceID,
		MailboxID: mailboxID,
		RelayBase: rc.Base(),
	})
	defer trace.Close()

	slog.Info("gbr-agent starting",
		"version", version,
		"device_id", dev.DeviceID,
		"device_name", dev.DeviceName,
		"mailbox", mailboxID,
		"relay", rc.Base(),
		"os", runtime.GOOS,
		"trace", tl.Enabled(),
		"trace_remote", tl.RemoteEnabled(),
		"trace_log", tl.Path(),
	)
	trace.Emit(trace.Event{
		Hop:    trace.HopAgentStart,
		Type:   "lifecycle",
		OK:     true,
		Detail: fmt.Sprintf("v%s %s/%s mailbox=%s", version, runtime.GOOS, runtime.GOARCH, mailboxID),
	})

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
			trace.Emit(trace.Event{Hop: trace.HopAgentStop, Type: "lifecycle", OK: true})
			return 0
		case <-ticker.C:
			rt.pollOnce(ctx, mailboxID)
		}
	}
}

func (rt *agentRuntime) pollOnce(ctx context.Context, mailboxID string) {
	pctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	pollStart := time.Now()
	raws, err := rt.relay.Poll(pctx, mailboxID, rt.dev.DeviceID, "agent")
	if err != nil {
		slog.Warn("poll", "err", err)
		trace.Emit(trace.Event{
			Hop:    trace.HopAgentPoll,
			OK:     false,
			MS:     time.Since(pollStart).Milliseconds(),
			Detail: err.Error(),
		})
		return
	}
	// Only trace polls that actually delivered work — idle polls stay quiet.
	if len(raws) > 0 {
		trace.Emit(trace.Event{
			Hop:    trace.HopAgentPoll,
			OK:     true,
			MS:     time.Since(pollStart).Milliseconds(),
			Detail: fmt.Sprintf("received=%d", len(raws)),
		})
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
			_ = rt.relay.Ack(actx, mailboxID, []string{env.CommandID}, rt.dev.DeviceID)
			cancel()
		}
	}
}

func (rt *agentRuntime) handle(ctx context.Context, mailboxID string, env *grok.Envelope) error {
	slog.Info("envelope", "type", env.Type, "session_id", env.SessionID, "command_id", env.CommandID)

	// Latency from the phone stamping the envelope to the agent receiving it.
	var relayLagMS int64
	if !env.TS.IsZero() {
		relayLagMS = time.Since(env.TS).Milliseconds()
	}
	trace.Emit(trace.Event{
		Hop:       trace.HopAgentRecv,
		Type:      string(env.Type),
		SessionID: env.SessionID,
		CommandID: env.CommandID,
		OK:        true,
		MS:        relayLagMS,
		Detail:    fmt.Sprintf("from_device=%s", env.DeviceID),
	})

	switch env.Type {
	case grok.TypeInject:
		var p grok.InjectPayload
		if err := env.UnmarshalPayload(&p); err != nil {
			trace.Emit(trace.Event{
				Hop:       trace.HopAgentError,
				Type:      string(env.Type),
				SessionID: env.SessionID,
				CommandID: env.CommandID,
				OK:        false,
				Detail:    "bad inject payload: " + err.Error(),
			})
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
		injStart := time.Now()
		injErr := rt.hybrid.Inject(env.SessionID, req)
		injDetail := fmt.Sprintf("chars=%d submit=%v mode=%s", len(text), p.Submit, p.Mode)
		if injErr != nil {
			injDetail = injErr.Error()
		}
		trace.Emit(trace.Event{
			Hop:       trace.HopAgentInject,
			Type:      string(env.Type),
			SessionID: env.SessionID,
			CommandID: env.CommandID,
			OK:        injErr == nil,
			MS:        time.Since(injStart).Milliseconds(),
			Detail:    injDetail,
		})

		// Capture output after short settle
		time.Sleep(400 * time.Millisecond)
		capStart := time.Now()
		cap, _ := rt.hybrid.Capture(env.SessionID)
		trace.Emit(trace.Event{
			Hop:       trace.HopAgentCapture,
			Type:      string(env.Type),
			SessionID: env.SessionID,
			CommandID: env.CommandID,
			OK:        cap.Text != "",
			MS:        time.Since(capStart).Milliseconds(),
			Detail:    fmt.Sprintf("bytes=%d", len(cap.Text)),
		})
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
	pushStart := time.Now()
	pushErr := rt.pushEnv(ctx, mailboxID, out)
	detail := fmt.Sprintf("stream=%s bytes=%d eof=%v", stream, len(chunk), eof)
	if pushErr != nil {
		detail = pushErr.Error()
	}
	trace.Emit(trace.Event{
		Hop:       trace.HopAgentPushOutput,
		Type:      "output",
		SessionID: sessionID,
		CommandID: commandID,
		OK:        pushErr == nil,
		MS:        time.Since(pushStart).Milliseconds(),
		Detail:    detail,
	})
	return pushErr
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
			hbErr := rt.pushEnv(ctx, mailboxID, env)
			if hbErr != nil {
				slog.Warn("heartbeat", "err", hbErr)
			}
			trace.Emit(trace.Event{
				Hop:    trace.HopAgentHeartbeat,
				Type:   "heartbeat",
				OK:     hbErr == nil,
				Detail: fmt.Sprintf("sessions=%d", n),
			})
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

	mbKey, err := rc.Pair(ctx, mailboxID, codeNorm, dev.DeviceID, dev.DeviceName)
	if err != nil {
		slog.Error("relay pair", "err", err)
		return 1
	}
	if mbKey != "" {
		if err := dev.SetMailboxKey(mbKey); err != nil {
			slog.Error("save mailbox key", "err", err)
			return 1
		}
	} else {
		slog.Warn("relay issued no mailbox key (legacy relay) — requests will be unauthenticated")
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
	if dev.MailboxKey != "" {
		fmt.Printf("mailbox_key: set (%d chars) — requests authenticated\n", len(dev.MailboxKey))
	} else {
		fmt.Printf("mailbox_key: NOT SET — unauthenticated; re-pair to obtain one\n")
	}
	fmt.Printf("relay:       %s (%s)\n", rc.Base(), relayOK)
	fmt.Printf("seen_cmds:   %d\n", seen.Len())
	fmt.Printf("device_file: %s\n", dev.Path())
	tl := trace.New(trace.Config{
		Actor:     "agent",
		DeviceID:  dev.DeviceID,
		MailboxID: dev.MailboxConversationID,
		RelayBase: rc.Base(),
	})
	fmt.Printf("trace:       enabled=%v remote=%v\n", tl.Enabled(), tl.RemoteEnabled())
	fmt.Printf("trace_log:   %s\n", tl.Path())
	tl.Close()
	if li, ok := core.ReadLock(dev.MailboxConversationID); ok {
		alive := core.ProcessAlive(li.PID)
		state := "STALE (will be reclaimed)"
		if alive {
			state = "running"
		}
		fmt.Printf("agent_lock:  pid=%d %s since %s\n",
			li.PID, state, li.StartedAt.Local().Format("15:04:05"))
	} else {
		fmt.Printf("agent_lock:  none — no agent running\n")
	}
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
