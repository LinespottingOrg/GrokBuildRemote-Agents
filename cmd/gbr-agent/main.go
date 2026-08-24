// Command gbr-agent is the Grok Build Remote PC agent.
//
//	gbr-agent run              — start relay poll loop + session scanner
//	gbr-agent version          — print version
//	gbr-agent pair             — generate code, open browser QR for phone camera
//	gbr-agent pair -code CODE  — pair with code (legacy / manual)
//	gbr-agent rename -name N   — set device display name
//	gbr-agent sessions         — list known sessions
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
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
	qrcode "github.com/skip2/go-qrcode"
)

var (
	version = "0.6.2"
	commit  = "none"
	date    = "unknown"
)

// Crockford base32 (no I L O U) — same alphabet as the mobile apps.
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

const pairPageBase = "https://grokbuildremote.com/pair.html"

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
	case "pair-as-mailbox", "pair_as_mailbox", "pairasmailbox":
		return cmdPairAsMailbox(subArgs)
	case "rename":
		return cmdRename(subArgs)
	case "sessions":
		return cmdSessions(subArgs)
	case "status":
		return cmdStatus(subArgs)
	case "bot":
		return cmdBot(subArgs)
	case "fleet":
		return cmdFleet(subArgs)
	case "logs":
		return cmdLogs(subArgs)
	case "support-log", "supportlog", "support":
		return cmdSupportLog(subArgs)
	case "feedback":
		return cmdFeedback(subArgs)
	case "doctor":
		return cmdDoctor(subArgs)
	case "netcheck", "net-check", "network":
		return cmdNetcheck(subArgs)
	case "service":
		return cmdService(subArgs)
	case "inbox":
		return cmdInbox(subArgs)
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
  gbr-agent [-log=info] netcheck [-relay URL] [-doc]
      Firewall/VPN test: DNS + TCP/443 + TLS + HTTPS /health (no inbound ports).
  gbr-agent [-log=info] status
      Also lists local + remotes (gbr-agent fleet).
  gbr-agent [-log=info] run [-session ID] [-conv MAILBOX_ID] [-relay URL] [-force] [-bot-port 8788]
  gbr-agent [-log=info] bot
      Print localhost + relay Bot API curl examples (Grok bots).
  gbr-agent [-log=info] fleet
  gbr-agent [-log=info] fleet add -name mac -os darwin [-class mac_mini]
      Hub: register a remote that already ran pair-as-mailbox (reads a local offer).
  gbr-agent [-log=info] fleet add -name studio-linux -mailbox gbr-ID -key KEY [-os linux] [-class linux]
      Classes: phone | linux | pc | laptop | mac_mini — Grok Bot routes by id, name, or unique class.
      Same, with mailbox id + key passed on the command line (do not log the key).
  gbr-agent [-log=info] pair-as-mailbox [-name mac] [-relay URL] [-force]
      Headless mailbox for a fleet remote. No phone, no QR, no code printed.
  gbr-agent [-log=info] pair [-code CODE] [-name DEVICE_NAME] [-conv MAILBOX_ID] [-relay URL] [-no-open]
      Default: PC generates the code, pairs this agent, opens a browser QR for the
      phone camera to scan (mobile does NOT show the QR — the phone reads it).
  gbr-agent [-log=info] rename -name DEVICE_NAME
  gbr-agent [-log=info] rename -session SESSION_ID -name "Phone title"
  gbr-agent [-log=info] sessions
  gbr-agent [-log=info] logs [-f] [-n 50] [-command COMMAND_ID]
  gbr-agent [-log=info] support-log [-open]
      Write diagnostics (sessions, discover, Grok Build windows, recent logs)
      to ~/Downloads/gbr-agent-support-*.txt for support.
  gbr-agent [-log=info] feedback [status|on|off|expand|compact|interval SEC]
      Minimal text feedback to phone (on/off · 5s|10s|1m|10m|1h · expand).
      Default OFF. Does not change inject behaviour.
  gbr-agent [-log=info] inbox
      One-shot dry tick of GitHub boss-steer watch (needs gh on PATH).
      Live inject runs inside gbr-agent run.
  gbr-agent [-log=info] service install|uninstall|status
      Auto-start in background at login (Windows Task Scheduler / macOS LaunchAgent /
      Linux systemd --user). Pair first, then install so the agent keeps polling.

