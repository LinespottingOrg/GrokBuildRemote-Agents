package core

import (
	"os"
	"path/filepath"
	"testing"
)

func withTaskHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".gbr"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertAndTouchTask(t *testing.T) {
	withTaskHome(t)
	got, err := UpsertTask(Task{Goal: "fix tests", SessionID: "proj-a", Holder: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.Status != TaskOpen || got.Holder != "claude-coworker" {
		t.Fatalf("%+v", got)
	}
	next, err := TouchTaskIteration(got.ID, "ok excerpt", "cmd-1", TaskIdle)
	if err != nil {
		t.Fatal(err)
	}
	if next.Iteration != 1 || next.LastExcerpt != "ok excerpt" || next.Status != TaskIdle {
		t.Fatalf("%+v", next)
	}
	list := ListTasks("proj-a")
	if len(list) != 1 {
		t.Fatalf("list=%d", len(list))
	}
}

func TestGetMissingTask(t *testing.T) {
	withTaskHome(t)
	if _, ok := GetTask("nope"); ok {
		t.Fatal("missing task")
	}
	if _, err := TouchTaskIteration("nope", "", "", ""); err != ErrTaskNotFound {
		t.Fatalf("got %v", err)
	}
}
