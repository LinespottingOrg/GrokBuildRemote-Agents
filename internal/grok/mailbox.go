package grok

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ProtoV1 is the locked Day-1 protocol version string.
const ProtoV1 = "gbr/1"

// GBRTag prefixes mailbox chat messages so Mode B can filter envelopes.
const GBRTag = "[GBR]"

// Message types (protocol v1).
const (
	TypePair      = "pair"
	TypeRegister  = "register"
	TypeList      = "list"
	TypeInject    = "inject"
	TypeOutput    = "output"
	TypeHeartbeat = "heartbeat"
	TypeError     = "error"
)

// Envelope is the GBR protocol wire format (see protocol/v1.md).
type Envelope struct {
	Proto     string          `json:"proto"`
	Type      string          `json:"type"`
	DeviceID  string          `json:"device_id,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	CommandID string          `json:"command_id,omitempty"`
	TS        time.Time       `json:"ts"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// SessionIDPattern matches protocol session_id: ^[a-z0-9][a-z0-9-]{1,62}$
var SessionIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

// NewEnvelope builds a proto-valid envelope with UTC timestamp.
func NewEnvelope(msgType, deviceID, sessionID, commandID string, payload any) (*Envelope, error) {
	if msgType == "" {
		return nil, fmt.Errorf("envelope type required")
	}
	if sessionID != "" && !ValidSessionID(sessionID) {
		return nil, fmt.Errorf("invalid session_id %q", sessionID)
	}
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		raw = b
	} else {
		raw = json.RawMessage(`{}`)
	}
	return &Envelope{
		Proto:     ProtoV1,
		Type:      msgType,
		DeviceID:  deviceID,
		SessionID: sessionID,
		CommandID: commandID,
		TS:        time.Now().UTC(),
		Payload:   raw,
	}, nil
}

// ValidSessionID enforces protocol slug rules and rejects path traversal.
func ValidSessionID(id string) bool {
	if id == "" {
		return false
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return false
	}
	return SessionIDPattern.MatchString(id)
}

// Serialize returns compact JSON for the envelope body (no [GBR] tag).
func (e *Envelope) Serialize() ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("nil envelope")
	}
	if e.Proto == "" {
		e.Proto = ProtoV1
	}
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	return json.Marshal(e)
}

// SerializeTagged returns a chat message body: "[GBR] {json}".
func (e *Envelope) SerializeTagged() (string, error) {
	b, err := e.Serialize()
	if err != nil {
		return "", err
	}
	return GBRTag + " " + string(b), nil
}

// ParseEnvelope unmarshals a single envelope JSON object.
func ParseEnvelope(data []byte) (*Envelope, error) {
	data = bytesTrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("empty envelope")
	}
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parse envelope: %w", err)
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return &e, nil
}

// ParseTaggedMessage extracts and parses a [GBR] tagged chat line/body.
// Accepts full message content that may include prose; finds first [GBR] JSON object.
func ParseTaggedMessage(content string) (*Envelope, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("empty message")
	}

	// Fast path: exact "[GBR] {...}"
	if strings.HasPrefix(content, GBRTag) {
		rest := strings.TrimSpace(strings.TrimPrefix(content, GBRTag))
		return ParseEnvelope([]byte(rest))
	}

	// Scan for tag anywhere (model may wrap output).
	idx := strings.Index(content, GBRTag)
	if idx < 0 {
		return nil, fmt.Errorf("no %s tag in message", GBRTag)
	}
	rest := strings.TrimSpace(content[idx+len(GBRTag):])
	// Prefer first JSON object starting at '{'
	brace := strings.IndexByte(rest, '{')
	if brace < 0 {
		return nil, fmt.Errorf("%s tag present but no JSON object", GBRTag)
	}
	jsonPart, err := extractJSONObject(rest[brace:])
	if err != nil {
		return nil, err
	}
	return ParseEnvelope([]byte(jsonPart))
}

// ExtractTaggedEnvelopes finds all [GBR] envelopes in multi-message content.
func ExtractTaggedEnvelopes(content string) ([]*Envelope, error) {
	var out []*Envelope
	rest := content
	for {
		idx := strings.Index(rest, GBRTag)
		if idx < 0 {
			break
		}
		after := strings.TrimSpace(rest[idx+len(GBRTag):])
		brace := strings.IndexByte(after, '{')
		if brace < 0 {
			break
		}
		jsonPart, err := extractJSONObject(after[brace:])
		if err != nil {
			// Skip broken fragment; continue scan after tag.
			rest = after
			continue
		}
		env, err := ParseEnvelope([]byte(jsonPart))
		if err == nil {
			out = append(out, env)
		}
		// Advance past this object.
		rest = after[brace+len(jsonPart):]
	}
	return out, nil
}

// Validate checks proto version and required fields.
func (e *Envelope) Validate() error {
	if e == nil {
		return fmt.Errorf("nil envelope")
	}
	if e.Proto != ProtoV1 {
		return fmt.Errorf("unsupported proto %q (want %s)", e.Proto, ProtoV1)
	}
	switch e.Type {
	case TypePair, TypeRegister, TypeList, TypeInject, TypeOutput, TypeHeartbeat, TypeError:
		// ok
	case "":
		return fmt.Errorf("missing type")
	default:
		return fmt.Errorf("unknown type %q", e.Type)
	}
	if e.TS.IsZero() {
		return fmt.Errorf("missing ts")
	}
	if e.SessionID != "" && !ValidSessionID(e.SessionID) {
		return fmt.Errorf("invalid session_id %q", e.SessionID)
	}
	return nil
}

// UnmarshalPayload decodes payload into dest.
func (e *Envelope) UnmarshalPayload(dest any) error {
	if e == nil {
		return fmt.Errorf("nil envelope")
	}
	if len(e.Payload) == 0 {
		return json.Unmarshal([]byte(`{}`), dest)
	}
	return json.Unmarshal(e.Payload, dest)
}

// --- typed payload helpers ---

// PairPayload is type=pair.
type PairPayload struct {
	PairingCode string `json:"pairing_code"`
	DeviceName  string `json:"device_name,omitempty"`
}

// RegisterPayload is type=register.
type RegisterPayload struct {
	CWD       string `json:"cwd,omitempty"`
	Shell     string `json:"shell,omitempty"`
	OS        string `json:"os,omitempty"`
	Title     string `json:"title,omitempty"`
	GitRemote string `json:"git_remote,omitempty"`
}

// InjectPayload is type=inject.
type InjectPayload struct {
	Mode     string `json:"mode"` // text | nl
	Text     string `json:"text,omitempty"`
	NLPrompt string `json:"nl_prompt,omitempty"`
	Submit   bool   `json:"submit"`
}

// OutputPayload is type=output.
type OutputPayload struct {
	Stream string `json:"stream"` // stdout | stderr | system
	Chunk  string `json:"chunk"`
	EOF    bool   `json:"eof"`
}

// HeartbeatPayload is type=heartbeat.
type HeartbeatPayload struct {
	SessionCount int    `json:"session_count"`
	Status       string `json:"status,omitempty"`
}

// ErrorPayload is type=error.
type ErrorPayload struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// extractJSONObject returns the first complete JSON object starting at s[0]=='{'.
func extractJSONObject(s string) (string, error) {
	if s == "" || s[0] != '{' {
		return "", fmt.Errorf("expected JSON object")
	}
	depth := 0
	inString := false
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1], nil
			}
		}
	}
	return "", fmt.Errorf("unterminated JSON object")
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