Environment:
  GBR_API_KEY / XAI_API_KEY     xAI API key (optional if relay-only)
  GBR_RELAY_URL                 durable mailbox relay (default production worker;
                                set the same URL on the phone for self-hosted relay)
  GBR_BASE_URL / XAI_BASE_URL   xAI base (legacy Mode B)
  GBR_TRACE=0                   disable hop tracing entirely
  GBR_TRACE_REMOTE=0            trace to local file only (no relay mirror)
  GBR_LOG_DIR                   override log directory
  GBR_BOT_PORT                  localhost bot HTTP port (default 8788, 0=off)
  GBR_BOT_REQUIRE_KEY=1         require mailbox key even on 127.0.0.1
  GBR_INBOX_WATCH=0             disable GitHub boss-steer → inject
  GBR_INBOX_REPO                default LinespottingOrg/grok-build-inbox
  GBR_INBOX_LABEL               default boss-steer
  GBR_INBOX_POLL                default 20s

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

	// lastFeedbackHash avoids spamming identical capture text on the periodic loop.
	fbMu       sync.Mutex
	fbLastHash map[string]string

	snapMu   sync.Mutex
	lastSnap string

	outMu  sync.Mutex
	outLog []botOutputItem
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	sessionFlag := fs.String("session", "", "also force-track this session_id at cwd")
	conv := fs.String("conv", "", "mailbox conversation id (else from device.json)")
	relayURL := fs.String("relay", "", "relay base URL (else GBR_RELAY_URL / default)")
	force := fs.Bool("force", false, "start even if another agent holds the lock (unsafe)")
	botPort := fs.Int("bot-port", botPortFromEnv(), "localhost bot HTTP port (0=off, default 8788)")
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
			slog.Warn("discover failed", "err", err)
			return nil, err
		}
		sortWindowsGrokFirst(wins)
		out, grokN, otherN := windowsToCandidates(wins)
		slog.Info("discover", "windows", len(out), "grok_build", grokN, "other_terminals", otherN)
		trace.Emit(trace.Event{
			Hop:    "agent.discover",
			Type:   "discover",
			OK:     true,
			Detail: fmt.Sprintf("windows=%d grok_build=%d other=%d", len(out), grokN, otherN),
		})
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
		"class", core.DetectClass(),
		"hostname", core.HostnameBest(),
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

	sc.OnScan = func(res session.ScanResult) {
		if rt.noteSessionSnapshot(res.All) {
			slog.Info("sessions changed",
				"n", len(res.All), "added", len(res.Added), "removed", len(res.Removed))
			go rt.publishSessionSnapshot(context.Background(), mailboxID)
		}
	}

	// Background: session scanner
	go func() {
		if err := sc.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("scanner", "err", err)
		}
	}()

	// Publish a full session snapshot periodically (and on add/remove/rename).
	go rt.registerLoop(ctx, mailboxID)

	// Heartbeat
	go rt.heartbeatLoop(ctx, mailboxID)

	// Optional minimal text feedback (default OFF — does not alter inject path)
	go rt.feedbackLoop(ctx, mailboxID)

	// Localhost Bot API for Grok Build / coding agents on this PC.
	go rt.startBotAPI(ctx, mailboxID, *botPort)

	// GitHub inbox → inject (kills email paste). Needs `gh` on PATH.
	go rt.inboxLoop(ctx)

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

		// Multi-sample capture: Grok Build UI often has empty console buffers.
		// We never change inject itself — only try harder to read text afterward.
		return rt.captureAndPushAfterInject(ctx, mailboxID, env.SessionID, env.CommandID, injErr)

	case grok.TypeControl:
		return rt.handleControl(ctx, mailboxID, env)

	case grok.TypeList:
		// Snapshots we pushed (device_id = this agent) come back on poll.
		// Only answer list requests from the phone.
		if env.DeviceID == rt.dev.DeviceID {
			return nil
		}
		sessions := rt.listSessionPayloads()
		out, err := grok.NewEnvelope(grok.TypeList, rt.dev.DeviceID, "", env.CommandID, map[string]any{
			"sessions": sessions,
			"replace":  true,
			"reason":   "snapshot",
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
	return rt.pushOutputFull(ctx, mailboxID, sessionID, commandID, stream, chunk, eof, "inject", "")
}

func (rt *agentRuntime) pushOutputFull(ctx context.Context, mailboxID, sessionID, commandID, stream, chunk string, eof bool, reason, method string) error {
	rt.recordBotOutput(botOutputItem{
		TS:        time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		CommandID: commandID,
		Stream:    stream,
		Chunk:     chunk,
		EOF:       eof,
		Reason:    reason,
		Method:    method,
	})
	if rt.dev == nil {
		return fmt.Errorf("no device")
	}
	out, err := grok.NewEnvelope(grok.TypeOutput, rt.dev.DeviceID, sessionID, commandID, grok.OutputPayload{
		Stream: stream,
		Chunk:  chunk,
		EOF:    eof,
		Reason: reason,
		Method: method,
	})
	if err != nil {
		return err
	}
	pushStart := time.Now()
	pushErr := rt.pushEnv(ctx, mailboxID, out)
	detail := fmt.Sprintf("stream=%s bytes=%d eof=%v reason=%s", stream, len(chunk), eof, reason)
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

// captureAndPushAfterInject samples capture several times without changing inject.
// Empty Grok Build UI buffers yield an honest system note (not fake success spam).
func (rt *agentRuntime) captureAndPushAfterInject(ctx context.Context, mailboxID, sessionID, commandID string, injErr error) error {
	delays := []time.Duration{400 * time.Millisecond, 1500 * time.Millisecond, 3000 * time.Millisecond}
	var lastText, lastMethod string
	var lastPartial bool
	for i, d := range delays {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
		capStart := time.Now()
		cap, _ := rt.hybrid.Capture(sessionID)
		trace.Emit(trace.Event{
			Hop:       trace.HopAgentCapture,
			Type:      "inject",
			SessionID: sessionID,
			CommandID: commandID,
			OK:        strings.TrimSpace(cap.Text) != "",
			MS:        time.Since(capStart).Milliseconds(),
			Detail:    fmt.Sprintf("sample=%d bytes=%d method=%s", i+1, len(cap.Text), cap.Method),
		})
		if strings.TrimSpace(cap.Text) != "" {
			lastText = cap.Text
			lastMethod = cap.Method
			lastPartial = cap.Partial
			// Push intermediate sample (non-eof) if text changed and not last sample.
			if i < len(delays)-1 {
				_ = rt.pushOutputFull(ctx, mailboxID, sessionID, commandID, "stdout",
					trimChunk(lastText, 8*1024), false, "inject", lastMethod)
			}
		} else if lastMethod == "" {
			lastMethod = cap.Method
			lastPartial = cap.Partial
		}
	}
	if strings.TrimSpace(lastText) != "" {
		return rt.pushOutputFull(ctx, mailboxID, sessionID, commandID, "stdout",
			trimChunk(lastText, 12*1024), true, "inject", lastMethod)
	}
	// Honest status when capture is unavailable (typical for Grok Build UI / WT).
	title := rt.sessionTitle(sessionID)
	chunk := ""
	if injErr != nil {
		chunk = "inject error: " + injErr.Error()
	} else {
		chunk = "inject delivered"
		if title != "" {
			chunk += " · window: " + title
		}
		chunk += " · capture empty"
		if lastMethod != "" {
			chunk += " (" + lastMethod + ")"
		}
		if lastPartial {
			chunk += " partial"
		}
		chunk += " — enable Settings → Feedback for periodic text peeks; full Grok UI scrollback is not readable on all platforms"
	}
	return rt.pushOutputFull(ctx, mailboxID, sessionID, commandID, "system", chunk, true, "inject", lastMethod)
}

func (rt *agentRuntime) sessionTitle(sessionID string) string {
	if rt.scanner == nil || rt.scanner.Registry == nil {
		return ""
	}
	if sess, ok := rt.scanner.Registry.Get(sessionID); ok && sess != nil {
		return sess.Title
	}
	return ""
}

func trimChunk(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	// keep tail (most recent terminal output)
	return s[len(s)-max:]
}

func (rt *agentRuntime) handleControl(ctx context.Context, mailboxID string, env *grok.Envelope) error {
	var p grok.ControlPayload
	if err := env.UnmarshalPayload(&p); err != nil {
		return err
	}
	action := strings.ToLower(strings.TrimSpace(p.Action))
	switch action {
	case "feedback", "set_feedback", "":
		cfg := core.LoadFeedback()
		if p.Enabled != nil {
			cfg.Enabled = *p.Enabled
		}
		if p.IntervalSec > 0 {
			cfg.IntervalSec = p.IntervalSec
		}
		if p.Expand != nil {
			cfg.Expand = *p.Expand
		}
		if p.SessionID != "" {
			cfg.SessionID = p.SessionID
		} else if env.SessionID != "" {
			cfg.SessionID = env.SessionID
		}
		if err := core.SaveFeedback(cfg); err != nil {
			return rt.pushOutputFull(ctx, mailboxID, env.SessionID, env.CommandID, "system",
				"feedback save failed: "+err.Error(), true, "control", "")
		}
		msg := fmt.Sprintf("feedback %s · interval %s · expand=%v",
			map[bool]string{true: "ON", false: "OFF"}[cfg.Enabled],
			core.FeedbackIntervalLabel(cfg.IntervalSec),
			cfg.Expand,
		)
		return rt.pushOutputFull(ctx, mailboxID, env.SessionID, env.CommandID, "system", msg, true, "control", "")
	case "feedback_now", "snapshot":
		sid := p.SessionID
		if sid == "" {
			sid = env.SessionID
		}
		return rt.pushFeedbackSample(ctx, mailboxID, sid, env.CommandID, true)
	case "open":
		return rt.controlOpen(ctx, mailboxID, env.CommandID, p)
	case "lock":
		sid := firstNonEmpty(p.SessionID, env.SessionID)
		lease, err := core.AcquireLease(sid, p.Holder, p.Goal, time.Duration(p.TTLSec)*time.Second, p.Steal)
		msg := "lock ok · " + sid
		if err != nil {
			msg = "lock failed · " + err.Error()
		} else {
			msg = fmt.Sprintf("lock · %s · %s until %s", sid, lease.Holder, lease.Expires.UTC().Format(time.RFC3339))
		}
		return rt.pushOutputFull(ctx, mailboxID, sid, env.CommandID, "system", msg, true, "control", "lock")
	case "unlock", "release":
		sid := firstNonEmpty(p.SessionID, env.SessionID)
		err := core.ReleaseLease(sid, p.Holder, p.Steal)
		msg := "unlock · " + sid
		if err != nil {
			msg = "unlock failed · " + err.Error()
		}
		return rt.pushOutputFull(ctx, mailboxID, sid, env.CommandID, "system", msg, true, "control", "unlock")
	case "result":
		sid := firstNonEmpty(p.SessionID, env.SessionID)
		text, method := "", ""
		if rt.hybrid != nil {
			cap, _ := rt.hybrid.Capture(sid)
			text, method = cap.Text, cap.Method
		}
		idle := inject.PeekIdle(text, method)
		chunk := idle.Excerpt
		if chunk == "" {
			chunk = "result empty · method=" + method
		}
		return rt.pushOutputFull(ctx, mailboxID, sid, env.CommandID, "stdout",
			trimChunk(chunk, 12*1024), true, "result", method)
	case "task":
		t, err := core.UpsertTask(core.Task{
			ID: p.TaskID, SessionID: firstNonEmpty(p.SessionID, env.SessionID),
			Holder: p.Holder, Goal: p.Goal, Status: p.Status,
			LastExcerpt: p.Excerpt, LastJudge: p.Judge, CommandID: env.CommandID,
		})
		msg := "task " + t.ID + " · " + t.Status
		if err != nil {
			msg = "task failed · " + err.Error()
		}
		return rt.pushOutputFull(ctx, mailboxID, t.SessionID, env.CommandID, "system", msg, true, "control", "task")
	default:
		return rt.pushOutputFull(ctx, mailboxID, env.SessionID, env.CommandID, "system",
			"unknown control action: "+action, true, "control", "")
	}
}

func (rt *agentRuntime) controlOpen(ctx context.Context, mailboxID, commandID string, p grok.ControlPayload) error {
	if rt.hybrid == nil {
		return rt.pushOutputFull(ctx, mailboxID, p.SessionID, commandID, "system",
			"open failed: no session manager", true, "control", "open")
	}
	res, err := rt.hybrid.OpenOrAttach(inject.OpenRequest{
		SessionID: p.SessionID, CWD: p.CWD, Resume: p.Resume,
		Command: p.Command, Title: p.Title, Holder: p.Holder,
	})
	if err != nil {
		return rt.pushOutputFull(ctx, mailboxID, p.SessionID, commandID, "system",
			"open failed: "+err.Error(), true, "control", "open")
	}
	if rt.scanner != nil {
		cwd := res.CWD
		if cwd == "" {
			cwd = "gbr-open-" + res.SessionID
		}
		rt.scanner.Track(session.Candidate{
			CWD: cwd, Shell: firstNonEmpty(res.Command, "grok-build"),
			PID: res.PID, Title: firstNonEmpty(p.Title, "Grok Build"), PreferID: res.SessionID,
		})
	}
	if _, lerr := core.AcquireLease(res.SessionID, p.Holder, p.Goal, time.Duration(p.TTLSec)*time.Second, p.Steal); lerr != nil {
		res.Note = strings.TrimSpace(res.Note + " · lease: " + lerr.Error())
	}
	if strings.TrimSpace(p.Goal) != "" {
		_, _ = core.UpsertTask(core.Task{SessionID: res.SessionID, Holder: p.Holder, Goal: p.Goal, Status: core.TaskOpen})
	}
	msg := fmt.Sprintf("opened %s · method=%s · %s", res.SessionID, res.Method, res.Note)
	if res.Attached {
		msg = fmt.Sprintf("attached %s · %s", res.SessionID, res.Note)
	}
	return rt.pushOutputFull(ctx, mailboxID, res.SessionID, commandID, "system", msg, true, "control", "open")
}

func (rt *agentRuntime) feedbackLoop(ctx context.Context, mailboxID string) {
	// Adaptive ticker: re-read config each tick so phone control takes effect without restart.
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	var lastFire time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cfg := core.LoadFeedback()
			if !cfg.Enabled {
				continue
			}
			interval := cfg.FeedbackTickerDuration()
			if time.Since(lastFire) < interval {
				continue
			}
			lastFire = time.Now()
			for _, sid := range rt.feedbackSessionIDs(cfg) {
				_ = rt.pushFeedbackSample(ctx, mailboxID, sid, uuid.NewString(), false)
			}
		}
	}
}

