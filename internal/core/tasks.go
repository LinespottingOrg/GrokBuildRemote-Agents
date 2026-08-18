package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Tasks persist the diagnose → inject → judge loop so Grok bot and Claude
// Cowork can iterate without keeping the whole conversation in RAM.

const TaskFileName = "tasks.json"

const (
	TaskOpen    = "open"
	TaskRunning = "running"
	TaskIdle    = "idle"
	TaskDone    = "done"
	TaskFailed  = "failed"
)

var ErrTaskNotFound = errors.New("task not found")

// Task is one feedback-loop job bound to a session.
type Task struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	Device      string `json:"device,omitempty"`
	Holder      string `json:"holder,omitempty"`
	Goal        string `json:"goal"`
	Status      string `json:"status"`
	Iteration   int    `json:"iteration"`
	LastExcerpt string `json:"last_excerpt,omitempty"`
	LastJudge   string `json:"last_judge,omitempty"`
	CommandID   string `json:"command_id,omitempty"`
	Created     string `json:"created"`
	Updated     string `json:"updated"`
}

type taskFile struct {
	Tasks   []Task `json:"tasks"`
	Updated string `json:"updated,omitempty"`
}

const maxTasks = 200

var taskMu sync.Mutex

func taskPath() (string, error) {
	dir, err := deviceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, TaskFileName), nil
}

func loadTasksLocked() ([]Task, error) {
	path, err := taskPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f taskFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, nil
	}
	return f.Tasks, nil
}

func saveTasksLocked(list []Task) error {
	if len(list) > maxTasks {
		list = list[len(list)-maxTasks:]
	}
	path, err := taskPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f := taskFile{Tasks: list, Updated: time.Now().UTC().Format(time.RFC3339Nano)}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func normTaskStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case TaskOpen, TaskRunning, TaskIdle, TaskDone, TaskFailed:
		return strings.ToLower(strings.TrimSpace(s))
	case "complete", "completed", "closed":
		return TaskDone
	case "error", "fail":
		return TaskFailed
	case "work", "working":
		return TaskRunning
	default:
		if s == "" {
			return TaskOpen
		}
		return TaskOpen
	}
}

// UpsertTask creates or updates a task. Empty ID allocates a uuid.
func UpsertTask(t Task) (Task, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	t.Status = normTaskStatus(t.Status)
	t.Holder = normalizeHolder(t.Holder)
	t.SessionID = strings.TrimSpace(t.SessionID)
	t.Goal = strings.TrimSpace(t.Goal)
	t.Device = strings.TrimSpace(t.Device)
	if t.Device == "" {
		t.Device = "local"
	}
	if t.ID == "" {
		t.ID = uuid.NewString()
		t.Created = now
	}
	t.Updated = now
	if t.Iteration < 0 {
		t.Iteration = 0
	}

	taskMu.Lock()
	defer taskMu.Unlock()
	list, err := loadTasksLocked()
	if err != nil {
		return Task{}, err
	}
	found := false
	for i, cur := range list {
		if cur.ID == t.ID {
			if t.Created == "" {
				t.Created = cur.Created
			}
			if t.Goal == "" {
				t.Goal = cur.Goal
			}
			if t.SessionID == "" {
				t.SessionID = cur.SessionID
			}
			if t.LastExcerpt == "" {
				t.LastExcerpt = cur.LastExcerpt
			}
			if t.CommandID == "" {
				t.CommandID = cur.CommandID
			}
			if t.Iteration == 0 {
				t.Iteration = cur.Iteration
			}
			list[i] = t
			found = true
			break
		}
	}
	if !found {
		if t.Created == "" {
			t.Created = now
		}
		list = append(list, t)
	}
	if err := saveTasksLocked(list); err != nil {
		return Task{}, err
	}
	return t, nil
}

// GetTask looks up by id.
func GetTask(id string) (Task, bool) {
	taskMu.Lock()
	defer taskMu.Unlock()
	list, err := loadTasksLocked()
	if err != nil {
		return Task{}, false
	}
	for _, t := range list {
		if t.ID == id {
			return t, true
		}
	}
	return Task{}, false
}

// ListTasks returns newest last. sessionID empty = all.
func ListTasks(sessionID string) []Task {
	taskMu.Lock()
	defer taskMu.Unlock()
	list, err := loadTasksLocked()
	if err != nil {
		return nil
	}
	if sessionID == "" {
		return list
	}
	out := make([]Task, 0, len(list))
	for _, t := range list {
		if t.SessionID == sessionID {
			out = append(out, t)
		}
	}
	return out
}

// TouchTaskIteration bumps iteration and stores the latest excerpt.
func TouchTaskIteration(id, excerpt, commandID, status string) (Task, error) {
	taskMu.Lock()
	defer taskMu.Unlock()
	list, err := loadTasksLocked()
	if err != nil {
		return Task{}, err
	}
	for i, t := range list {
		if t.ID != id {
			continue
		}
		t.Iteration++
		if excerpt != "" {
			t.LastExcerpt = excerpt
		}
		if commandID != "" {
			t.CommandID = commandID
		}
		if status != "" {
			t.Status = normTaskStatus(status)
		} else if t.Status == TaskOpen {
			t.Status = TaskRunning
		}
		t.Updated = time.Now().UTC().Format(time.RFC3339Nano)
		list[i] = t
		if err := saveTasksLocked(list); err != nil {
			return Task{}, err
		}
		return t, nil
	}
	return Task{}, ErrTaskNotFound
}
