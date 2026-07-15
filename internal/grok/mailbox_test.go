package grok

import (
	"strings"
	"testing"
	"time"
)

func TestSerializeTaggedAndParse(t *testing.T) {
	env, err := NewEnvelope(TypeHeartbeat, "11111111-1111-1111-1111-111111111111", "", "22222222-2222-2222-2222-222222222222", HeartbeatPayload{
		SessionCount: 1,
		Status:       "alive",
	})
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := env.SerializeTagged()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tagged, GBRTag+" ") {
		t.Fatalf("missing tag: %s", tagged)
	}
	got, err := ParseTaggedMessage(tagged)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != TypeHeartbeat || got.Proto != ProtoV1 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	var p HeartbeatPayload
	if err := got.UnmarshalPayload(&p); err != nil {
		t.Fatal(err)
	}
	if p.SessionCount != 1 || p.Status != "alive" {
		t.Fatalf("payload: %+v", p)
	}
}

func TestParseTaggedEmbedded(t *testing.T) {
	raw := `Sure, here you go:
[GBR] {"proto":"gbr/1","type":"pair","device_id":"11111111-1111-1111-1111-111111111111","ts":"2026-07-15T17:00:00Z","payload":{"pairing_code":"A1B2C3D4"}}
thanks`
	got, err := ParseTaggedMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != TypePair {
		t.Fatalf("type=%s", got.Type)
	}
}

func TestValidSessionID(t *testing.T) {
	ok := []string{"global-edition", "a1", "x"}
	// "x" is only 1 char after first — pattern is [a-z0-9][a-z0-9-]{1,62} so min length 2
	ok = []string{"global-edition", "a1", "ab"}
	for _, s := range ok {
		if !ValidSessionID(s) {
			t.Fatalf("expected valid: %s", s)
		}
	}
	bad := []string{"", "A-B", "../x", "a/b", "UPPER", "x"}
	for _, s := range bad {
		if ValidSessionID(s) {
			t.Fatalf("expected invalid: %s", s)
		}
	}
}

func TestRejectBadProto(t *testing.T) {
	_, err := ParseEnvelope([]byte(`{"proto":"gbr/2","type":"list","ts":"2026-07-15T17:00:00Z"}`))
	if err == nil {
		t.Fatal("expected proto mismatch")
	}
}

func TestExtractMultiple(t *testing.T) {
	content := `[GBR] {"proto":"gbr/1","type":"heartbeat","ts":"2026-07-15T17:00:00Z","payload":{}}
[GBR] {"proto":"gbr/1","type":"list","ts":"2026-07-15T17:00:01Z","payload":{}}`
	envs, err := ExtractTaggedEnvelopes(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 2 {
		t.Fatalf("got %d", len(envs))
	}
}

func TestTSRoundTrip(t *testing.T) {
	ts := time.Date(2026, 7, 15, 17, 0, 0, 0, time.UTC)
	env := &Envelope{Proto: ProtoV1, Type: TypeList, TS: ts, Payload: []byte(`{}`)}
	b, err := env.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseEnvelope(b)
	if err != nil {
		t.Fatal(err)
	}
	if !got.TS.Equal(ts) {
		t.Fatalf("ts %v != %v", got.TS, ts)
	}
}