func (rt *agentRuntime) feedbackSessionIDs(cfg core.FeedbackConfig) []string {
	if cfg.SessionID != "" {
		return []string{cfg.SessionID}
	}
	var ids []string
	if rt.scanner != nil && rt.scanner.Registry != nil {
		for _, s := range rt.scanner.Registry.List() {
			ids = append(ids, s.ID)
		}
	}
	for _, id := range rt.hybrid.ManagedIDs() {
		ids = append(ids, id)
	}
	// Grok Build first, then the agent shell, then other terminals. No cap.
	return prioritizeSessionIDs(ids)
}

func (rt *agentRuntime) pushFeedbackSample(ctx context.Context, mailboxID, sessionID, commandID string, force bool) error {
	if sessionID == "" {
		return nil
	}
	cfg := core.LoadFeedback()
	cap, _ := rt.hybrid.Capture(sessionID)
	text := strings.TrimSpace(cap.Text)
	title := rt.sessionTitle(sessionID)
	method := cap.Method
	if text == "" {
		// Title-only peek — still useful when console capture is empty.
		if title == "" && !force {
			return nil
		}
		chunk := "feedback peek · capture empty"
		if method != "" {
			chunk += " (" + method + ")"
		}
		if title != "" {
			chunk += " · " + title
		}
		// Only send title peeks at most when force or hash changes.
		return rt.pushIfChanged(ctx, mailboxID, sessionID, commandID, "system", chunk, method, force, cfg)
	}
	chunk := trimChunk(text, cfg.MaxFeedbackChunk())
	return rt.pushIfChanged(ctx, mailboxID, sessionID, commandID, "stdout", chunk, method, force, cfg)
}

