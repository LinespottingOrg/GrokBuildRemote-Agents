package session

import (
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GrokProc is one live `grok` / `grok --resume` process.
type GrokProc struct {
	PID      int    `json:"pid"`
	PPID     int    `json:"ppid"`
	ResumeID string `json:"resume_id,omitempty"`
	Cmd      string `json:"cmd"`
}

var resumeFlag = regexp.MustCompile(`(?i)--resume(?:\s+|=)([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9a-f]{16,})`)

var grokBinName = regexp.MustCompile(`(?i)(?:^|[\\/])grok(?:\.exe)?(?:\s|$)`)

var (
	procCacheMu sync.Mutex
	procCache   []GrokProc
	procCacheAt time.Time
)

// ParseGrokCommand reports whether a process command line is a Grok Build CLI
// (not gbr-agent, not "grep grok"). Resume UUID is extracted when present.
func ParseGrokCommand(cmd string) (isGrok bool, resume string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false, ""
	}
	low := strings.ToLower(cmd)
	if strings.Contains(low, "gbr-agent") || strings.Contains(low, "grep ") {
		return false, ""
	}
	if !grokBinName.MatchString(cmd) && !strings.Contains(low, "grok --") && !strings.Contains(low, "grok-build") {
		// "Something - grok — agent" is a window title, not a process argv.
		return false, ""
	}
	if m := resumeFlag.FindStringSubmatch(cmd); len(m) > 1 {
		return true, strings.ToLower(m[1])
	}
	return true, ""
}

// LooksLikeGrokWindow is the title/exe classifier used when the process tree
// is unavailable. "ProjectA - grok — agent · clang" counts.
func LooksLikeGrokWindow(title, exe, kind string) bool {
	for _, s := range []string{title, exe, kind} {
		if looksLikeGrok(s) {
			return true
		}
	}
	return false
}

// SuggestGrokSessionID picks a stable PreferID for a Grok window.
func SuggestGrokSessionID(resume string, hwnd uintptr, pid int) string {
	if r := slugResume(resume); r != "" {
		return r
	}
	if hwnd != 0 {
		id := Slugify("grok-build-" + strings.ToLower(strconv.FormatUint(uint64(hwnd), 16)))
		if ValidSessionID(id) {
			return id
		}
	}
	if pid > 0 {
		id := Slugify("grok-build-p" + strconv.Itoa(pid))
		if ValidSessionID(id) {
			return id
		}
	}
	return "grok-build"
}

func slugResume(resume string) string {
	resume = strings.ToLower(strings.TrimSpace(resume))
	resume = strings.ReplaceAll(resume, "-", "")
	if len(resume) < 8 {
		return ""
	}
	id := Slugify("grok-" + resume[:8])
	if ValidSessionID(id) {
		return id
	}
	return ""
}

// ScanGrokProcesses lists live grok CLIs (cached 2s).
func ScanGrokProcesses() []GrokProc {
	procCacheMu.Lock()
	defer procCacheMu.Unlock()
	if time.Since(procCacheAt) < 2*time.Second && procCache != nil {
		return procCache
	}
	procCache = scanGrokProcesses()
	procCacheAt = time.Now()
	return procCache
}

func scanGrokProcesses() []GrokProc {
	if runtime.GOOS == "windows" {
		return scanGrokWindows()
	}
	return scanGrokUnix()
}

func scanGrokUnix() []GrokProc {
	// pid ppid args — portable enough for macOS 13+ and Linux ps.
	out, err := exec.Command("ps", "-ax", "-o", "pid=,ppid=,command=").Output()
	if err != nil {
		return nil
	}
	return ParsePSTable(string(out))
}

func scanGrokWindows() []GrokProc {
	// Best-effort; title classifier still covers most Windows Grok hosts.
	out, err := exec.Command("wmic", "process", "get", "ProcessId,ParentProcessId,CommandLine", "/FORMAT:CSV").Output()
	if err != nil {
		return nil
	}
	return parseWMIC(string(out))
}

// ParsePSTable is exported for tests.
func ParsePSTable(raw string) []GrokProc {
	var out []GrokProc
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, rest, ok := cutInt(line)
		if !ok {
			continue
		}
		ppid, cmd, ok := cutInt(rest)
		if !ok {
			cmd = rest
		}
		is, resume := ParseGrokCommand(cmd)
		if !is {
			continue
		}
		out = append(out, GrokProc{PID: pid, PPID: ppid, ResumeID: resume, Cmd: strings.TrimSpace(cmd)})
	}
	return out
}

func parseWMIC(raw string) []GrokProc {
	var out []GrokProc
	for i, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" {
			continue
		}
		// Node,CommandLine,ParentProcessId,ProcessId
		parts := splitCSV(line)
		if len(parts) < 4 {
			continue
		}
		cmd := parts[len(parts)-3]
		ppid, _ := strconv.Atoi(parts[len(parts)-2])
		pid, _ := strconv.Atoi(parts[len(parts)-1])
		is, resume := ParseGrokCommand(cmd)
		if !is {
			continue
		}
		out = append(out, GrokProc{PID: pid, PPID: ppid, ResumeID: resume, Cmd: cmd})
	}
	return out
}

func splitCSV(s string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
		case r == ',' && !inQ:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	out = append(out, cur.String())
	return out
}

func cutInt(s string) (int, string, bool) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, s, false
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, s, false
	}
	return n, strings.TrimSpace(s[i:]), true
}

// MatchGrokProcess walks the parent chain of pid looking for a grok CLI.
func MatchGrokProcess(pid int, procs []GrokProc) (GrokProc, bool) {
	if pid <= 0 || len(procs) == 0 {
		return GrokProc{}, false
	}
	byPID := make(map[int]GrokProc, len(procs))
	childOf := make(map[int]int, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
		if p.PPID > 0 {
			childOf[p.PID] = p.PPID
		}
	}
	if p, ok := byPID[pid]; ok {
		return p, true
	}
	// pid may be the terminal; any grok whose ancestor is pid.
	for _, p := range procs {
		walk := p.PPID
		for i := 0; i < 12 && walk > 0; i++ {
			if walk == pid {
				return p, true
			}
			next, ok := childOf[walk]
			if !ok {
				// walk may not itself be a grok proc — stop
				break
			}
			walk = next
		}
	}
	return GrokProc{}, false
}

// ResetGrokProcCache is for tests.
func ResetGrokProcCache() {
	procCacheMu.Lock()
	procCache = nil
	procCacheAt = time.Time{}
	procCacheMu.Unlock()
}
