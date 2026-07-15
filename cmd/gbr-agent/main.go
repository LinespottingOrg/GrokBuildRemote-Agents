// Command gbr-agent is the Grok Build Remote PC agent.
//
//	gbr-agent run              — start mailbox poll loop (Mode B)
//	gbr-agent version          — print version
//	gbr-agent pair -code CODE  — complete mobile pairing
//	gbr-agent rename -name N   — set device display name
package main

import (
	"context"
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
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/grok"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
	"github.com/google/uuid"
)

// Build-time metadata (release.yml / scripts/build-all.*):
//
//	-X main.version=… -X main.commit=… -X main.date=…
var (
	version = "0.1.0-dev"
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
	fmt.Fprintf(os.Stderr, `gbr-agent — Grok Build Remote agent (protocol gbr/1)

Usage:
  gbr-agent [-log=info] version
  gbr-agent [-log=info] run [-session ID] [-conv MAILBOX_ID]
  gbr-agent [-log=info] pair -code PAIRING_CODE [-name DEVICE_NAME] [-conv MAILBOX_ID]
  gbr-agent [-log=info] rename -name DEVICE_NAME

Environment:
  GBR_API_KEY / XAI_API_KEY     xAI API key (or %%USERPROFILE%%\.grok\config.json)
  GBR_BASE_URL / XAI_BASE_URL   default https://api.x.ai/v1

Device identity: %%USERPROFILE%%\.gbr\device.json

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

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	session := fs.String("session", "", "session_id to register (optional)")
	conv := fs.String("conv", "", "mailbox conversation id (else from device.json)")
	_ = fs.Parse(args)

	cfg, err := core.LoadConfig()
	if err != nil {
		slog.Error("config", "err", err)
		return 1
	}
	dev, err := core.LoadOrCreateDevice()
	if err != nil {
		slog.Error("device", "err", err)
		return 1
	}

	mailboxID := firstNonEmpty(*conv, dev.MailboxConversationID)
	if mailboxID == "" {
		slog.Error("no mailbox conversation; run: gbr-agent pair -code ... first")
		return 1
	}

	client := grok.NewClient(
		cfg.BaseURL,
		cfg.APIKey,
		time.Duration(cfg.HTTPTimeoutSec)*time.Second,
		grok.WithLogger(slog.Default()),
	)
	client.SetConversation(mailboxID)

	inj := newInjector()
	defer func() { _ = inj.Close() }()

	slog.Info("gbr-agent starting",
		"version", version,
		"device_id", dev.DeviceID,
		"device_name", dev.DeviceName,
		"mailbox", mailboxID,
		"base_url", cfg.BaseURL,
		"api_key", core.RedactKey(cfg.APIKey),
		"inject", runtime.GOOS,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *session != "" {
		if !grok.ValidSessionID(*session) {
			slog.Error("invalid session_id", "session", *session)
			return 1
		}
		if err := postRegister(ctx, client, dev, *session); err != nil {
			slog.Error("register", "err", err)
		}
	}

	go heartbeatLoop(ctx, client, dev)

	interval := time.Duration(cfg.PollIntervalSec) * time.Second
	handler := func(ctx context.Context, env *grok.Envelope) error {
		return handleEnvelope(ctx, client, dev, inj, env)
	}

	err = client.StartMailboxLoop(ctx, dev.DeviceID, interval, handler)
	if err != nil && ctx.Err() != nil {
		slog.Info("shutdown")
		return 0
	}
	if err != nil {
		slog.Error("mailbox loop", "err", err)
		return 1
	}
	return 0
}

func cmdPair(args []string) int {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	code := fs.String("code", "", "8-char pairing code from mobile")
	name := fs.String("name", "", "device display name")
	conv := fs.String("conv", "", "optional mailbox conversation id (generated if empty)")
	_ = fs.Parse(args)

	if strings.TrimSpace(*code) == "" {
		slog.Error("pair requires -code")
		return 2
	}

	cfg, err := core.LoadConfig()
	if err != nil {
		slog.Error("config", "err", err)
		return 1
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

	mailboxID := firstNonEmpty(*conv, dev.MailboxConversationID)
	if mailboxID == "" {
		mailboxID = grok.CreateMailboxConversation()
	}
	if err := dev.SetMailboxConversationID(mailboxID); err != nil {
		slog.Error("save mailbox id", "err", err)
		return 1
	}

	client := grok.NewClient(
		cfg.BaseURL,
		cfg.APIKey,
		time.Duration(cfg.HTTPTimeoutSec)*time.Second,
		grok.WithLogger(slog.Default()),
	)
	client.SetConversation(mailboxID)

	payload := grok.PairPayload{
		PairingCode: strings.ToUpper(strings.TrimSpace(*code)),
		DeviceName:  dev.DeviceName,
	}
	env, err := grok.NewEnvelope(grok.TypePair, dev.DeviceID, "", uuid.NewString(), payload)
	if err != nil {
		slog.Error("envelope", "err", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.HTTPTimeoutSec)*time.Second)
	defer cancel()

	if err := client.PostEnvelope(ctx, env); err != nil {
		slog.Error("pair post failed", "err", err)
		return 1
	}

	fmt.Printf("paired device_id=%s mailbox=%s name=%s\n", dev.DeviceID, mailboxID, dev.DeviceName)
	fmt.Printf("device file: %s\n", dev.Path())
	return 0
}

func cmdRename(args []string) int {
	fs := flag.NewFlagSet("rename", flag.ExitOnError)
	name := fs.String("name", "", "new device display name")
	_ = fs.Parse(args)
	if strings.TrimSpace(*name) == "" {
		slog.Error("rename requires -name")
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

func handleEnvelope(ctx context.Context, client *grok.Client, dev *core.Device, inj inject.Injector, env *grok.Envelope) error {
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
		err := inj.Inject(env.SessionID, req)
		chunk := "ok"
		if err != nil {
			chunk = err.Error()
		}
		out, nerr := grok.NewEnvelope(grok.TypeOutput, dev.DeviceID, env.SessionID, env.CommandID, grok.OutputPayload{
			Stream: "system",
			Chunk:  chunk,
			EOF:    true,
		})
		if nerr != nil {
			return nerr
		}
		return client.PostEnvelope(ctx, out)

	case grok.TypeList:
		out, err := grok.NewEnvelope(grok.TypeList, dev.DeviceID, "", env.CommandID, map[string]any{
			"sessions": []any{},
		})
		if err != nil {
			return err
		}
		return client.PostEnvelope(ctx, out)

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

func postRegister(ctx context.Context, client *grok.Client, dev *core.Device, sessionID string) error {
	cwd, _ := os.Getwd()
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "pwsh"
	}
	payload := grok.RegisterPayload{
		CWD:   filepath.ToSlash(cwd),
		Shell: shell,
		OS:    runtime.GOOS,
		Title: "gbr-agent",
	}
	env, err := grok.NewEnvelope(grok.TypeRegister, dev.DeviceID, sessionID, uuid.NewString(), payload)
	if err != nil {
		return err
	}
	return client.PostEnvelope(ctx, env)
}

func heartbeatLoop(ctx context.Context, client *grok.Client, dev *core.Device) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			env, err := grok.NewEnvelope(grok.TypeHeartbeat, dev.DeviceID, "", uuid.NewString(), grok.HeartbeatPayload{
				SessionCount: 0,
				Status:       "alive",
			})
			if err != nil {
				slog.Error("heartbeat envelope", "err", err)
				continue
			}
			hctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err = client.PostEnvelope(hctx, env)
			cancel()
			if err != nil {
				slog.Warn("heartbeat post", "err", err)
			}
		}
	}
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}