func (rt *agentRuntime) pushIfChanged(ctx context.Context, mailboxID, sessionID, commandID, stream, chunk, method string, force bool, cfg core.FeedbackConfig) error {
	sum := sha256.Sum256([]byte(chunk))
	h := hex.EncodeToString(sum[:8])
	rt.fbMu.Lock()
	if rt.fbLastHash == nil {
		rt.fbLastHash = map[string]string{}
	}
	prev := rt.fbLastHash[sessionID]
	if !force && prev == h {
		rt.fbMu.Unlock()
		return nil
	}
	rt.fbLastHash[sessionID] = h
	rt.fbMu.Unlock()
	return rt.pushOutputFull(ctx, mailboxID, sessionID, commandID, stream, chunk, false, "periodic", method)
}

func cmdFeedback(args []string) int {
	if len(args) == 0 {
		cfg := core.LoadFeedback()
		fmt.Printf("feedback enabled=%v interval=%s expand=%v session=%q\n",
			cfg.Enabled, core.FeedbackIntervalLabel(cfg.IntervalSec), cfg.Expand, cfg.SessionID)
		path, _ := core.FeedbackPath()
		fmt.Printf("file: %s\n", path)
		return 0
	}
	cfg := core.LoadFeedback()
	switch strings.ToLower(args[0]) {
	case "status":
		fmt.Printf("feedback enabled=%v interval=%s expand=%v\n",
			cfg.Enabled, core.FeedbackIntervalLabel(cfg.IntervalSec), cfg.Expand)
		return 0
	case "on", "enable", "true", "1":
		cfg.Enabled = true
	case "off", "disable", "false", "0":
		cfg.Enabled = false
	case "expand":
		cfg.Expand = true
	case "compact", "minimal":
		cfg.Expand = false
	case "interval":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: gbr-agent feedback interval 5|10|60|600|3600")
			return 2
		}
		var sec int
		switch strings.ToLower(args[1]) {
		case "5", "5s":
			sec = 5
		case "10", "10s":
			sec = 10
		case "60", "1m", "1min":
			sec = 60
		case "600", "10m", "10min":
			sec = 600
		case "3600", "1h", "60m":
			sec = 3600
		default:
			if _, err := fmt.Sscanf(args[1], "%d", &sec); err != nil {
				fmt.Fprintln(os.Stderr, "bad interval")
				return 2
			}
		}
		cfg.IntervalSec = sec
	default:
		fmt.Fprintln(os.Stderr, "usage: gbr-agent feedback [status|on|off|expand|compact|interval SEC]")
		return 2
	}
	if err := core.SaveFeedback(cfg); err != nil {
		slog.Error("save feedback", "err", err)
		return 1
	}
	fmt.Printf("ok feedback enabled=%v interval=%s expand=%v\n",
		cfg.Enabled, core.FeedbackIntervalLabel(cfg.IntervalSec), cfg.Expand)
	return 0
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

