package session

import (
	"context"
	"testing"
)

func TestBuildSession_PreferIDUniquePerHWND(t *testing.T) {
	a, err := BuildSession(Candidate{
		CWD:      "gbr-ui-a",
		Shell:    "grok-build",
		HWND:     0x1111,
		Title:    "Grok Build",
		PreferID: "grok-build-1111",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildSession(Candidate{
		CWD:      "gbr-ui-b",
		Shell:    "powershell",
		HWND:     0x2222,
		Title:    "PowerShell",
		PreferID: "powershell-2222",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatalf("expected unique session ids, both %q", a.ID)
	}
	if a.Source != SourceUI {
		t.Fatalf("source=%s", a.Source)
	}
	if !ValidSessionID(a.ID) || !ValidSessionID(b.ID) {
		t.Fatalf("invalid ids %q %q", a.ID, b.ID)
	}
}

func TestScanner_DoesNotCollapseUIWindows(t *testing.T) {
	sc := NewScanner(nil, NewRegistry(), func(ctx context.Context) ([]Candidate, error) {
		return []Candidate{
			{CWD: "gbr-ui-1", PreferID: "grok-build-aaa", Shell: "grok-build", HWND: 1, Title: "Grok Build"},
			{CWD: "gbr-ui-2", PreferID: "powershell-bbb", Shell: "powershell", HWND: 2, Title: "pwsh"},
		}, nil
	})
	res, err := sc.ScanOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.All) < 2 {
		t.Fatalf("want ≥2 sessions (Grok + PowerShell), got %d: %+v", len(res.All), res.All)
	}
	ids := map[string]bool{}
	for _, s := range res.All {
		ids[s.ID] = true
	}
	if len(ids) < 2 {
		t.Fatalf("session ids collapsed: %v", ids)
	}
}
