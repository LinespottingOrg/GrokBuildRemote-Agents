package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScannerScanOnce_DiscoverAndRegister(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	proj := filepath.Join(dir, "cool-app")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, GrokSessionFile), []byte("cool-app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	storePath := filepath.Join(dir, "sessions.json")
	st, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	sc := NewScanner(st, reg, func(ctx context.Context) ([]Candidate, error) {
		return []Candidate{{
			CWD:   proj,
			Shell: "pwsh",
			PID:   1234,
			Title: "Windows Terminal",
		}}, nil
	})
	sc.StaleAfter = time.Hour // don't stale in unit test

	res, err := sc.ScanOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 1 {
		t.Fatalf("added=%d all=%d", len(res.Added), len(res.All))
	}
	if res.Added[0].ID != "cool-app" {
		t.Fatalf("id=%s", res.Added[0].ID)
	}

	msgs := sc.Registers("dev-1")
	if len(msgs) != 1 || msgs[0].SessionID != "cool-app" {
		t.Fatalf("msgs=%+v", msgs)
	}
	if msgs[0].Payload.CWD == "" || msgs[0].Payload.Shell != "pwsh" {
		t.Fatalf("payload=%+v", msgs[0].Payload)
	}
}

func TestScannerRename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	sc := NewScanner(st, NewRegistry(), nil)
	sc.StaleAfter = time.Hour
	sc.Track(Candidate{CWD: proj, Shell: "pwsh"})
	if _, err := sc.ScanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	sess, err := sc.Rename(proj, "renamed-session")
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID != "renamed-session" {
		t.Fatalf("id=%s", sess.ID)
	}
	// resolve priority: rename without .grok-session
	id, src, err := ResolveSessionID(proj, st.Snapshot())
	if err != nil || id != "renamed-session" || src != SourceRename {
		t.Fatalf("id=%s src=%s err=%v", id, src, err)
	}
}

func TestScannerRunContextCancel(t *testing.T) {
	t.Parallel()
	sc := NewScanner(nil, NewRegistry(), func(ctx context.Context) ([]Candidate, error) {
		return nil, nil
	})
	sc.Interval = 20 * time.Millisecond
	sc.StaleAfter = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := sc.Run(ctx)
	if err != context.DeadlineExceeded && err != context.Canceled {
		// Run returns ctx.Err()
		if err == nil {
			t.Fatal("expected ctx error")
		}
	}
}

func TestScannerTrackManual(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sc := NewScanner(nil, NewRegistry(), nil)
	sc.StaleAfter = time.Hour
	sc.Track(Candidate{CWD: dir, Shell: "bash", PID: 1})
	res, err := sc.ScanOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.All) != 1 {
		t.Fatalf("all=%d", len(res.All))
	}
	if res.All[0].Shell != "bash" {
		t.Fatalf("shell=%s", res.All[0].Shell)
	}
}

func TestScannerIDChangeOnGrokSession(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	proj := filepath.Join(dir, "x")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	sc := NewScanner(nil, NewRegistry(), func(ctx context.Context) ([]Candidate, error) {
		return []Candidate{{CWD: proj, Shell: "pwsh"}}, nil
	})
	sc.StaleAfter = time.Hour
	if _, err := sc.ScanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	oldID := sc.Registry.List()[0].ID
	if err := os.WriteFile(filepath.Join(proj, GrokSessionFile), []byte("brand-new-id\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := sc.ScanOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	list := sc.Registry.List()
	if len(list) != 1 || list[0].ID != "brand-new-id" {
		t.Fatalf("list=%+v old=%s res=%+v", list, oldID, res)
	}
}