func (rt *agentRuntime) refreshSessionStore() {
	if rt == nil || rt.store == nil {
		return
	}
	if err := rt.store.Load(); err != nil {
		slog.Debug("session store reload", "err", err)
	}
}

func (rt *agentRuntime) listSessionPayloads() []map[string]any {
	rt.refreshSessionStore()
	var labels map[string]string
	if rt.store != nil {
		labels = rt.store.LabelsSnapshot()
	}
	var out []map[string]any
	if rt.scanner != nil && rt.scanner.Registry != nil {
		for _, s := range rt.scanner.Registry.List() {
			title := session.ResolveDisplayTitle(s.ID, s.Title, s.Shell, labels)
			if title != "" && title != s.Title {
				upd := s.Clone()
				upd.Title = title
				rt.scanner.Registry.Upsert(upd)
			}
			row := map[string]any{
				"session_id": s.ID,
				"cwd":        s.CWD,
				"shell":      s.Shell,
				"title":      title,
				"os":         runtime.GOOS,
				"git_remote": s.GitRemote,
			}
			if l, ok := core.GetLease(s.ID); ok {
				row["lock"] = l.Public(false)
			}
			out = append(out, row)
		}
	}
	// Include managed PTY sessions not already listed
	seen := map[string]bool{}
	for _, m := range out {
		if id, ok := m["session_id"].(string); ok {
			seen[id] = true
		}
	}
	if rt.hybrid != nil {
		for _, id := range rt.hybrid.ManagedIDs() {
			if seen[id] {
				continue
			}
			title := session.ResolveDisplayTitle(id, "gbr managed shell", "managed", labels)
			row := map[string]any{
				"session_id": id,
				"shell":      "managed",
				"title":      title,
				"os":         runtime.GOOS,
			}
			if l, ok := core.GetLease(id); ok {
				row["lock"] = l.Public(false)
			}
			out = append(out, row)
		}
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
	sort.SliceStable(out, func(i, j int) bool {
		idi, _ := out[i]["session_id"].(string)
		idj, _ := out[j]["session_id"].(string)
		ti, _ := out[i]["title"].(string)
		tj, _ := out[j]["title"].(string)
		si, _ := out[i]["shell"].(string)
		sj, _ := out[j]["shell"].(string)
		pi := sessionPriority(idi, ti, si)
		pj := sessionPriority(idj, tj, sj)
		if pi != pj {
			return pi < pj
		}
		return false
	})
	return clipAndLog(out, "list")
}

func (rt *agentRuntime) registerLoop(ctx context.Context, mailboxID string) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	rt.publishSessionSnapshot(ctx, mailboxID)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rt.publishSessionSnapshot(ctx, mailboxID)
		}
	}
}

