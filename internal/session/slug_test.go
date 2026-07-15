package session

import (
	"strings"
	"testing"
)

func TestValidSessionID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id   string
		want bool
	}{
		{"ab", true},
		{"global-edition", true},
		{"a", false},
		{"", false},
		{"-abc", false},
		{"ABC", false},
		{"has_under", false},
		{"ok-123", true},
		{strings.Repeat("a", 63), true},
		{strings.Repeat("a", 64), false},
		{"a" + strings.Repeat("-", 61) + "b", true},
	}
	for _, tc := range cases {
		if got := ValidSessionID(tc.id); got != tc.want {
			t.Errorf("ValidSessionID(%q)=%v want %v", tc.id, got, tc.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"Global Edition", "global-edition"},
		{"My_Repo", "my-repo"},
		{"  Foo/Bar  ", "foo-bar"},
		{"", "session"},
		{"A", "a0"},
		{"---", "session"},
		{strings.Repeat("x", 100), strings.Repeat("x", 63)},
		{"C:\\Users\\User\\proj", "c-users-user-proj"},
	}
	for _, tc := range cases {
		got := Slugify(tc.in)
		if got != tc.want {
			t.Errorf("Slugify(%q)=%q want %q", tc.in, got, tc.want)
		}
		if !ValidSessionID(got) {
			t.Errorf("Slugify(%q)=%q is not ValidSessionID", tc.in, got)
		}
	}
}

func TestSlugifyAlwaysValid(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"!", "@#$", "日本語", "x", "0", ".", "..", "a--b", "---a---",
		"MiXeD Case Name!!!", strings.Repeat("-a", 40),
	}
	for _, in := range inputs {
		got := _slugifyCheck(t, in)
		if len(got) < 2 || len(got) > 63 {
			t.Errorf("length out of range for %q → %q", in, got)
		}
	}
}

func _slugifyCheck(t *testing.T, in string) string {
	t.Helper()
	got := Slugify(in)
	if !ValidSessionID(got) {
		t.Fatalf("Slugify(%q)=%q invalid", in, got)
	}
	return got
}
