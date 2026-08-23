package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inbox"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/session"
	"github.com/google/uuid"
)

func inboxEnabled() bool {
	v := strings.TrimSpace(os.Getenv("GBR_INBOX_WATCH"))
	if v == "0" || strings.EqualFold(v, "off") || strings.EqualFold(v, "false") {
		return false
	}
	return true
}

func inboxPoll() time.Duration {
	s := strings.TrimSpace(os.Getenv("GBR_INBOX_POLL"))
	if s == "" {
		return 20 * time.Second
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 5*time.Second {
		return 20 * time.Second
	}
	return d
}

func cmdInbox(args []string) int {
	fs := flag.NewFlagSet("inbox", flag.ContinueOnError)
	once := fs.Bool("once", true, "run one tick and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	_ = once
	w := inbox.New(os.Getenv("GBR_INBOX_REPO"), os.Getenv("GBR_INBOX_LABEL"), nil)
	acts, err := w.Tick(nil)
	if err != nil {
		slog.Error("inbox", "err", err)
		fmt.Fprintf(os.Stderr, "inbox: %v\n", err)
		return 1
	}
	if len(acts) == 0 {
		fmt.Println("inbox: no new boss-steer comments (seeded or quiet)")
		return 0
	}
	for _, a := range acts {
		fmt.Printf("inbox action=%s issue=#%d title=%q session=%s bytes=%d\n",
			a.Kind, a.Issue, a.Title, a.SessionID, len(a.Text))
	}
	fmt.Println("inbox: dry (no inject). Start `gbr-agent run` to apply.")
	return 0
}

func (rt *agentRuntime) inboxLoop(ctx context.Context) {
	if rt == nil || !inboxEnabled() {
		return
	}
	w := inbox.New(os.Getenv("GBR_INBOX_REPO"), os.Getenv("GBR_INBOX_LABEL"), nil)
	slog.Info("inbox watch on",
		"repo", w.Repo, "label", w.Label, "poll", inboxPoll().String())
	t := time.NewTicker(inboxPoll())
	defer t.Stop()
	rt.inboxTick(ctx, w)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rt.inboxTick(ctx, w)
		}
	}
}

func (rt *agentRuntime) inboxTick(ctx context.Context, w *inbox.Watcher) {
	if ctx.Err() != nil {
		return
	}
	var sessions []inbox.Session
	for _, row := range rt.listSessionPayloads() {
		id, _ := row["session_id"].(string)
		title, _ := row["title"].(string)
		if id == "" || id == "session" || title == "gbr-agent" {
			continue
		}
		sessions = append(sessions, inbox.Session{ID: id, Title: title})
	}
	acts, err := w.Tick(sessions)
	if err != nil {
		slog.Warn("inbox tick", "err", err)
		return
	}
	for _, a := range acts {
		if err := rt.applyInbox(a); err != nil {
			slog.Error("inbox apply", "issue", a.Issue, "kind", a.Kind, "err", err)
			continue
		}
		slog.Info("inbox applied", "issue", a.Issue, "kind", a.Kind, "title", a.Title, "session", a.SessionID)
	}
}

func (rt *agentRuntime) applyInbox(a inbox.Action) error {
	if strings.TrimSpace(a.Text) == "" {
		return fmt.Errorf("empty text")
	}
	if a.Kind == "inject" {
		return rt.hybrid.Inject(a.SessionID, inject.InjectRequest{
			SessionID: a.SessionID,
			CommandID: "inbox-" + uuid.NewString()[:8],
			Text:      a.Text,
			Submit:    true,
		})
	}
	if a.Kind != "spawn" {
		return fmt.Errorf("unknown kind %s", a.Kind)
	}
	res, err := rt.hybrid.OpenOrAttach(inject.OpenRequest{
		Command: "grok",
		Title:   a.Title,
		Holder:  "inbox",
		CWD:     os.Getenv("HOME"),
	})
	if err != nil {
		return err
	}
	if rt.scanner != nil {
		cwd := res.CWD
		if cwd == "" {
			cwd = "gbr-open-" + res.SessionID
		}
		rt.scanner.Track(session.Candidate{
			CWD:      cwd,
			Shell:    "grok-build",
			PID:      res.PID,
			Title:    a.Title,
			PreferID: res.SessionID,
		})
	}
	time.Sleep(2 * time.Second)
	_ = rt.hybrid.Inject(res.SessionID, inject.InjectRequest{
		SessionID: res.SessionID,
		CommandID: "inbox-rename-" + uuid.NewString()[:8],
		Text:      "/rename " + a.Title,
		Submit:    true,
	})
	time.Sleep(800 * time.Millisecond)
	return rt.hybrid.Inject(res.SessionID, inject.InjectRequest{
		SessionID: res.SessionID,
		CommandID: "inbox-body-" + uuid.NewString()[:8],
		Text:      a.Text,
		Submit:    true,
	})
}
