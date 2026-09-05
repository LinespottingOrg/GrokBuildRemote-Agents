package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestProductDisplayName(t *testing.T) {
	if ProductName != "Grok Build Remote" {
		t.Fatalf("ProductName = %q", ProductName)
	}
	if ProductNameAgent != "Grok Build Remote Agent" {
		t.Fatalf("ProductNameAgent = %q", ProductNameAgent)
	}
	if BinaryName != "gbr-agent" {
		t.Fatalf("CLI binary must stay gbr-agent, got %q", BinaryName)
	}
	if WindowsTaskName != ProductName {
		t.Fatalf("Task Scheduler /TN must be the human name, got %q", WindowsTaskName)
	}
	if WindowsTaskName == WindowsLegacyTaskName || WindowsTaskName == "gbr" || WindowsTaskName == BinaryName {
		t.Fatalf("task name must not be the internal id or bare gbr, got %q", WindowsTaskName)
	}
	if DarwinLaunchAgentLabel != "com.linespotting.gbr-agent" {
		t.Fatalf("launchd Label stays reverse-DNS, got %q", DarwinLaunchAgentLabel)
	}
	if DarwinBundleID != "com.linespotting.grok-build-remote" {
		t.Fatalf("bundle id = %q", DarwinBundleID)
	}
	if LinuxUnitName != "gbr-agent.service" {
		t.Fatalf("unit filename stays gbr-agent.service, got %q", LinuxUnitName)
	}
	if !strings.HasPrefix(LinuxDescription, ProductName) {
		t.Fatalf("systemd Description must start with product name: %q", LinuxDescription)
	}
}

func TestWindowsTaskXMLShowsProductName(t *testing.T) {
	xml := windowsTaskXML(WindowsTaskName, `C:\Users\me\gbr-agent.exe`, `C:\Users\me`)
	for _, want := range []string{
		`<URI>\Grok Build Remote</URI>`,
		`<Description>Grok Build Remote Agent`,
		`<Hidden>false</Hidden>`,
		`<Author>Linespotting AB</Author>`,
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("task XML missing %q", want)
		}
	}
	if strings.Contains(xml, `<URI>\GrokBuildRemoteAgent</URI>`) {
		t.Error("task XML still uses legacy GrokBuildRemoteAgent as URI")
	}
}

func TestDarwinPlistShowsLoginItemName(t *testing.T) {
	body, err := renderDarwinPlist("/Users/me/.local/bin/gbr-agent", "/Users/me", "/Users/me/.gbr", "/Users/me", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "<string>"+DarwinLaunchAgentLabel+"</string>") {
		t.Error("plist must keep reverse-DNS Label")
	}
	if !strings.Contains(body, "<key>AssociatedBundleIdentifiers</key>") {
		t.Error("plist missing AssociatedBundleIdentifiers (Login Items title)")
	}
	if !strings.Contains(body, "<string>"+DarwinBundleID+"</string>") {
		t.Error("plist missing bundle id")
	}
	if !strings.Contains(body, "/Users/me/.local/bin/gbr-agent") {
		t.Error("plist must still launch the gbr-agent binary")
	}
}

func TestLinuxUnitDescription(t *testing.T) {
	body, err := renderLinuxUnit("/home/me/.local/bin/gbr-agent", "/home/me", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Description="+LinuxDescription) {
		t.Fatalf("unit missing Description: %s", body)
	}
	if !strings.Contains(body, ProductName) {
		t.Error("unit Description must include Grok Build Remote")
	}
}

func TestLinuxDesktopName(t *testing.T) {
	raw := linuxDesktopSample
	if !strings.Contains(raw, "Name=Grok Build Remote\n") {
		t.Fatalf("desktop file Name= is not %q:\n%s", ProductName, raw)
	}
	if !strings.Contains(raw, "Comment=") || !strings.Contains(raw, ProductName) {
		t.Error("desktop Comment must mention the product")
	}
	got := linuxDesktopEntry(`/home/me/.local/bin/gbr-agent`)
	if !strings.Contains(got, "Exec=/home/me/.local/bin/gbr-agent -log=info run") {
		t.Fatalf("custom Exec missing:\n%s", got)
	}
	if !strings.Contains(got, "Name=Grok Build Remote\n") {
		t.Error("generated desktop lost Name=")
	}
}

