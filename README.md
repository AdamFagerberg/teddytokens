# teddytokens

A terminal toy that shows how many Claude Code tokens you've spent. Every
10,000 tokens a pixel-art character drops from the top of the terminal and
lands on a pile with real (home-made) physics. When the pile reaches the top
it gets squashed and cleared, and a `×N` counter keeps score.

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

1. Tag and push. The release workflow runs GoReleaser and publishes a GitHub
   release with darwin/linux amd64/arm64 archives and `checksums.txt`:

   ```sh
   git tag v0.1.0
   git push origin v0.1.0
   ```

   Try the build locally with `goreleaser release --snapshot --clean`.

2. Update the Homebrew tap
   ([AdamFagerberg/homebrew-tap](https://github.com/AdamFagerberg/homebrew-tap)).
   The formula builds from the tag's source tarball. For the first release,
   copy `packaging/homebrew/teddytokens.rb` to `Formula/teddytokens.rb` in the
   tap, set the version in `url` and fill in `sha256`:

   ```sh
   curl -sL https://github.com/AdamFagerberg/teddytokens/archive/refs/tags/v0.1.0.tar.gz | shasum -a 256
   ```

   Open a PR so the tap's test-bot builds bottles, then publish them with the
   `pr-pull` workflow. For later versions:

   ```sh
   brew bump-formula-pr --url https://github.com/AdamFagerberg/teddytokens/archive/refs/tags/vX.Y.Z.tar.gz adamfagerberg/tap/teddytokens
   ```

## License

MIT
