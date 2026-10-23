# Prompt: build `teddytokens`

Build **teddytokens**, a terminal app that shows how many Claude Code tokens I've spent. Every 10,000 tokens, a pixel-art character spawns at the top of the terminal and falls to the bottom, with physics. The more tokens I spend, the bigger the pile gets.

Build it in the current folder (`/Users/adam/Desktop/compileit/teddytokens`), which is empty and not yet a git repo. Connect it to `https://github.com/AdamFagerberg/teddytokens.git`.

## Stack

- Go (latest stable) using the Charm stack:
  - **Bubble Tea** for the app loop, with the alt screen and a tick-driven frame loop
  - **Bubbles** components where they fit: `progress` for the next-spawn bar, `help` + `key` for the key hints
  - **Lip Gloss** for HUD styling
- Use the current stable major version of each Charm library, and check which one that is before you pin it.
- Module path: `github.com/AdamFagerberg/teddytokens`. The binary is called `teddytokens`.
- Suggested layout: `main.go`, `internal/tokens` (parse/watch), `internal/physics`, `internal/sprites`, `internal/ui` (Bubble Tea model).
- Write the physics engine yourself. Don't add a physics dependency.
- The app has to be installable from a Homebrew tap later:
  - Add a `.goreleaser.yaml` that builds darwin/linux binaries for amd64 and arm64, with `-ldflags` setting the version.
  - Add a `brews:` section pointing at a tap repo. Leave the tap owner/name as a clearly marked TODO.
  - Add `teddytokens --version`.
  - Don't tag or release anything yet.

## Token source

Read the Claude Code session transcripts at `~/.claude/projects/<project-dir>/*.jsonl`. Each assistant line has `message.id` and `message.usage`, like this:

```json
"usage": {"input_tokens":2,"cache_creation_input_tokens":28752,"cache_read_input_tokens":41314,"output_tokens":1361}
```

- **Deduplicate by `message.id`.** The same message can show up on several lines.
- **Count** `input_tokens + output_tokens + cache_creation_input_tokens` by default. Add a `--include-cache-reads` flag that also counts `cache_read_input_tokens`. Cache reads are very large and would flood the screen, so they're off by default.
- **Scope**:
  - Default: the most recently modified session in the project dir that matches the current working directory. The dir name is the cwd with `/` replaced by `-`.
  - `--today`: all sessions across all projects, today.
  - `--all`: everything.
- **Watch** the files with `fs.watch` plus polling as a fallback. Read only the newly appended bytes, and tolerate a partial last line.
- On startup, existing tokens should already be lying on the floor as a settled pile, not raining in. Only tokens that arrive afterwards fall from the top.

## Characters

Draw the sprites with half-block characters (`▀`, `▄`, `█`) so each terminal cell holds 2 vertical pixels, using truecolor foreground and background. Store the sprites as plain string grids in `internal/sprites`.

**Claude** (orange `#D97757`, dark eyes `#2B2B2B`, 14×7 px). `X` is body, `E` is eye, `.` is transparent:

```
..XXXXXXXXXX..
..XXEXXXXEXX..
..XXEXXXXEXX..
XXXXXXXXXXXXXX
..XXXXXXXXXX..
..XXXXXXXXXX..
...X.X..X.X...
```

**Teddy** (brown `#8B5A2B`, muzzle and inner ears `#D2A679`, eyes and nose `#1E1E1E`, about 14×10 px). Design a cute pixel teddy bear head with round ears, two eyes, a lighter muzzle and a nose. It should read clearly at this size.

- Press `t` to toggle between Claude and Teddy, or start in a mode with `--teddy`. The toggle re-skins every existing body in place; the physics doesn't reset.
- Give each spawned body a slight random color variation (±8% lightness) so the pile doesn't look flat.

## Physics

- Each character is a rigid axis-aligned box the size of its sprite. Don't rotate the bodies, because rotated pixel art looks bad in a terminal.
- **Motion**: gravity, a fixed timestep (60 Hz simulation, rendering capped at about 30 fps), restitution of about 0.3 so bodies bounce a little, and friction.
- **Collisions**: floor, walls, and body-on-body collisions, resolved with a few iterations per step so stacks are stable.
- **Spawning**: new bodies spawn at a random x near the top, with a small random horizontal velocity.
- **Sleeping**: bodies that have rested for a while go to sleep, which keeps CPU low with hundreds of bodies.
- **Resize**: when the terminal is resized, update the walls and floor and wake every body.
- **Full screen**: when the pile reaches the top row, play a short "squash" animation that clears it and keep counting. Show a small `×N` counter for how many piles have been cleared.
- **Bursts**: if a large chunk of tokens arrives at once (for example 200k), queue the spawns and release them a few per second instead of all at once.

## HUD

Show a one-line status bar at the top or bottom:

- total tokens, formatted (`1.24M`)
- the number of characters
- progress toward the next spawn, as a mini bar
- the current scope
- the key hints: `t` toggle · `r` replay (drop the whole pile again from the top) · `q`/Ctrl-C quit

On exit, always restore the terminal: show the cursor and leave the alternate screen. This includes panics and SIGINT. Bubble Tea handles most of this.

## Config

Support these CLI flags, with no config file:

- `--per <n>`: tokens per character (default 10000)
- `--teddy`
- `--today`
- `--all`
- `--include-cache-reads`
- `--demo`: fake token growth, so you can test without spending tokens

## Done when

1. `go run . --demo` shows characters falling, bouncing and stacking stably, with no jitter in resting piles.
2. Pressing `t` switches between the Claude and Teddy sprites live.
3. Running it alongside a real Claude Code session in another pane spawns a new character about every 10k tokens. Check this against a manual sum from the jsonl.
4. Resizing the terminal and quitting both behave cleanly, and the shell isn't left broken afterwards.
5. Unit tests (`go test ./...`) cover the token parser (dedup, partial lines, scopes) and a physics sanity test: 50 bodies dropped all end up asleep inside the bounds, with no overlaps. `go vet ./...` and `gofmt -l .` must both be clean, and so must `golangci-lint run` if it's installed.
6. There's a README with a short GIF placeholder, install instructions (`brew install <tap>/teddytokens` as a TODO, and `go install`), usage, and the flags.
7. `goreleaser check` passes, and so does `goreleaser release --snapshot --clean` if goreleaser is installed.
8. Run `git init`, add `.gitignore` (dist/, the binary), make the first commit, then `git remote add origin https://github.com/AdamFagerberg/teddytokens.git` and push `main`. Ask before pushing if the remote already has commits.

State your plan briefly before coding, and say so if any of the decisions above look wrong once you're in the code.
