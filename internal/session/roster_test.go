package session

import "testing"

func TestClipRoster_SoftMax255(t *testing.T) {
	t.Parallel()
	small := []int{1, 2, 3}
	kept, dropped := ClipRoster(small)
	if dropped != 0 || len(kept) != 3 {
		t.Fatalf("small kept=%v dropped=%d", kept, dropped)
	}

	in := make([]int, MaxSessions+40)
	for i := range in {
		in[i] = i
	}
	kept, dropped = ClipRoster(in)
	if dropped != 40 || len(kept) != MaxSessions {
		t.Fatalf("len=%d dropped=%d want max=%d dropped=40", len(kept), dropped, MaxSessions)
	}
	if kept[0] != 0 || kept[MaxSessions-1] != MaxSessions-1 {
		t.Fatalf("order broken: first=%d last=%d", kept[0], kept[MaxSessions-1])
	}
}

func TestMaxSessionsIsSoftNotSix(t *testing.T) {
	t.Parallel()
	if MaxSessions <= 6 || MaxSessions > 255 {
		t.Fatalf("MaxSessions=%d want 7..255", MaxSessions)
	}
}
