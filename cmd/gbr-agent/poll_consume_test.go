package main

import (
	"errors"
	"testing"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/grok"
)

func TestConsumeEnvelope_InjectAcksOnHandleError(t *testing.T) {
	seen, ack := consumeEnvelope(grok.TypeInject, errors.New("capture push failed"))
	if !seen || !ack {
		t.Fatalf("failed inject must be marked seen and acked (seen=%v ack=%v) — otherwise PollOverlap re-types it", seen, ack)
	}
}

func TestConsumeEnvelope_ListAcksOnHandleError(t *testing.T) {
	seen, ack := consumeEnvelope(grok.TypeList, errors.New("push failed"))
	if !seen || !ack {
		t.Fatalf("list must still consume on error: seen=%v ack=%v", seen, ack)
	}
}

func TestConsumeEnvelope_OtherTypesNeedSuccess(t *testing.T) {
	seen, ack := consumeEnvelope(grok.TypeControl, errors.New("nope"))
	if seen || ack {
		t.Fatalf("control must not consume on error: seen=%v ack=%v", seen, ack)
	}
	seen, ack = consumeEnvelope(grok.TypeControl, nil)
	if !seen || ack {
		t.Fatalf("successful control is seen but not acked: seen=%v ack=%v", seen, ack)
	}
}
