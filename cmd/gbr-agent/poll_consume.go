package main

import "github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/grok"

// consumeEnvelope says whether a processed mailbox envelope should be marked
// seen and acked.
//
// Inject and list are consumed even when handle failed. Relay PollOverlap is
// 30s (internal/relay/client.go) and pollOnce used to `continue` without
// seen/ack on error — so a capture/push failure re-delivered the same
// inject every 2–5s and Hybrid typed the keys again (Grok approval-card loop).
func consumeEnvelope(typ string, handleErr error) (markSeen, ack bool) {
	switch typ {
	case grok.TypeInject, grok.TypeList:
		return true, true
	default:
		return handleErr == nil, false
	}
}