func (rt *agentRuntime) noteSessionSnapshot(all []*session.Session) bool {
	parts := make([]string, 0, len(all))
	for _, s := range all {
		if s == nil {
			continue
		}
		parts = append(parts, s.ID+"\t"+s.Title)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	fp := hex.EncodeToString(sum[:8])
	rt.snapMu.Lock()
	defer rt.snapMu.Unlock()
	if fp == rt.lastSnap {
		return false
	}
	rt.lastSnap = fp
	return true
}

func (rt *agentRuntime) heartbeatSessions() []grok.HeartbeatSession {
	raw := rt.listSessionPayloads()
	out := make([]grok.HeartbeatSession, 0, len(raw))
	for _, m := range raw {
		id, _ := m["session_id"].(string)
		if id == "" {
			continue
		}
		title, _ := m["title"].(string)
		out = append(out, grok.HeartbeatSession{SessionID: id, Title: title})
	}
	return out
}

func (rt *agentRuntime) publishSessionSnapshot(ctx context.Context, mailboxID string) {
	sessions := rt.listSessionPayloads()
	env, err := grok.NewEnvelope(grok.TypeList, rt.dev.DeviceID, "", uuid.NewString(), map[string]any{
		"sessions": sessions,
		"replace":  true,
		"reason":   "snapshot",
	})
	if err == nil {
		if err := rt.pushEnv(ctx, mailboxID, env); err != nil {
			slog.Debug("list snapshot push", "err", err)
		}
	}
	rt.publishRegisters(ctx, mailboxID)
}

func (rt *agentRuntime) publishRegisters(ctx context.Context, mailboxID string) {
	regs := rt.scanner.Registers(rt.dev.DeviceID)
	sort.SliceStable(regs, func(i, j int) bool {
		pi := sessionPriority(regs[i].SessionID, regs[i].Payload.Title, regs[i].Payload.Shell)
		pj := sessionPriority(regs[j].SessionID, regs[j].Payload.Title, regs[j].Payload.Shell)
		if pi != pj {
			return pi < pj
		}
		return false
	})
	regs = clipAndLog(regs, "register")
	for _, msg := range regs {
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
			roster := rt.heartbeatSessions()
			env, err := grok.NewEnvelope(grok.TypeHeartbeat, rt.dev.DeviceID, "", uuid.NewString(), grok.HeartbeatPayload{
				SessionCount: len(roster),
				Status:       "alive",
				AgentVersion: version,
				Relay:        rt.relay.Base(),
				OS:           runtime.GOOS,
				Sessions:     roster,
			})
			if err != nil {
				continue
			}
			hbErr := rt.pushEnv(ctx, mailboxID, env)
			if hbErr != nil {
				slog.Warn("heartbeat", "err", hbErr)
			}
			core.GlobalWatchdog.TouchRoster()
			core.GlobalWatchdog.TouchRelay(hbErr == nil)
			trace.Emit(trace.Event{
				Hop:    trace.HopAgentHeartbeat,
				Type:   "heartbeat",
				OK:     hbErr == nil,
				Detail: fmt.Sprintf("sessions=%d version=%s", len(roster), version),
			})
		}
	}
}

func cmdPair(args []string) int {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	code := fs.String("code", "", "optional: use this code (default: PC generates one for phone camera QR)")
	name := fs.String("name", "", "device display name")
	conv := fs.String("conv", "", "optional mailbox id (default: gbr-<code>)")
	relayURL := fs.String("relay", "", "relay base URL")
	noOpen := fs.Bool("no-open", false, "do not open the browser QR page")
	_ = fs.Parse(args)

	// Preferred: PC generates the code. Phone camera scans the browser QR.
	// Legacy: -code from phone still works for manual entry.
	generated := false
	codeNorm := strings.ToUpper(strings.TrimSpace(*code))
	codeNorm = strings.ReplaceAll(codeNorm, " ", "")
	codeNorm = strings.ReplaceAll(codeNorm, "-", "")
	if codeNorm == "" {
		var err error
		codeNorm, err = generatePairingCode(8)
		if err != nil {
			slog.Error("generate pairing code", "err", err)
			return 1
		}
		generated = true
	}

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
	if mbKey == "" {
		// Live relay is enforce-mode; without a key poll/heartbeat always 401.
		// Treat empty key as hard failure so users never get a false "paired".
		slog.Error("relay issued no mailbox key — upgrade agent and re-pair",
			"hint", "download latest from https://grokbuildremote.com/#download")
		return 1
	}
	if err := dev.SetMailboxKey(mbKey); err != nil {
		slog.Error("save mailbox key", "err", err)
		return 1
	}
	// Ensure client uses key for the follow-up pair envelope push.
	rc.SetKey(mbKey)

	ident := collectPairIdent(codeNorm, dev.DeviceName, mailboxID, mbKey)

	// Also push a pair envelope into the mailbox so mobile can observe.
	payload := grok.PairPayload{
		PairingCode:  codeNorm,
		DeviceName:   dev.DeviceName,
		AgentVersion: ident.Version,
		SHA256:       ident.SHA256,
		OS:           ident.OS,
		Arch:         ident.Arch,
		Host:         ident.Host,
		KeyFP:        ident.KeyFP,
	}
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

	deepLink := pairDeepLinkIdent(ident)
	webURL := pairPageURL(ident)

	fmt.Printf("\n")
	fmt.Printf("=== PAIR — phone camera scans the QR on this PC ===\n")
	if generated {
		fmt.Printf("code (generated on PC):  %s\n", codeNorm)
	} else {
		fmt.Printf("code (from -code):       %s\n", codeNorm)
	}
	fmt.Print(formatPairIdent(ident))
	fmt.Printf("deep link:               %s\n", deepLink)
	fmt.Printf("browser page:            %s\n", webURL)
	fmt.Printf("\n")
	fmt.Printf("1) Leave this terminal open / next run agent.\n")
	fmt.Printf("2) On the PHONE: open Build Remote Agent → Scan QR\n")
	fmt.Printf("   (point camera at the browser window — not the phone screen)\n")
	fmt.Printf("3) Then: gbr-agent run\n")
	fmt.Printf("\n")
	fmt.Printf("paired device_id=%s mailbox=%s name=%s\n", dev.DeviceID, mailboxID, dev.DeviceName)
	fmt.Printf("mailbox_key: set (%d chars) — fingerprint %s (key not printed)\n", len(mbKey), ident.KeyFP)
	fmt.Printf("relay=%s\n", rc.Base())
	fmt.Printf("device file: %s\n", dev.Path())
	fmt.Printf("verify: gbr-agent status   # must show mailbox_key: set + binary sha256\n")

	if !*noOpen {
		// Local HTML always works offline; also try hosted pair.html.
		if local, err := writeLocalPairHTML(codeNorm, dev.DeviceName, deepLink, ident); err != nil {
			slog.Warn("local pair HTML", "err", err)
			if err := openBrowser(webURL); err != nil {
				slog.Warn("open browser", "err", err, "url", webURL)
				fmt.Printf("open this URL manually: %s\n", webURL)
			}
		} else {
			if err := openBrowser(local); err != nil {
				slog.Warn("open local pair page", "err", err)
				_ = openBrowser(webURL)
			} else {
				fmt.Printf("opened QR page: %s\n", local)
			}
		}
	}
	return 0
}

