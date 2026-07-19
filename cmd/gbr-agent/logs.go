package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/core"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/relay"
	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/trace"
)

// cmdLogs prints correlated hop events, either from the local JSONL file
// (agent hops only) or from the relay ring buffer (-remote: all three hops,
// phone + relay + agent, in one timeline).
func cmdLogs(args []string) int {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	follow := fs.Bool("f", false, "follow (tail -f)")
	n := fs.Int("n", 50, "show last N events")
	commandID := fs.String("command", "", "filter to one command_id")
	remote := fs.Bool("remote", false, "read the relay trace (all hops) instead of local file")
	raw := fs.Bool("json", false, "print raw JSONL")
	_ = fs.Parse(args)

	if *remote {
		return logsRemote(*n, *commandID, *raw, *follow)
	}
	return logsLocal(*n, *commandID, *raw, *follow)
}

func logPath() string {
	return trace.New(trace.Config{Actor: "agent"}).Path()
}

func logsLocal(n int, commandID string, raw, follow bool) int {
	path := logPath()
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no trace log yet at %s\n", path)
		fmt.Fprintf(os.Stderr, "hint: start the agent with  gbr-agent run\n")
		return 1
	}
	defer f.Close()

	fmt.Fprintf(os.Stderr, "== %s ==\n", path)
	var events []trace.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev trace.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if commandID != "" && ev.CommandID != commandID {
			continue
		}
		events = append(events, ev)
	}
	if len(events) > n {
		events = events[len(events)-n:]
	}
	printEvents(events, raw)

	if !follow {
		return 0
	}
	return followFile(f, commandID, raw)
}

// followFile tails an already-opened, already-read file handle.
func followFile(f *os.File, commandID string, raw bool) int {
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return 1
	}
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if err != nil {
			return 1
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev trace.Event
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if commandID != "" && ev.CommandID != commandID {
			continue
		}
		printEvents([]trace.Event{ev}, raw)
	}
}

func logsRemote(n int, commandID string, raw, follow bool) int {
	dev, err := core.LoadOrCreateDevice()
	if err != nil {
		fmt.Fprintf(os.Stderr, "device: %v\n", err)
		return 1
	}
	if dev.MailboxConversationID == "" {
		fmt.Fprintln(os.Stderr, "not paired — run: gbr-agent pair -code YOURCODE")
		return 1
	}
	base := relay.New(os.Getenv("GBR_RELAY_URL"), 20*time.Second).Base()
	fmt.Fprintf(os.Stderr, "== relay trace %s mailbox=%s ==\n", base, dev.MailboxConversationID)

	after := ""
	first := true
	for {
		events, now, err := fetchRemoteTrace(base, dev.MailboxConversationID, commandID, after, n)
		if err != nil {
			fmt.Fprintf(os.Stderr, "trace fetch: %v\n", err)
			if !follow {
				return 1
			}
		} else {
			if first && len(events) > n {
				events = events[len(events)-n:]
			}
			printEvents(events, raw)
			after = now
			first = false
		}
		if !follow {
			return 0
		}
		time.Sleep(2 * time.Second)
	}
}

func fetchRemoteTrace(base, mailbox, commandID, after string, limit int) ([]trace.Event, string, error) {
	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", limit))
	if commandID != "" {
		q.Set("command_id", commandID)
	}
	if after != "" {
		q.Set("after", after)
	}
	u := fmt.Sprintf("%s/v1/mb/%s/trace?%s", strings.TrimRight(base, "/"), url.PathEscape(mailbox), q.Encode())
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(u)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var body struct {
		Events []trace.Event `json:"events"`
		Now    string        `json:"now"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, "", err
	}
	sort.SliceStable(body.Events, func(i, j int) bool { return body.Events[i].TS < body.Events[j].TS })
	return body.Events, body.Now, nil
}

func printEvents(events []trace.Event, raw bool) {
	for _, ev := range events {
		if raw {
			b, _ := json.Marshal(ev)
			fmt.Println(string(b))
			continue
		}
		fmt.Println(formatEvent(ev))
	}
}

// formatEvent renders one hop as a fixed-width line:
//
//	20:14:07.412  agent   agent.inject      global-edition  4f2a1c9b   18ms  chars=24 submit=true
func formatEvent(ev trace.Event) string {
	ts := ev.TS
	if t, err := time.Parse(time.RFC3339Nano, ev.TS); err == nil {
		ts = t.Local().Format("15:04:05.000")
	}
	cmd := ev.CommandID
	if len(cmd) > 8 {
		cmd = cmd[:8]
	}
	if cmd == "" {
		cmd = "-"
	}
	sess := ev.SessionID
	if sess == "" {
		sess = "-"
	}
	ms := ""
	if ev.MS > 0 {
		ms = fmt.Sprintf("%dms", ev.MS)
	}
	status := " "
	if !ev.OK {
		status = "!"
	}
	return fmt.Sprintf("%s %s %-7s %-20s %-18s %-9s %6s  %s",
		ts, status, ev.Actor, ev.Hop, truncStr(sess, 18), cmd, ms, ev.Detail)
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
