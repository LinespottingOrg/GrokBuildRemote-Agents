package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode/utf16"
)

// Must stay in sync with install/darwin/Grok Build Remote.app/Contents/Info.plist
const darwinInfoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleDisplayName</key>
	<string>Grok Build Remote</string>
	<key>CFBundleExecutable</key>
	<string>Grok Build Remote</string>
	<key>CFBundleIdentifier</key>
	<string>com.linespotting.grok-build-remote</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>Grok Build Remote</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>0.6.3</string>
	<key>CFBundleVersion</key>
	<string>0.6.3</string>
	<key>LSMinimumSystemVersion</key>
	<string>12.0</string>
	<key>LSUIElement</key>
	<true/>
	<key>NSHumanReadableCopyright</key>
	<string>Copyright © Linespotting AB</string>
</dict>
</plist>
`

// Must stay in sync with install/linux/gbr-agent.desktop (Exec= rewritten at install).
const linuxDesktopSample = `[Desktop Entry]
Type=Application
Version=1.5
Name=Grok Build Remote
GenericName=Grok Build Remote Agent
Comment=Grok Build Remote desktop agent (gbr-agent) — pairs with the phone app
Exec=gbr-agent run
TryExec=gbr-agent
Terminal=false
Categories=Development;Network;Utility;
Keywords=gbr;gbr-agent;Grok;Build Remote;
StartupNotify=false
NoDisplay=false
X-GNOME-UsesNotifications=false
`

const darwinPlistTmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>{{.Label}}</string>
  <key>AssociatedBundleIdentifiers</key>
  <array>
    <string>{{.BundleID}}</string>
  </array>
  <key>ProgramArguments</key>
  <array>
    <string>{{.Binary}}</string>
    <string>-log=info</string>
    <string>run</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>WorkingDirectory</key>
  <string>{{.WorkDir}}</string>
  <key>StandardOutPath</key>
  <string>{{.DataDir}}/agent.out.log</string>
  <key>StandardErrorPath</key>
  <string>{{.DataDir}}/agent.err.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>{{.Home}}/.local/bin:{{.Home}}/bin:/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin</string>
    <key>HOME</key>
    <string>{{.Home}}</string>{{if .RelayURL}}
    <key>GBR_RELAY_URL</key>
    <string>{{.RelayURL}}</string>{{end}}
  </dict>
</dict>
</plist>
`

const linuxUnitTmpl = `[Unit]
Description={{.Description}}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={{.Binary}} -log=info run
WorkingDirectory={{.WorkDir}}
Restart=on-failure
RestartSec=3
Environment=PATH=/usr/local/bin:/usr/bin:/bin
{{if .RelayURL}}Environment=GBR_RELAY_URL={{.RelayURL}}
{{end}}# Hub PCs should set GBR_BOT_REQUIRE_KEY=1 (or true/on). Default off for MCP.
# Environment=GBR_BOT_REQUIRE_KEY=1
# Uncomment if DISPLAY needed for xdotool:
# Environment=DISPLAY=:0

[Install]
WantedBy=default.target
`

func renderDarwinPlist(binary, workDir, dataDir, home, relayURL string) (string, error) {
	t, err := template.New("plist").Parse(darwinPlistTmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, map[string]string{
		"Label":    DarwinLaunchAgentLabel,
		"BundleID": DarwinBundleID,
		"Binary":   binary,
		"WorkDir":  workDir,
		"DataDir":  dataDir,
		"Home":     home,
		"RelayURL": strings.TrimSpace(relayURL),
	}); err != nil {
		return "", err
	}
	return b.String(), nil
}

func renderLinuxUnit(binary, workDir, relayURL string) (string, error) {
	t, err := template.New("unit").Parse(linuxUnitTmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, map[string]string{
		"Description": LinuxDescription,
		"Binary":      binary,
		"WorkDir":     workDir,
		"RelayURL":    strings.TrimSpace(relayURL),
	}); err != nil {
		return "", err
	}
	return b.String(), nil
}

func linuxDesktopEntry(binary string) string {
	s := linuxDesktopSample
	execLine := "Exec=" + quoteDesktopExec(binary) + " -log=info run"
	s = strings.Replace(s, "Exec=gbr-agent run", execLine, 1)
	if filepath.IsAbs(binary) {
		s = strings.Replace(s, "TryExec=gbr-agent", "TryExec="+quoteDesktopExec(binary), 1)
	}
	return s
}

func quoteDesktopExec(p string) string {
	if strings.ContainsAny(p, " \t") {
		return `"` + p + `"`
	}
	return p
}

func darwinStubScript(binary string) string {
	return fmt.Sprintf(`#!/bin/sh
# Grok Build Remote — Login Item stub. Accessibility is granted to gbr-agent.
exec %s "$@"
`, shSingleQuote(binary))
}

func shSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func windowsTaskXML(taskName, binary, workDir, userId string) string {
	// S4U + Hidden + direct exec. InteractiveToken / powershell host is forbidden
	// (desktop popups). Flags are argv because S4U does not inherit User env.
	if userId == "" {
		userId = os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Author>Linespotting AB</Author>
    <Description>%s — NON-INTERACTIVE (S4U+Highest). InteractiveToken forbidden.</Description>
    <URI>\%s</URI>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%s</UserId>
      <LogonType>S4U</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>-log=info run -inject-halt -no-auto-open -no-inbox-watch</Arguments>
      <WorkingDirectory>%s</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`, ProductNameAgent, taskName, userId, binary, workDir)
}

func utf16LEBOM(s string) []byte {
	encoded := utf16.Encode([]rune(s))
	u := make([]byte, 2+len(encoded)*2)
	u[0], u[1] = 0xFF, 0xFE
	for i, r := range encoded {
		u[2+i*2] = byte(r)
		u[2+i*2+1] = byte(r >> 8)
	}
	return u
}

func darwinAppBundlePath(home string) string {
	return filepath.Join(home, "Applications", ProductName+".app")
}