func generatePairingCode(n int) (string, error) {
	if n < 6 {
		n = 8
	}
	out := make([]byte, n)
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := 0; i < n; i++ {
		out[i] = crockfordAlphabet[int(buf[i])%len(crockfordAlphabet)]
	}
	return string(out), nil
}

func pairDeepLink(code, deviceName string) string {
	s := "gbr://pair?v=1&code=" + code
	if deviceName != "" {
		s += "&device_name=" + urlQueryEscape(deviceName)
	}
	return s
}

func urlQueryEscape(s string) string {
	// Minimal escape for query values (device names are short).
	r := strings.NewReplacer(
		" ", "%20",
		"&", "%26",
		"=", "%3D",
		"#", "%23",
		"?", "%3F",
		"+", "%2B",
	)
	return r.Replace(s)
}

func writeLocalPairHTML(code, deviceName, deepLink string, ident pairIdent) (string, error) {
	png, err := qrcode.Encode(deepLink, qrcode.Medium, 512)
	if err != nil {
		return "", err
	}
	b64 := base64.StdEncoding.EncodeToString(png)
	title := "Scan with phone camera"
	if deviceName != "" {
		title = "Scan to pair · " + deviceName
	}
	idBits := ""
	if ident.Version != "" {
		idBits += fmt.Sprintf("<p class=\"m\">gbr-agent <b>%s</b> · %s/%s</p>", htmlEscape(ident.Version), htmlEscape(ident.OS), htmlEscape(ident.Arch))
	}
	if ident.SHA256 != "" {
		idBits += fmt.Sprintf("<p class=\"p\">sha256 %s</p>", htmlEscape(ident.SHA256))
	}
	if ident.Host != "" {
		idBits += fmt.Sprintf("<p class=\"m\">host %s</p>", htmlEscape(ident.Host))
	}
	if ident.KeyFP != "" {
		idBits += fmt.Sprintf("<p class=\"m\">pair-key fingerprint <b>%s</b> (not the key)</p>", htmlEscape(ident.KeyFP))
	}
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>%s</title>
<style>
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
font-family:ui-monospace,Menlo,Consolas,monospace;background:#020402;color:#3dff7a}
.card{max-width:440px;padding:28px;border:1px solid #1a3a22;border-radius:16px;background:#081208;text-align:center}
h1{font-size:1.15rem;margin:0 0 8px} .m{color:#6a9a78;font-size:.85rem;line-height:1.45}
.code{font-size:2rem;font-weight:800;letter-spacing:.12em;margin:16px 0 8px}
img{background:#fff;padding:12px;border-radius:12px;width:260px;height:260px}
.p{font-size:.7rem;color:#6a9a78;word-break:break-all}
</style></head><body><div class="card">
<h1>%s</h1>
<p class="m">Open the mobile app → <b>Scan QR</b>. Point the <b>phone camera</b> at this window.</p>
<img alt="pair QR" src="data:image/png;base64,%s"/>
<div class="code">%s</div>
%s
<p class="p">%s</p>
<p class="m">Then on this PC: <b>gbr-agent run</b></p>
</div></body></html>`,
		htmlEscape(title), htmlEscape(title), b64, htmlEscape(code), idBits, htmlEscape(deepLink))

	dir := filepath.Join(os.TempDir(), "gbr-pair")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "pair-"+code+".html")
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		return "", err
	}
	// file:// URL
	abs, err := filepath.Abs(path)
	if err != nil {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		return "file:///" + strings.ReplaceAll(abs, "\\", "/"), nil
	}
	return "file://" + abs, nil
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func cmdRename(args []string) int {
	fs := flag.NewFlagSet("rename", flag.ExitOnError)
	name := fs.String("name", "", "new device display name")
	sessionID := fs.String("session", "", "optional: rename current cwd to this session_id")
	_ = fs.Parse(args)

	if *sessionID != "" {
		store, err := session.OpenStore("")
		if err != nil {
			slog.Error("store", "err", err)
			return 1
		}
		// -session + -name → phone display title (does not change session_id).
		if strings.TrimSpace(*name) != "" {
			if err := store.SetLabel(*sessionID, *name); err != nil {
				slog.Error("session label", "err", err)
				return 1
			}
			fmt.Printf("session %s title=%s\n", *sessionID, strings.TrimSpace(*name))
			return 0
		}
		if !grok.ValidSessionID(*sessionID) {
			slog.Error("invalid session_id", "id", *sessionID)
			return 2
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
	ui := inject.Default()
	defer func() { _ = ui.Close() }()
	sc := session.NewScanner(store, reg, func(ctx context.Context) ([]session.Candidate, error) {
		wins, err := ui.Discover()
		if err != nil {
			return nil, err
		}
		sortWindowsGrokFirst(wins)
		out, _, _ := windowsToCandidates(wins)
		return out, nil
	})
	cwd, _ := os.Getwd()
	sc.Track(session.Candidate{CWD: cwd, Shell: defaultShellName(), Title: "gbr-agent"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := sc.ScanOnce(ctx); err != nil {
		slog.Warn("scan", "err", err)
	}
	list := reg.List()
	sort.SliceStable(list, func(i, j int) bool {
		pi := sessionPriority(list[i].ID, list[i].Title, list[i].Shell)
		pj := sessionPriority(list[j].ID, list[j].Title, list[j].Shell)
		if pi != pj {
			return pi < pj
		}
		return list[i].ID < list[j].ID
	})
	for _, s := range list {
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
	if sha := runningBinarySHA256(); sha != "" {
		fmt.Printf("binary_sha256: %s\n", sha)
	}
	if dev.MailboxKey != "" {
		fmt.Printf("mailbox_key: set (%d chars) — fingerprint %s — requests authenticated\n", len(dev.MailboxKey), keyFingerprint(dev.MailboxKey))
	} else {
		fmt.Printf("mailbox_key: NOT SET — unauthenticated; re-pair to obtain one\n")
	}
	fmt.Printf("relay:       %s (%s)\n", rc.Base(), relayOK)
	if dev.MailboxConversationID != "" {
		fmt.Printf("relay_bot:   %s/v1/mb/%s/bot\n", strings.TrimRight(rc.Base(), "/"), dev.MailboxConversationID)
	}
	fmt.Printf("local_bot:   http://127.0.0.1:%d  (while gbr-agent run; gbr-agent bot)\n", botPortFromEnv())
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
	printStatusFleet(dev.MailboxConversationID, dev.MailboxKey != "")
	if dev.MailboxConversationID == "" {
		fmt.Printf("hint: run  gbr-agent pair -code YOURCODE\n")
	} else {
		fmt.Printf("hint: run  gbr-agent -log=info run\n")
	}
	return 0
}

func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	full := fs.Bool("net", true, "include network/firewall checks (default true)")
	_ = fs.Parse(args)
	var results []doctor.Result
	if *full {
		results = doctor.RunAll()
	} else {
		results = doctor.Run()
	}
	fmt.Print(doctor.Format(results))
	for _, r := range results {
		// site.* optional warnings from netcheck embedded in RunAll
		if !r.OK && !strings.HasPrefix(r.Name, "site.") {
			return 1
		}
	}
	return 0
}

func cmdNetcheck(args []string) int {
	fs := flag.NewFlagSet("netcheck", flag.ExitOnError)
	relayURL := fs.String("relay", "", "relay base URL (else GBR_RELAY_URL / default)")
	showDoc := fs.Bool("doc", false, "print firewall/ports documentation and exit 0")
	_ = fs.Parse(args)
	if *showDoc {
		fmt.Print(doctor.NetworkDoc)
		return 0
	}
	results := doctor.RunNetwork(*relayURL)
	fmt.Print(doctor.FormatNetwork(results))
	for _, r := range results {
		if !r.OK && !strings.HasPrefix(r.Name, "site.") {
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
		fmt.Println("✓ gbr-agent auto-start installed (user session background)")
		fmt.Println("  Windows: Task Scheduler logon · Mac: LaunchAgent · Linux: systemd --user")
		fmt.Println("  Pair first if needed: gbr-agent pair   or   gbr-agent pair-as-mailbox")
		fmt.Println("  Check: gbr-agent service status")
		if u := strings.TrimSpace(os.Getenv("GBR_RELAY_URL")); u != "" {
			fmt.Printf("  note: GBR_RELAY_URL=%s is set in this shell — ensure the service inherits it (see SELF-HOSTED-RELAY.md)\n", u)
		}
		st, _ := service.Status()
		fmt.Print(st)
		return 0
	case "uninstall":
		if err := service.Uninstall(); err != nil {
			slog.Error("service uninstall", "err", err)
			return 1
		}
		fmt.Println("service uninstalled (auto-start removed)")
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
