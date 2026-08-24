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

func TestCollectResultRetryFalseWhenNotPrompt(t *testing.T) {
	s := &botServer{rt: &agentRuntime{}, mailboxID: "gbr-x"}
	res := s.collectResult("missing", "cmd-1", 0, 0, 400)
	if res["retry"] != false {
		t.Fatalf("timeout/empty must set retry=false so bots do not re-inject: %+v", res)
	}
	if res["state"] == "idle" && res["prompt"] == true {
		t.Fatalf("missing session must not look like a prompt: %+v", res)
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

func TestBotDiscoveryHasChainEndpoints(t *testing.T) {
	s := &botServer{mailboxID: "gbr-testcode", port: 8788}
	w := httptest.NewRecorder()
	s.writeDiscovery(w)
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	eps, _ := got["endpoints"].(map[string]any)
	for _, k := range []string{"open", "result", "lock", "tasks"} {
		if eps[k] == nil {
			t.Fatalf("missing endpoint %s: %s", k, w.Body.String())
		}
	}
	if got["chain"] == nil {
		t.Fatal("discovery must advertise the chain recipe")
	}
}

func TestHandleLockAndResult(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	s := &botServer{rt: &agentRuntime{}, mailboxID: "gbr-x"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/lock", strings.NewReader(`{"session_id":"proj-a","holder":"grok-bot"}`))
	s.handleLock(w, r)
	if w.Code != 200 {
		t.Fatalf("lock %d %s", w.Code, w.Body.String())
	}
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodPost, "/v1/lock", strings.NewReader(`{"session_id":"proj-a","holder":"claude-coworker"}`))
	s.handleLock(w2, r2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d %s", w2.Code, w2.Body.String())
	}
	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodGet, "/v1/result", nil)
	s.handleResult(w3, r3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("result without session should 400, got %d", w3.Code)
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

func TestWriteStatusIncludesDevices(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	s := &botServer{mailboxID: "gbr-testcode", port: 8788, key: "secret"}

	decode := func(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var got map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("json: %v body=%s", err, w.Body.String())
		}
		return got
	}
	devicesOf := func(t *testing.T, got map[string]any) []any {
		t.Helper()
		devs, _ := got["devices"].([]any)
		if len(devs) == 0 {
			t.Fatalf("devices must be a non-empty array: %v", got["devices"])
		}
		for i, raw := range devs {
			d, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("device %d not an object: %v", i, raw)
			}
			if _, leak := d["mailbox_key"]; leak {
				t.Fatalf("device %d leaked mailbox_key: %v", i, d)
			}
		}
		return devs
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	s.writeStatus(w, r)
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	got := decode(t, w)
	devs := devicesOf(t, got)
	first, _ := devs[0].(map[string]any)
	if first["id"] != "local" || first["kind"] != "local" {
		t.Fatalf("first device want local: %v", first)
	}

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/v1/status?device=local", nil)
	s.writeStatus(w2, r2)
	if w2.Code != 200 {
		t.Fatalf("status?device=local %d %s", w2.Code, w2.Body.String())
	}
	got2 := decode(t, w2)
	_ = devicesOf(t, got2)
	sel, _ := got2["device"].(map[string]any)
	if sel == nil || sel["id"] != "local" {
		t.Fatalf("device selector want id=local: %v", got2["device"])
	}

	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodGet, "/v1/status?device=no-such-box", nil)
	s.writeStatus(w3, r3)
	if w3.Code < 400 || w3.Code > 499 {
		t.Fatalf("unknown device want 4xx, got %d %s", w3.Code, w3.Body.String())
	}
	got3 := decode(t, w3)
	if got3["error"] != "unknown_device" {
		t.Fatalf("want unknown_device, got %s", w3.Body.String())
	}
}

func TestBotInjectUnknownDeviceDoesNotFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	s := &botServer{rt: &agentRuntime{}, mailboxID: "gbr-x"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/inject", strings.NewReader(`{"device":"typo-box","text":"hello","submit":true}`))
	s.handleInject(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 unknown_device, got %d %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] != "unknown_device" || got["ok"] != false {
		t.Fatalf("body %s", w.Body.String())
	}
}

func TestBotOpenUnknownDeviceDoesNotFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	s := &botServer{rt: &agentRuntime{}, mailboxID: "gbr-x"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/sessions/open", strings.NewReader(`{"device":"typo-box","holder":"grok-bot"}`))
	s.handleOpen(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 unknown_device, got %d %s", w.Code, w.Body.String())
	}
}

func TestBotInjectPhoneRefused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	s := &botServer{rt: &agentRuntime{}, mailboxID: "gbr-x"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/inject", strings.NewReader(`{"device":"phone","text":"hello"}`))
	s.handleInject(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 cannot_inject_phone, got %d %s", w.Code, w.Body.String())
	}
}

func TestBotHealthIncludesClassAndCompanions(t *testing.T) {
	s := &botServer{mailboxID: "gbr-x", port: 8788}
	w := httptest.NewRecorder()
	s.writeHealth(w)
	if w.Code != 200 {
		t.Fatalf("health %d %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	h, _ := got["health"].(map[string]any)
	if h == nil || h["class"] == nil {
		t.Fatalf("health.class missing: %s", w.Body.String())
	}
	if _, ok := h["companions"].([]any); !ok {
		t.Fatalf("health.companions missing: %s", w.Body.String())
	}
	classes, _ := got["classes"].([]any)
	if len(classes) != 5 {
		t.Fatalf("health.classes want 5, got %v", got["classes"])
	}
}
