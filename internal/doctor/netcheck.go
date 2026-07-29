package doctor

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/relay"
)

// NetworkDoc is the one-line product rule for firewalls / VPNs.
const NetworkDoc = `
GBR network model (firewalls / VPN / corp proxy)
================================================
Phone and PC NEVER open ports to each other.
Both are OUTBOUND HTTPS clients only.

Required outbound (agent + mobile):
  • TCP 443 / HTTPS to the relay host (default Cloudflare Workers)
  • DNS resolution for that host
  • TLS 1.2+ (SNI + valid cert)

Default relay:
  https://gbr-relay.ekobrott.workers.dev
  paths: /health  /v1/mb/{id}/pair|push|poll|ack|trace

Optional outbound:
  • https://grokbuildremote.com  (install page, pair.html QR browser page)
  • GitHub releases / CDN if you download binaries

NOT required:
  • No inbound ports on phone or PC
  • No LAN discovery, mDNS, or UDP hole-punch
  • No VPN between phone and PC
  • No port forward / DMZ / UPnP

Corporate / school networks:
  Allowlist HTTPS to *.workers.dev (or your GBR_RELAY_URL host) and optionally
  grokbuildremote.com. SSL inspection must not break certificate validation.
  If a VPN intercepts all traffic, exclude the relay host or allow HTTPS 443.

Test on PC:
  gbr-agent netcheck
  gbr-agent doctor
`

// RunNetwork executes DNS → TCP:443 → TLS → HTTP health (and optional site).
// Use for firewall/VPN diagnosis. Does not require pairing.
func RunNetwork(relayBase string) []Result {
	if strings.TrimSpace(relayBase) == "" {
		relayBase = os.Getenv("GBR_RELAY_URL")
	}
	c := relay.New(relayBase, 12*time.Second)
	base := c.Base()

	var out []Result
	out = append(out, Result{
		Name:   "model",
		OK:     true,
		Detail: "outbound HTTPS only · no inbound ports · no phone↔PC sockets",
		FixHint: "see gbr-agent netcheck -doc  or NETWORK.md",
	})
	out = append(out, Result{
		Name:   "relay.url",
		OK:     true,
		Detail: base,
	})

	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return append(out, Result{
			Name: "relay.parse", OK: false, Detail: fmt.Sprintf("%v", err),
			FixHint: "set GBR_RELAY_URL to https://host with no path",
		})
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}

	// DNS
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		out = append(out, Result{
			Name: "dns", OK: false, Detail: err.Error(),
			FixHint: "VPN/DNS filter may block " + host + " — try public DNS or disable split-DNS blackhole",
		})
		// still try other checks; may fail
	} else {
		var parts []string
		for i, ip := range ips {
			if i >= 4 {
				parts = append(parts, "…")
				break
			}
			parts = append(parts, ip.IP.String())
		}
		out = append(out, Result{
			Name: "dns", OK: true, Detail: fmt.Sprintf("%s → %s", host, strings.Join(parts, ", ")),
		})
	}

	// TCP connect
	addr := net.JoinHostPort(host, port)
	d := net.Dialer{Timeout: 8 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		out = append(out, Result{
			Name: "tcp", OK: false, Detail: fmt.Sprintf("%s: %v", addr, err),
			FixHint: "firewall/VPN must allow OUTBOUND TCP " + port + " to " + host + " (no inbound rules needed)",
		})
		// cannot TLS/HTTP without TCP
		out = append(out, Result{
			Name: "tls", OK: false, Detail: "skipped (no TCP)",
			FixHint: "open outbound 443 first",
		})
		out = append(out, Result{
			Name: "https.health", OK: false, Detail: "skipped (no TCP)",
			FixHint: "open outbound 443 first",
		})
	} else {
		_ = conn.Close()
		out = append(out, Result{
			Name: "tcp", OK: true, Detail: fmt.Sprintf("connected %s (outbound only)", addr),
		})

		// TLS
		tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		raw, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			out = append(out, Result{Name: "tls", OK: false, Detail: err.Error(), FixHint: "retry; intermittent block"})
		} else {
			tconn := tls.Client(raw, tlsCfg)
			_ = tconn.SetDeadline(time.Now().Add(8 * time.Second))
			if err := tconn.Handshake(); err != nil {
				out = append(out, Result{
					Name: "tls", OK: false, Detail: err.Error(),
					FixHint: "SSL inspection / MITM proxy may break TLS — allowlist " + host + " or install corp root CA for the agent process",
				})
				_ = raw.Close()
			} else {
				state := tconn.ConnectionState()
				out = append(out, Result{
					Name: "tls", OK: true,
					Detail: fmt.Sprintf("TLS ok version=0x%x server=%s", state.Version, host),
				})
				_ = tconn.Close()
			}
		}

		// HTTP health
		hctx, hcancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer hcancel()
		if err := c.Health(hctx); err != nil {
			out = append(out, Result{
				Name: "https.health", OK: false, Detail: err.Error(),
				FixHint: "block is at HTTP layer (proxy auth, category filter). Allow HTTPS GET " + base + "/health",
			})
		} else {
			out = append(out, Result{
				Name: "https.health", OK: true, Detail: base + "/health → 200",
			})
		}
	}

	// Optional: marketing site (pair page / install) — non-fatal
	out = append(out, checkOptionalHTTPS("site.pair", "https://grokbuildremote.com/pair.html"))

	// Summarize ports policy
	out = append(out, Result{
		Name: "ports",
		OK:   true,
		Detail: "inbound: none · outbound required: TCP/" + port + " HTTPS to " + host,
		FixHint: "do NOT open ports on the PC; only allow outbound 443",
	})

	return out
}

func checkOptionalHTTPS(name, rawURL string) Result {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{Name: name, OK: true, Detail: "skip: " + err.Error(), FixHint: "optional"}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{
			Name: name, OK: true, // optional
			Detail:  "unreachable (optional): " + err.Error(),
			FixHint: "pair.html / install may fail in browser; relay can still work if https.health PASS",
		}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return Result{Name: name, OK: true, Detail: fmt.Sprintf("%s → %d", rawURL, resp.StatusCode)}
	}
	return Result{
		Name: name, OK: true,
		Detail:  fmt.Sprintf("%s → HTTP %d (optional)", rawURL, resp.StatusCode),
		FixHint: "optional site check",
	}
}

// FormatNetwork is like Format but titled netcheck.
func FormatNetwork(results []Result) string {
	var b strings.Builder
	b.WriteString("gbr-agent netcheck — firewall / VPN / outbound HTTPS\n")
	allOK := true
	for _, r := range results {
		// optional site.* never fail overall
		fatal := !r.OK && !strings.HasPrefix(r.Name, "site.")
		mark := "PASS"
		if !r.OK {
			if fatal {
				mark = "FAIL"
				allOK = false
			} else {
				mark = "WARN"
			}
		}
		b.WriteString(fmt.Sprintf("  %s  %-16s %s\n", mark, r.Name, r.Detail))
		if r.FixHint != "" && !r.OK {
			b.WriteString(fmt.Sprintf("         fix: %s\n", r.FixHint))
		} else if r.FixHint != "" && r.Name == "model" {
			b.WriteString(fmt.Sprintf("         note: %s\n", r.FixHint))
		}
	}
	if allOK {
		b.WriteString("overall: OK — outbound HTTPS to relay works (VPN/firewall OK for GBR)\n")
	} else {
		b.WriteString("overall: BLOCKED — fix FAIL lines; typically outbound TCP 443 or TLS inspection\n")
	}
	return b.String()
}
