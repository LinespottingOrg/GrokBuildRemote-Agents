package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBotAuthorizeLoopbackOpen(t *testing.T) {
	s := &botServer{key: "secret"}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if !s.authorize(r) {
		t.Fatal("loopback should allow missing key by default")
	}
	r.Header.Set("X-GBR-Key", "secret")
	if !s.authorize(r) {
		t.Fatal("matching key should pass")
	}
	r.Header.Set("X-GBR-Key", "nope")
	if s.authorize(r) {
		t.Fatal("wrong key must fail")
	}
	r.Header.Del("X-GBR-Key")
	r.Header.Set("Authorization", "Bearer secret")
	if !s.authorize(r) {
		t.Fatal("bearer key should pass")
	}
}

func TestBotAuthorizeRequireKey(t *testing.T) {
	t.Setenv("GBR_BOT_REQUIRE_KEY", "1")
	s := &botServer{key: "secret"}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if s.authorize(r) {
		t.Fatal("require-key must reject empty")
	}
	r.Header.Set("X-GBR-Key", "secret")
	if !s.authorize(r) {
		t.Fatal("require-key should accept match")
	}
}

func TestBotDiscoveryJSON(t *testing.T) {
	s := &botServer{mailboxID: "gbr-testcode", port: 8788}
	w := httptest.NewRecorder()
	s.writeDiscovery(w)
	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != true {
		t.Fatalf("ok=%v body=%s", got["ok"], body)
	}
	if got["service"] != "gbr-agent-bot" {
		t.Fatalf("service=%v", got["service"])
	}
	eps, _ := got["endpoints"].(map[string]any)
	if eps == nil || eps["inject"] == nil {
		t.Fatalf("missing endpoints: %s", body)
	}
}

func TestBotInjectEmpty(t *testing.T) {
	s := &botServer{rt: &agentRuntime{}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/inject", strings.NewReader(`{"text":""}`))
	s.handleInject(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 empty_text, got %d %s", w.Code, w.Body.String())
	}
}

func TestIsLoopback(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:54321"
	if !isLoopback(r) {
		t.Fatal("127.0.0.1 should be loopback")
	}
	r.RemoteAddr = "[::1]:9"
	if !isLoopback(r) {
		t.Fatal("::1 should be loopback")
	}
	r.RemoteAddr = "8.8.8.8:443"
	if isLoopback(r) {
		t.Fatal("8.8.8.8 must not be loopback")
	}
}

func TestBotOutputsFilter(t *testing.T) {
	rt := &agentRuntime{}
	rt.recordBotOutput(botOutputItem{TS: "2026-08-16T00:00:00Z", SessionID: "a", CommandID: "1", Chunk: "one"})
	rt.recordBotOutput(botOutputItem{TS: "2026-08-16T00:00:01Z", SessionID: "b", CommandID: "2", Chunk: "two"})
	got := rt.botOutputs("b", "", "", 10)
	if len(got) != 1 || got[0].Chunk != "two" {
		t.Fatalf("filter session: %+v", got)
	}
}
