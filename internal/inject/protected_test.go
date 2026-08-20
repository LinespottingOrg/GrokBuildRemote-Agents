package inject

import "testing"

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
