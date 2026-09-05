package inject

import (
	"errors"
	"testing"
)

func TestIsProtectedTitle(t *testing.T) {
	if !IsProtectedTitle("++ Felanmälan.org") {
		t.Fatal("Felanmälan must be protected")
	}
	if !IsProtectedTitle("++ QA PC ANdroid") {
		t.Fatal("QA PC Android must be protected")
	}
	if IsProtectedTitle("Grok Build") {
		t.Fatal("plain Grok Build is not protected")
	}
	if IsProtectedTitle("gbr-open-6d9acaaf") {
		t.Fatal("agent-opened id is not protected")
	}
}

func TestRefuseProtected(t *testing.T) {
	if err := RefuseProtected("Grok Build"); err != nil {
		t.Fatalf("plain grok must be allowed: %v", err)
	}
	err := RefuseProtected("++ Felanmälan.org")
	if err == nil {
		t.Fatal("expected ErrProtected")
	}
	if !errors.Is(err, ErrProtected) {
		t.Fatalf("want ErrProtected, got %v", err)
	}
}
