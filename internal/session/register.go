package session

import (
	"time"
)

// ProtoVersion is the locked protocol identifier.
const ProtoVersion = "gbr/1"

// RegisterType is the envelope type for session advertisement.
const RegisterType = "register"

// RegisterPayload matches protocol v1 register message payload.
//
//	{
//	  "cwd": "...",
//	  "shell": "pwsh",
//	  "os": "windows",
//	  "title": "Windows Terminal",
//	  "git_remote": "Org/Repo"
//	}
type RegisterPayload struct {
	CWD       string `json:"cwd"`
	Shell     string `json:"shell"`
	OS        string `json:"os"`
	Title     string `json:"title"`
	GitRemote string `json:"git_remote,omitempty"`
}

// RegisterMessage is a full protocol envelope of type "register".
type RegisterMessage struct {
	Proto     string          `json:"proto"`
	Type      string          `json:"type"`
	DeviceID  string          `json:"device_id,omitempty"`
	SessionID string          `json:"session_id"`
	TS        string          `json:"ts"`
	Payload   RegisterPayload `json:"payload"`
}

// RegisterPayload builds the protocol payload from a Session.
func (s *Session) RegisterPayload() RegisterPayload {
	if s == nil {
		return RegisterPayload{}
	}
	return RegisterPayload{
		CWD:       s.CWD,
		Shell:     s.Shell,
		OS:        s.OS,
		Title:     s.Title,
		GitRemote: s.GitRemote,
	}
}

// ToRegister builds a protocol register envelope for this session.
// deviceID may be empty (caller fills later). ts defaults to now UTC RFC3339.
func (s *Session) ToRegister(deviceID string) RegisterMessage {
	ts := time.Now().UTC().Format(time.RFC3339)
	if s == nil {
		return RegisterMessage{
			Proto: ProtoVersion,
			Type:  RegisterType,
			TS:    ts,
		}
	}
	return RegisterMessage{
		Proto:     ProtoVersion,
		Type:      RegisterType,
		DeviceID:  deviceID,
		SessionID: s.ID,
		TS:        ts,
		Payload:   s.RegisterPayload(),
	}
}

// BuildSession constructs a Session from a Candidate using naming resolution.
func BuildSession(c Candidate, renames map[string]string) (*Session, error) {
	cwd := NormalizeCWD(c.CWD)
	id, src, err := ResolveSessionID(cwd, renames)
	if err != nil {
		return nil, err
	}
	shell := c.Shell
	if shell == "" {
		shell = defaultShell()
	}
	title := c.Title
	if title == "" {
		title = shell
	}
	return &Session{
		ID:        id,
		CWD:       cwd,
		Shell:     shell,
		PID:       c.PID,
		HWND:      c.HWND,
		Title:     title,
		GitRemote: GitRemoteDisplay(cwd),
		LastSeen:  time.Now().UTC(),
		OS:        hostOS(),
		Source:    src,
	}, nil
}
