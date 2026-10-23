package tokens

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const fixtureCwd = "/Users/me/my_proj.v2"

var fixtureNow = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

// fixtureRoot copies testdata/projects into a temp dir, makes old.jsonl the
// older session and returns the root.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("testdata/projects")); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(root, "-Users-me-my-proj-v2", "old.jsonl"), old, old); err != nil {
		t.Fatal(err)
	}
	return root
}

func load(t *testing.T, opts Options) *Tracker {
	t.Helper()
	tr, err := Load(opts)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestProjectDir(t *testing.T) {
	tests := []struct{ cwd, want string }{
		{"/Users/adam/Desktop/compileit/teddytokens", "-Users-adam-Desktop-compileit-teddytokens"},
		{"/Users/adam/Desktop/App Repositories", "-Users-adam-Desktop-App-Repositories"},
		{"/Users/adam/jula-app/.claude/worktrees/x", "-Users-adam-jula-app--claude-worktrees-x"},
		{"/Users/me/my_proj.v2", "-Users-me-my-proj-v2"},
	}
	for _, tt := range tests {
		if got := ProjectDir(tt.cwd); got != tt.want {
			t.Errorf("ProjectDir(%q) = %q, want %q", tt.cwd, got, tt.want)
		}
	}
}

// TestLoadScopes covers dedup (including streaming lines whose output grows
// and an id repeated across files), malformed and usage-less lines, the
// cache-read flag, and each scope.
func TestLoadScopes(t *testing.T) {
	tests := []struct {
		name       string
		scope      Scope
		cacheReads bool
		want       int64
	}{
		{"session", ScopeSession, false, 186},
		{"session with cache reads", ScopeSession, true, 1190},
		{"today", ScopeToday, false, 1180},
		{"today with cache reads", ScopeToday, true, 2180},
		{"all", ScopeAll, false, 1193},
		{"all with cache reads", ScopeAll, true, 2197},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := load(t, Options{
				Scope:             tt.scope,
				IncludeCacheReads: tt.cacheReads,
				Root:              fixtureRoot(t),
				Cwd:               fixtureCwd,
				Now:               func() time.Time { return fixtureNow },
			})
			if got := tr.Total(); got != tt.want {
				t.Errorf("Total() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLoadMissing(t *testing.T) {
	for _, scope := range []Scope{ScopeSession, ScopeToday, ScopeAll} {
		tr := load(t, Options{Scope: scope, Root: filepath.Join(t.TempDir(), "nope"), Cwd: fixtureCwd})
		if tr.Total() != 0 {
			t.Errorf("%v: Total() = %d, want 0", scope, tr.Total())
		}
	}
}

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func poll(t *testing.T, tr *Tracker) int64 {
	t.Helper()
	d, err := tr.Poll()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPollPartialLine(t *testing.T) {
	root := fixtureRoot(t)
	tr := load(t, Options{Scope: ScopeAll, Root: root})
	start := tr.Total()

	path := filepath.Join(root, "-Users-me-other", "b.jsonl")
	line := `{"timestamp":"2026-10-23T11:30:00Z","message":{"id":"msg_new","usage":{"input_tokens":40,"output_tokens":2}}}` + "\n"
	half := len(line) / 2

	steps := []struct {
		name   string
		append string
		want   int64
	}{
		{"nothing new", "", 0},
		{"first half", line[:half], 0},
		{"rest of line", line[half:], 42},
		{"same id, larger output", `{"message":{"id":"msg_new","usage":{"input_tokens":40,"output_tokens":10}}}` + "\n", 8},
		{"same id, same usage", `{"message":{"id":"msg_new","usage":{"input_tokens":40,"output_tokens":10}}}` + "\n", 0},
		{"malformed", "{oops\n", 0},
	}
	var sum int64
	for _, s := range steps {
		appendFile(t, path, s.append)
		if got := poll(t, tr); got != s.want {
			t.Errorf("%s: Poll() = %d, want %d", s.name, got, s.want)
		}
		sum += s.want
	}
	if got := tr.Total(); got != start+sum {
		t.Errorf("Total() = %d, want %d", got, start+sum)
	}
}

func TestPollSessionPicksUpNewerFile(t *testing.T) {
	root := fixtureRoot(t)
	tr := load(t, Options{Scope: ScopeSession, Root: root, Cwd: fixtureCwd})
	start := tr.Total()

	// A file in another project is out of scope.
	appendFile(t, filepath.Join(root, "-Users-me-other", "c.jsonl"),
		`{"message":{"id":"msg_other","usage":{"input_tokens":5}}}`+"\n")
	if got := poll(t, tr); got != 0 {
		t.Errorf("other project: Poll() = %d, want 0", got)
	}

	path := filepath.Join(root, "-Users-me-my-proj-v2", "newer.jsonl")
	appendFile(t, path, `{"message":{"id":"msg_newer","usage":{"input_tokens":3,"output_tokens":4}}}`+"\n")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if got := poll(t, tr); got != 7 {
		t.Errorf("newer session: Poll() = %d, want 7", got)
	}
	if got := tr.Total(); got != start+7 {
		t.Errorf("Total() = %d, want %d", got, start+7)
	}
}

func TestScopeString(t *testing.T) {
	tests := map[Scope]string{ScopeSession: "session", ScopeToday: "today", ScopeAll: "all"}
	for s, want := range tests {
		if got := s.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", s, got, want)
		}
	}
}