func TestDarwinInfoPlistDisplayName(t *testing.T) {
	s := darwinInfoPlist
	for _, want := range []string{
		"<string>Grok Build Remote</string>",
		"<key>CFBundleDisplayName</key>",
		"<key>CFBundleName</key>",
		"<string>" + DarwinBundleID + "</string>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("Info.plist missing %q", want)
		}
	}
}

func TestDarwinStubExecsBinary(t *testing.T) {
	stub := darwinStubScript(`/Users/me/.local/bin/gbr-agent`)
	if !strings.Contains(stub, "exec '/Users/me/.local/bin/gbr-agent'") {
		t.Fatalf("stub must exec gbr-agent:\n%s", stub)
	}
}

func TestUtf16LEBOMRoundtrip(t *testing.T) {
	s := ProductNameAgent + " — polls"
	got := utf16LEBOM(s)
	if got[0] != 0xFF || got[1] != 0xFE {
		t.Fatal("missing BOM")
	}
	u16 := make([]uint16, (len(got)-2)/2)
	for i := range u16 {
		u16[i] = uint16(got[2+i*2]) | uint16(got[3+i*2])<<8
	}
	if string(utf16.Decode(u16)) != s {
		t.Fatalf("roundtrip: got %q want %q", string(utf16.Decode(u16)), s)
	}
}

func TestInstallArtifactsShowProductName(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatal(err)
	}
	checks := map[string][]string{
		filepath.Join("install", "windows", "gbr-agent.xml"): {
			"<name>Grok Build Remote Agent</name>",
		},
		filepath.Join("install", "windows", "service.md"): {
			"Grok Build Remote",
			"GrokBuildRemoteAgentService",
		},
		filepath.Join("install", "linux", "gbr-agent.service"): {
			"Description=Grok Build Remote agent (gbr-agent)",
		},
		filepath.Join("install", "linux", "gbr-agent.desktop"): {
			"Name=Grok Build Remote",
		},
		filepath.Join("install", "darwin", "launchd.plist.example"): {
			"AssociatedBundleIdentifiers",
			"Grok Build Remote",
			DarwinLaunchAgentLabel,
		},
		filepath.Join("install", "darwin", "Grok Build Remote.app", "Contents", "Info.plist"): {
			"CFBundleDisplayName",
			"Grok Build Remote",
			DarwinBundleID,
		},
		filepath.Join("install", "darwin", "Grok Build Remote.app", "Contents", "MacOS", "Grok Build Remote"): {
			"exec",
			"gbr-agent",
		},
	}
	for rel, wants := range checks {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		// Normalize CRLF so Windows checkouts still match.
		text := string(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")))
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", rel, want)
			}
		}
	}
}

func TestInstallSamplesMatchConstants(t *testing.T) {
	root := filepath.Join("..", "..")
	pairs := map[string]string{
		filepath.Join("install", "linux", "gbr-agent.desktop"):                                linuxDesktopSample,
		filepath.Join("install", "darwin", "Grok Build Remote.app", "Contents", "Info.plist"): darwinInfoPlist,
	}
	for rel, want := range pairs {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		got := strings.ReplaceAll(string(b), "\r\n", "\n")
		want = strings.ReplaceAll(want, "\r\n", "\n")
		if got != want {
			t.Errorf("%s does not match Go constant (len got=%d want=%d)", rel, len(got), len(want))
		}
	}
}

func TestWindowsLegacyTasksAreNotCurrentName(t *testing.T) {
	for _, name := range windowsLegacyTaskNames() {
		if name == WindowsTaskName {
			t.Fatalf("legacy id %q equals current human task name", name)
		}
	}
	if WindowsPR54TaskName == WindowsTaskName {
		t.Fatal("PR #54 task id must stay distinct from the human Task Scheduler name")
	}
}
