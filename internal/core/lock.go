package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// ErrAlreadyRunning is returned when another live agent holds the mailbox lock.
//
// One agent per MAILBOX is the correct topology: a single agent already serves
// MANY named sessions (the scanner tracks every terminal, the registry lists
// them, and each inject is routed by session_id). A second agent on the same
// mailbox is never useful — both poll the same queue, both inject, and both ack,
// so they consume and acknowledge each other's commands.
var ErrAlreadyRunning = errors.New("another gbr-agent is already running")

// LockInfo is what we persist about the holder.
type LockInfo struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Mailbox   string    `json:"mailbox,omitempty"`
	Host      string    `json:"host,omitempty"`
}

// Lock is an advisory single-instance lock backed by a PID file.
type Lock struct {
	path string
	info LockInfo
}

// LockPath is the lock file for one mailbox:
// %USERPROFILE%\.gbr\agent-<mailbox>.lock
//
// Scoped PER MAILBOX, not per machine. The defect being prevented is two agents
// polling the SAME mailbox — they consume and ack each other's commands. Two
// agents on DIFFERENT mailboxes is legitimate (the QA harness runs a throwaway
// agent on a fresh pairing code alongside the installed daemon), and a global
// lock wrongly blocked it — it broke three assertions in run-qa-advanced.
//
// Caveat: agents on different mailboxes still share the machine's terminals, so
// concurrent UI injects can interleave. That is a deliberate multi-account case,
// not the corruption case.
func LockPath(mailbox string) string {
	name := "agent.lock"
	if s := sanitizeMailbox(mailbox); s != "" {
		name = "agent-" + s + ".lock"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "gbr-"+name)
	}
	return filepath.Join(home, ".gbr", name)
}

func sanitizeMailbox(m string) string {
	m = strings.TrimSpace(strings.ToLower(m))
	if m == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range m {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	s := b.String()
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// ReadLock returns the current holder for a mailbox, if any.
func ReadLock(mailbox string) (*LockInfo, bool) {
	b, err := os.ReadFile(LockPath(mailbox))
	if err != nil {
		return nil, false
	}
	var li LockInfo
	if err := json.Unmarshal(b, &li); err != nil {
		return nil, false
	}
	return &li, true
}

// AcquireLock takes the single-instance lock.
//
// A stale lock (holder process no longer alive) is reclaimed automatically, so
// a hard kill or power loss never wedges the agent permanently.
func AcquireLock(mailbox string) (*Lock, error) {
	path := LockPath(mailbox)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	if li, ok := ReadLock(mailbox); ok {
		if li.PID != os.Getpid() && ProcessAlive(li.PID) {
			return nil, fmt.Errorf("%w on mailbox %s (pid %d, started %s)",
				ErrAlreadyRunning, mailbox, li.PID, li.StartedAt.Local().Format(time.RFC3339))
		}
		// Stale — previous holder is gone.
		_ = os.Remove(path)
	}

	host, _ := os.Hostname()
	l := &Lock{
		path: path,
		info: LockInfo{
			PID:       os.Getpid(),
			StartedAt: time.Now().UTC(),
			Mailbox:   mailbox,
			Host:      host,
		},
	}
	b, err := json.MarshalIndent(l.info, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return nil, err
	}
	return l, nil
}

// Release removes the lock file if we still own it.
func (l *Lock) Release() {
	if l == nil {
		return
	}
	if li, ok := ReadLock(l.info.Mailbox); ok && li.PID != l.info.PID {
		return // someone else owns it now; leave theirs alone
	}
	_ = os.Remove(l.path)
}

// Path returns the lock file location.
func (l *Lock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// ProcessAlive reports whether a PID belongs to a live process.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		// On Windows FindProcess opens a real handle, so success implies the
		// process exists. On Unix it always succeeds, hence the signal probe.
		return true
	}
	return p.Signal(syscall.Signal(0)) == nil
}
