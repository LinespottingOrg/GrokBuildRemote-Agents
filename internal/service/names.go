package service

// User-visible product name (issue #55). CLI binary stays BinaryName; env stays GBR_*.
const (
	ProductName      = "Grok Build Remote"
	ProductNameAgent = "Grok Build Remote Agent"
	BinaryName       = "gbr-agent"

	// DarwinLaunchAgentLabel is the reverse-DNS launchd Label (not the Login Items title).
	DarwinLaunchAgentLabel = "com.linespotting.gbr-agent"
	// DarwinBundleID is CFBundleIdentifier for ~/Applications/Grok Build Remote.app.
	DarwinBundleID = "com.linespotting.grok-build-remote"

	// LinuxUnitName is the systemd unit filename. Description= is ProductNameAgent.
	LinuxUnitName    = "gbr-agent.service"
	LinuxDesktopFile = "gbr-agent.desktop"
	LinuxDescription = "Grok Build Remote agent (gbr-agent)"

	// WindowsTaskName is the Task Scheduler /TN users see (spaces are valid).
	WindowsTaskName = ProductName
	// WindowsLegacyTaskName is the pre-#55 interactive logon task.
	// Disable on install; do not delete without David-yes.
	WindowsLegacyTaskName = "GrokBuildRemoteAgent"
	// WindowsLegacyDocsTaskName is the sample -TaskName in older service.md.
	WindowsLegacyDocsTaskName = "GrokBuildRemote-Agent"
	// WindowsPR54TaskName is the #54 non-interactive / WinSW task id (not this PR).
	// Human label is still ProductName / ProductNameAgent, not the id.
	WindowsPR54TaskName = "GrokBuildRemoteAgentService"

	WindowsStartupCmd       = "Grok Build Remote.cmd"
	WindowsLegacyStartupCmd = "GrokBuildRemoteAgent.cmd"
)

// windowsLegacyTaskNames are old interactive Task Scheduler ids to disable
// (not delete) when installing WindowsTaskName.
func windowsLegacyTaskNames() []string {
	return []string{WindowsLegacyTaskName, WindowsLegacyDocsTaskName}
}
