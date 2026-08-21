package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/core"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/relay"
)

// cmdPairAsMailbox registers this machine as a fleet remote mailbox.
// No phone, no QR, no pairing code or mailbox key on stdout.
func cmdPairAsMailbox(args []string) int {
	fs := flag.NewFlagSet("pair-as-mailbox", flag.ExitOnError)
	name := fs.String("name", "mac", "fleet device slug the hub will use (default mac)")
	relayURL := fs.String("relay", "", "relay base URL (default production)")
	force := fs.Bool("force", false, "mint a new mailbox even if this machine is already paired")
	osName := fs.String("os", runtime.GOOS, "os label stored on the offer (darwin/linux/windows)")
	_ = fs.Parse(args)

	slug := coreNormalizeFleetName(*name)
	if slug == "" {
		slug = "mac"
	}

	dev, err := core.LoadOrCreateDevice()
	if err != nil {
		slog.Error("device", "err", err)
		return 1
	}
	if err := dev.SetDeviceName(slug); err != nil {
		slog.Error("rename", "err", err)
		return 1
	}

	rc := relay.New(firstNonEmpty(*relayURL, os.Getenv("GBR_RELAY_URL")), 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reused := false
	if !*force && strings.TrimSpace(dev.MailboxConversationID) != "" && strings.TrimSpace(dev.MailboxKey) != "" {
		if mailboxKeyRejected(ctx, rc, dev.MailboxConversationID, dev.MailboxKey) {
			slog.Warn("existing mailbox key rejected; minting a new mailbox")
		} else {
			reused = true
		}
	}

	if !reused {
		codeNorm, err := generatePairingCode(8)
		if err != nil {
			slog.Error("generate pairing code", "err", err)
			return 1
		}
		mailboxID := "gbr-" + strings.ToLower(codeNorm)
		if err := dev.SetMailboxConversationID(mailboxID); err != nil {
			slog.Error("save mailbox id", "err", err)
			return 1
		}
		mbKey, err := rc.Pair(ctx, mailboxID, codeNorm, dev.DeviceID, slug)
		if err != nil {
			slog.Error("relay pair", "err", err)
			return 1
		}
		if mbKey == "" {
			slog.Error("relay issued no mailbox key — upgrade agent and re-pair",
				"hint", "download latest from https://grokbuildremote.com/#download")
			return 1
		}
		if err := dev.SetMailboxKey(mbKey); err != nil {
			slog.Error("save mailbox key", "err", err)
			return 1
		}
		rc.SetKey(mbKey)
	}

	offerOS := strings.TrimSpace(*osName)
	if offerOS == "" {
		offerOS = runtime.GOOS
	}
	written, err := core.WriteFleetOffer(core.FleetOffer{
		Name:       slug,
		MailboxID:  dev.MailboxConversationID,
		MailboxKey: dev.MailboxKey,
		OS:         offerOS,
	})
	if err != nil {
		slog.Error("write fleet offer", "err", err)
		return 1
	}

	relayOK := "down"
	hctx, hcancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer hcancel()
	if err := rc.Health(hctx); err == nil {
		relayOK = "ok"
	}

	fmt.Print(formatPairAsMailboxReport(pairAsMailboxReport{
		Name:       slug,
		OS:         offerOS,
		Reused:     reused,
		KeyLen:     len(dev.MailboxKey),
		Relay:      rc.Base(),
		RelayOK:    relayOK,
		OfferFiles: len(written),
	}))
	return 0
}

func coreNormalizeFleetName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func mailboxKeyRejected(ctx context.Context, rc *relay.Client, mailboxID, key string) bool {
	_, code, err := rc.BotJSON(ctx, mailboxID, key, http.MethodGet, "/status", nil)
	if err != nil {
		return false
	}
	return code == http.StatusUnauthorized || code == http.StatusForbidden
}

type pairAsMailboxReport struct {
	Name       string
	OS         string
	Reused     bool
	KeyLen     int
	Relay      string
	RelayOK    string
	OfferFiles int
}

func formatPairAsMailboxReport(r pairAsMailboxReport) string {
	how := "new mailbox"
	if r.Reused {
		how = "existing mailbox"
	}
	return fmt.Sprintf(
		"paired as mailbox (%s)\n"+
			"name=%s os=%s\n"+
			"mailbox_key: set (%d chars) — not printed\n"+
			"relay=%s (%s)\n"+
			"offer files: %d (0600; hub reads these, never paste the key)\n"+
			"hub next (Windows, after halt-clear): gbr-agent fleet add -name %s -os %s\n",
		how, r.Name, r.OS, r.KeyLen, r.Relay, r.RelayOK, r.OfferFiles, r.Name, r.OS)
}
