# teddytokens

A terminal toy that shows how many Claude Code tokens you've spent. Every
10,000 tokens a pixel-art character drops from the top of the terminal and
lands on a pile with real (home-made) physics. When the pile reaches the top
it gets squashed and cleared, and a `×N` counter keeps score.

<!-- TODO: replace with a recording, e.g. `vhs` or asciinema + agg -->
![teddytokens demo](docs/demo.gif)

Press `t` to swap the Claude mascot for a teddy bear.

## Install

With Homebrew (macOS and Linux):

```sh
# TODO: confirm the tap name once AdamFagerberg/homebrew-tap exists
brew install --cask AdamFagerberg/tap/teddytokens
```

With Go 1.27 or newer:

```sh
go install github.com/AdamFagerberg/teddytokens@latest
```

## Usage

Run it in a spare pane in the project you're working on with Claude Code:

```sh
teddytokens            # current session of the project in this directory
teddytokens --today    # every project, today
teddytokens --all      # everything, ever
teddytokens --demo     # fake bursty token growth, costs nothing
```

Tokens you've already spent start as a settled pile; only new tokens fall
from the top. Big bursts (say 200k at once) are queued and released a few
characters per second.

### Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--per <n>` | `10000` | tokens per character |
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
| `r` | replay: drop the whole pile again from the top |
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

A very long history would bury the screen many times over, so at startup the
piles it would have filled are counted straight into `×N` and only the
remainder is drawn.

## Development

```sh
go run . --demo
go test -race ./...
golangci-lint run ./...
```

## Releasing

Releases are built by GoReleaser from GitHub Actions when a `v*` tag is pushed:

```sh
git tag v0.1.0
git push origin v0.1.0
```

This publishes darwin/linux archives for amd64 and arm64 with checksums, and
updates the Homebrew cask in the tap. The workflow needs a
`HOMEBREW_TAP_GITHUB_TOKEN` repository secret with write access to the tap
repo. Try the build locally with `goreleaser release --snapshot --clean`.

## License

MIT
