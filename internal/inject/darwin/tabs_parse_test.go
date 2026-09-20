package darwin

import "testing"

func TestParseTabLinesRealTabs(t *testing.T) {
	raw := "1\t1\tMac Mini GBR agent cleanup - grok\t/dev/ttys002\n2\t1\tQA SIM - grok\t/dev/ttys003"
	got := parseTabLines(raw, "Terminal")
	if len(got) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(got), got)
	}
	if got[0].WindowID != "1" || got[0].Index != 1 {
		t.Fatalf("row0 ids: %+v", got[0])
	}
	if got[0].Title != "Mac Mini GBR agent cleanup - grok" {
		t.Fatalf("row0 title=%q", got[0].Title)
	}
	if got[0].TTY != "/dev/ttys002" {
		t.Fatalf("row0 tty=%q", got[0].TTY)
	}
	win := tabToWindow(got[0])
	if win.HWND == 0 {
		t.Fatal("hwnd collapsed to 0")
	}
	if win.ExeName != "Terminal" {
		t.Fatalf("exe=%q", win.ExeName)
	}
}

func TestParseTabLinesLiteralWordTabDoesNotSplit(t *testing.T) {
	// AppleScript `tab` is class text ("tab") on some Macs.
	raw := "1tab1tabGrok Buildtab/dev/ttys002"
	got := parseTabLines(raw, "Terminal")
	if len(got) != 1 {
		t.Fatalf("len=%d: %+v", len(got), got)
	}
	if got[0].Title != "" {
		t.Fatalf("want empty title when delimiter is the word tab, got %+v", got[0])
	}
	if tabToWindow(got[0]).HWND != 0 {
		t.Fatalf("want hwnd 0 for unparsed row, got %+v", tabToWindow(got[0]))
	}
}
