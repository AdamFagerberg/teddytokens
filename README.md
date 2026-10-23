# teddytokens

A terminal toy that shows how many Claude Code tokens you've spent. Every
1,000 tokens a pixel-art character drops from the top of the terminal and
lands on a pile with real (home-made) physics. When the pile reaches the top
the characters merge into a big one; three big ones merge into a giant. There
is only ever one giant on screen: later ones join it, and the `N giants`
counter in the status bar keeps score.

<!-- TODO: add a recording (e.g. `vhs` or asciinema + agg) at docs/demo.gif
and embed it here: ![teddytokens demo](docs/demo.gif) -->

Press `t` to swap the Claude mascot for a teddy bear.

## Install

With Homebrew (macOS and Linux):

```sh
brew install adamfagerberg/tap/teddytokens
```

With Go 1.27 or newer:

```sh
go install github.com/AdamFagerberg/teddytokens@latest
```

### From a clone

```sh
go run . --demo      # try it straight from the repo
go install .         # build and install to $(go env GOPATH)/bin
```

By default teddytokens tracks the session of the directory it is started in,
so `go run .` inside the repo only shows sessions for the repo itself. To
watch another project, `go install .` once (with `$(go env GOPATH)/bin` on
your `PATH`) and run `teddytokens` from that project.

## Usage

Run it in a spare pane in the project you're working on with Claude Code:

```sh
teddytokens            # current session of the project in this directory
teddytokens --today    # every project, today
teddytokens --all      # everything, ever
teddytokens --demo     # fake bursty token growth, costs nothing
```

In the default session mode, tokens you've already spent start as a settled
pile. With `--today` or `--all` the pile starts empty (the status bar still
shows the full total). Either way, only new tokens fall from the top. Big bursts (say 200k at once) are queued and released a few
characters per second.

### Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--per <n>` | `1000` | tokens per character |
| `--teddy` | off | start with the teddy bear skin |
| `--today` | off | count sessions from all projects, today (local time) |
| `--all` | off | count every session in every project |
| `--include-cache-reads` | off | also count `cache_read_input_tokens` |
| `--demo` | off | fake token growth instead of reading transcripts |
| `--version` | | print the version and exit |

### Keys

| Key | Action |
| --- | --- |
| `t` | toggle Claude / teddy skin (in place, the pile isn't disturbed) |
| `r` | replay: drop every character again from the top |
| `s` | shake: throw every character into the air |
| `q`, `Ctrl-C` | quit |

## How tokens are counted

teddytokens reads Claude Code's transcripts under
`~/.claude/projects/<project-dir>/`, where `<project-dir>` is the working
directory with every non-alphanumeric character replaced by `-`. It re-reads
only newly appended bytes every half second, so it stays cheap on large logs.

- Each assistant message counts `input_tokens + output_tokens +
  cache_creation_input_tokens`. Cache reads are excluded by default because
  they're huge and re-counted on every turn; add `--include-cache-reads` to
  count them too.
- A message can appear on several transcript lines while it streams. Lines are
  deduplicated by `message.id`, keeping the largest usage seen for it.
- Subagent transcripts (`<session>/subagents/*.jsonl`) are counted along with
  their session.
- The default scope is the most recently modified session in the project for
  the current directory. If a newer session starts, teddytokens follows it and
  keeps adding to the running total.

A very long session would bury the screen many times over, so at startup
every pile it would have filled becomes a big character, every three bigs a
giant, and only the remainder is drawn as small ones. A huge burst is handled
the same way. In a small terminal the big and giant characters are drawn at a
smaller scale so they still fit.

## Development

```sh
go run . --demo
go test -race ./...
golangci-lint run ./...
```

## License

MIT
