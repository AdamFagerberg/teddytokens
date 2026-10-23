// Command teddytokens shows Claude Code token spend as a pile of pixel-art
// characters: one falls from the top for every --per tokens.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AdamFagerberg/teddytokens/internal/tokens"
	"github.com/AdamFagerberg/teddytokens/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	err := run()
	if errors.Is(err, tea.ErrInterrupted) {
		os.Exit(130) // SIGINT: the terminal is already restored, exit quietly
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "teddytokens:", err)
		os.Exit(1)
	}
}

func run() error {
	per := flag.Int64("per", 10000, "tokens per character")
	teddy := flag.Bool("teddy", false, "start with the teddy bear skin")
	today := flag.Bool("today", false, "count all projects' sessions from today")
	all := flag.Bool("all", false, "count every session in every project")
	cacheReads := flag.Bool("include-cache-reads", false, "also count cache_read_input_tokens")
	demo := flag.Bool("demo", false, "use fake, bursty token growth instead of real transcripts")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("teddytokens", version)
		return nil
	}
	if *per <= 0 {
		return errors.New("--per must be positive")
	}
	if *today && *all {
		return errors.New("--today and --all are mutually exclusive")
	}

	cfg := ui.Config{Per: *per, Teddy: *teddy, Seed: uint64(time.Now().UnixNano())}
	if *demo {
		d := ui.NewDemo(cfg.Seed)
		cfg.Source, cfg.Initial, cfg.Scope = d, d.Total(), "demo"
	} else {
		scope := tokens.ScopeSession
		switch {
		case *today:
			scope = tokens.ScopeToday
		case *all:
			scope = tokens.ScopeAll
		}
		tr, err := tokens.Load(tokens.Options{Scope: scope, IncludeCacheReads: *cacheReads})
		if err != nil {
			return err
		}
		cfg.Source, cfg.Initial, cfg.Scope = tr, tr.Total(), scope.String()
		if *cacheReads {
			cfg.Scope += "+cache"
		}
	}

	_, err := tea.NewProgram(ui.New(cfg)).Run()
	return err
}
