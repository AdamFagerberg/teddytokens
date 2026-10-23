// Package tokens reads Claude Code transcripts and keeps a running total of
// the tokens they record.
//
// Transcripts live under ~/.claude/projects/<project-dir>/<session>.jsonl,
// with subagent transcripts under <project-dir>/<session>/subagents/*.jsonl.
// Each assistant line carries message.id and message.usage. A message can
// appear on several lines while it streams, with output_tokens growing, so
// the Tracker counts each message id once at its largest observed value.
package tokens

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scope selects which transcripts are counted.
type Scope int

const (
	// ScopeSession counts the most recently modified session (and its
	// subagents) in the project dir matching the working directory.
	ScopeSession Scope = iota
	// ScopeToday counts entries from all projects timestamped today, local time.
	ScopeToday
	// ScopeAll counts every entry in every project.
	ScopeAll
)

// String returns the lowercase name of the scope.
func (s Scope) String() string {
	switch s {
	case ScopeToday:
		return "today"
	case ScopeAll:
		return "all"
	default:
		return "session"
	}
}

// Options configures a Tracker.
type Options struct {
	Scope Scope
	// IncludeCacheReads adds cache_read_input_tokens to the count.
	IncludeCacheReads bool
	// Root is the transcripts root. Defaults to ~/.claude/projects.
	Root string
	// Cwd selects the project in ScopeSession. Defaults to os.Getwd.
	Cwd string
	// Now returns the current time, used by ScopeToday. Defaults to time.Now.
	Now func() time.Time
}

// Tracker incrementally tails transcripts and sums their tokens.
type Tracker struct {
	opts  Options
	files map[string]*fileState
	seen  map[string]int64 // message id -> tokens counted for it
	total int64
}

type fileState struct {
	offset  int64
	partial []byte // trailing bytes not yet terminated by a newline
}

type entry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		ID    string `json:"id"`
		Usage *struct {
			Input         int64 `json:"input_tokens"`
			Output        int64 `json:"output_tokens"`
			CacheCreation int64 `json:"cache_creation_input_tokens"`
			CacheRead     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ProjectDir returns the transcript directory name Claude Code uses for cwd:
// every character other than an ASCII letter or digit becomes "-".
func ProjectDir(cwd string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, cwd)
}

// Load creates a Tracker and reads all existing matching transcripts, so
// Total reports the starting count.
func Load(opts Options) (*Tracker, error) {
	if opts.Root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("locate home dir: %w", err)
		}
		opts.Root = filepath.Join(home, ".claude", "projects")
	}
	if opts.Cwd == "" && opts.Scope == ScopeSession {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working dir: %w", err)
		}
		opts.Cwd = cwd
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	t := &Tracker{
		opts:  opts,
		files: make(map[string]*fileState),
		seen:  make(map[string]int64),
	}
	if _, err := t.Poll(); err != nil {
		return nil, err
	}
	return t, nil
}

// Total returns the number of tokens counted so far.
func (t *Tracker) Total() int64 { return t.total }

// Poll reads bytes appended since the last call, including any new
// transcript files in scope, and returns the number of tokens added.
// In ScopeSession a newer session file replaces the current one; the
// total keeps accumulating rather than resetting.
func (t *Tracker) Poll() (int64, error) {
	paths, err := t.discover()
	if err != nil {
		return 0, err
	}
	var delta int64
	for _, p := range paths {
		d, err := t.readFile(p)
		if err != nil {
			return delta, err
		}
		delta += d
	}
	t.total += delta
	return delta, nil
}

// discover lists the transcript files currently in scope.
func (t *Tracker) discover() ([]string, error) {
	if t.opts.Scope == ScopeSession {
		return t.sessionFiles()
	}
	var since time.Time
	if t.opts.Scope == ScopeToday {
		since = startOfDay(t.opts.Now())
	}
	var paths []string
	err := filepath.WalkDir(t.opts.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == t.opts.Root && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() || filepath.Ext(p) != ".jsonl" {
			return nil
		}
		if !since.IsZero() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.ModTime().Before(since) {
				return nil // cannot contain entries from today
			}
		}
		paths = append(paths, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", t.opts.Root, err)
	}
	return paths, nil
}

// sessionFiles returns the latest-modified session transcript in the
// project dir plus its subagent transcripts.
func (t *Tracker) sessionFiles() ([]string, error) {
	dir := filepath.Join(t.opts.Root, ProjectDir(t.opts.Cwd))
	mains, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var latest string
	var latestMod time.Time
	for _, p := range mains {
		info, err := os.Stat(p)
		if err != nil {
			continue // removed between glob and stat
		}
		if latest == "" || info.ModTime().After(latestMod) {
			latest, latestMod = p, info.ModTime()
		}
	}
	if latest == "" {
		return nil, nil
	}
	subs, err := filepath.Glob(filepath.Join(strings.TrimSuffix(latest, ".jsonl"), "subagents", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	return append([]string{latest}, subs...), nil
}

// readFile consumes appended bytes of path and returns the tokens added.
func (t *Tracker) readFile(path string) (int64, error) {
	st := t.files[path]
	if st == nil {
		st = &fileState{}
		t.files[path] = st
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer func() { _ = f.Close() }() // read-only; close error is irrelevant
	if _, err := f.Seek(st.offset, io.SeekStart); err != nil {
		return 0, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}
	st.offset += int64(len(data))
	data = append(st.partial, data...)

	var delta int64
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		delta += t.countLine(data[:i])
		data = data[i+1:]
	}
	st.partial = bytes.Clone(data)
	return delta, nil
}

// countLine returns the tokens a single transcript line adds to the total.
func (t *Tracker) countLine(line []byte) int64 {
	if !bytes.Contains(line, []byte(`"usage"`)) {
		return 0 // cheap skip for the many lines without usage
	}
	var e entry
	if err := json.Unmarshal(line, &e); err != nil {
		return 0
	}
	u := e.Message.Usage
	if e.Message.ID == "" || u == nil {
		return 0
	}
	if t.opts.Scope == ScopeToday && e.Timestamp.Before(startOfDay(t.opts.Now())) {
		return 0
	}
	n := u.Input + u.Output + u.CacheCreation
	if t.opts.IncludeCacheReads {
		n += u.CacheRead
	}
	prev, ok := t.seen[e.Message.ID]
	if ok && n <= prev {
		return 0
	}
	t.seen[e.Message.ID] = n
	return n - prev
}

func startOfDay(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}
