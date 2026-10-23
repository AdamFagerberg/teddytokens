// Package ui is the Bubble Tea front end: it turns token counts into falling
// characters, steps the physics world and draws the pile plus a status bar.
package ui

import (
	"math"
	"math/rand/v2"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/harmonica"

	"github.com/AdamFagerberg/teddytokens/internal/physics"
	"github.com/AdamFagerberg/teddytokens/internal/sprites"
)

const (
	fps          = 30
	pollInterval = 500 * time.Millisecond
	hudRows      = 1
	maxFrameDT   = 0.1  // seconds; longer gaps (e.g. a suspended terminal) are clamped
	spawnSpeed   = 20.0 // max |vx| of a new character, px/s
	settleSteps  = 30 * 60
)

// Source reports tokens added since its previous call. It is only ever
// called from one goroutine at a time.
type Source interface {
	Poll() (int64, error)
}

// Config holds what the model needs from the command line.
type Config struct {
	Source  Source
	Initial int64  // tokens already spent at startup
	Per     int64  // tokens per character, > 0
	Teddy   bool   // start with the teddy skin
	Scope   string // label shown in the status bar
	Seed    uint64
}

// skin caches one body's tinted sprites so toggling never re-tints.
type skin struct{ claude, teddy *sprites.Sprite }

// squash is the pile-reached-the-top animation: scale springs from 1 to 0
// while the pile sinks into the floor, then the world is cleared. counts is
// false when the pile only overflowed because the terminal shrank.
type squash struct {
	scale, vel float64
	counts     bool
}

type (
	frameMsg time.Time
	pollMsg  struct {
		delta int64
		err   error
	}
)

// Model is the Bubble Tea model. Use it through a pointer.
type Model struct {
	src   Source
	rng   *rand.Rand
	scope string
	teddy bool

	total int64
	spawn spawner
	// initial is the number of characters to lay down as a resting pile
	// once the terminal size is known; -1 after that has happened.
	initial int

	width, height int
	world         *physics.World
	canvas        *sprites.Canvas
	skins         []skin // parallel to world.Bodies()
	last          time.Time
	acc           float64 // unsimulated time, seconds
	disturbed     bool    // the pile has not settled since a resize or shake

	squash       *squash
	squashSpring harmonica.Spring
	cleared      int

	bar  progress.Model
	help help.Model
	err  error
}

// New builds the model. Nothing is drawn until the first WindowSizeMsg.
func New(cfg Config) *Model {
	m := &Model{
		src:          cfg.Source,
		rng:          rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9e3779b97f4a7c15)),
		scope:        cfg.Scope,
		teddy:        cfg.Teddy,
		total:        cfg.Initial,
		spawn:        spawner{per: cfg.Per},
		squashSpring: harmonica.NewSpring(harmonica.FPS(fps), 9, 0.55),
		bar: progress.New(progress.WithWidth(12), progress.WithoutPercentage(),
			progress.WithColors(accent)),
		help: help.New(),
	}
	m.spawn.add(cfg.Initial)
	m.initial, m.spawn.queue = m.spawn.queue, 0
	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.frameCmd(), m.pollCmd())
}

func (m *Model) frameCmd() tea.Cmd {
	return tea.Tick(time.Second/fps, func(t time.Time) tea.Msg { return frameMsg(t) })
}

// pollCmd polls the source off the UI goroutine. The next poll is only
// scheduled once this one's result arrives, so polls never overlap.
func (m *Model) pollCmd() tea.Cmd {
	src := m.src
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		d, err := src.Poll()
		return pollMsg{d, err}
	})
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Toggle):
			m.teddy = !m.teddy
		case key.Matches(msg, keys.Replay):
			m.replay()
		case key.Matches(msg, keys.Shake):
			m.shake()
		}
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case pollMsg:
		m.err = msg.err
		m.total += msg.delta
		m.spawn.add(msg.delta)
		if m.world != nil {
			// More than a pile's worth would rain for minutes; count the
			// full piles it would fill, as at startup, and rain the rest.
			if c := pileCapacity(m.world.Size()); m.spawn.queue > c {
				m.cleared += m.spawn.queue / c
				m.spawn.queue %= c
			}
		}
		return m, m.pollCmd()
	case frameMsg:
		m.frame(time.Time(msg))
		return m, m.frameCmd()
	}
	return m, nil
}

// playSize is the pile area in terminal cells: everything but the HUD.
func (m *Model) playSize() (cols, rows int) {
	return m.width, max(0, m.height-hudRows)
}

func (m *Model) tooSmall() bool {
	cols, rows := m.playSize()
	return cols < 2*sprites.Claude.W || rows < sprites.Claude.H
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	if m.tooSmall() {
		return // keep the world as it was until there is room again
	}
	cols, rows := m.playSize()
	m.canvas = sprites.NewCanvas(cols, rows)
	if m.world != nil {
		m.world.Resize(cols, 2*rows)
		m.disturbed = true
		return
	}
	m.world = physics.NewWorld(cols, 2*rows)
	if m.initial > 0 {
		// A long history would bury the screen many times over; count the
		// piles it would have filled and only lay down the remainder.
		c := pileCapacity(cols, 2*rows)
		m.cleared, m.initial = m.initial/c, m.initial%c
		m.dropRows(m.initial, float64(2*rows))
		for range m.initial {
			m.addSkin()
		}
		m.world.Settle(settleSteps)
	}
	m.initial = -1
}

// pileCapacity estimates how many characters fit before the pile reaches
// the top. Random packing leaves plenty of gaps, so use a third of the area.
func pileCapacity(w, h int) int {
	return max(1, (w/sprites.Claude.W)*(h/sprites.Claude.H)/3)
}

// dropRows adds n bodies in loose rows stacked upward from bottom, with
// random gaps so the rows don't settle into a flat wall. It adds no skins.
func (m *Model) dropRows(n int, bottom float64) {
	ww, _ := m.world.Size()
	sw, sh := sprites.Claude.W, sprites.Claude.H
	perRow := max(1, ww/sw*2/3)
	for y := bottom - float64(sh); n > 0; y -= float64(sh + 1) {
		k := min(n, perRow)
		slack := float64(ww - k*sw)
		x := 0.0
		for range k {
			x += m.rng.Float64() * slack / float64(k)
			m.world.AddBody(x, y, sw, sh, 0)
			x += float64(sw)
		}
		n -= k
	}
}

func (m *Model) addBody(x, y, vx float64) {
	m.world.AddBody(x, y, sprites.Claude.W, sprites.Claude.H, vx)
	m.addSkin()
}

// addSkin tints a new skin for the next body.
func (m *Model) addSkin() {
	f := (m.rng.Float64()*2 - 1) * sprites.MaxVariation
	m.skins = append(m.skins, skin{sprites.Claude.Tinted(f), sprites.Teddy.Tinted(f)})
}

// spawnBody drops one new character from just above the top.
func (m *Model) spawnBody() {
	ww, _ := m.world.Size()
	x := m.rng.Float64() * float64(ww-sprites.Claude.W)
	m.addBody(x, -float64(sprites.Claude.H), (m.rng.Float64()*2-1)*spawnSpeed)
}

// replay drops the whole pile again from above the top, keeping each
// character's tint.
func (m *Model) replay() {
	if m.world == nil || m.squash != nil {
		return
	}
	m.world.Clear()
	m.dropRows(len(m.skins), 0)
}

// shake throws every character up and sideways. A pile that lands taller
// and reaches the top is cleared without counting, as after a resize.
func (m *Model) shake() {
	if m.world == nil || m.squash != nil {
		return
	}
	m.world.Shake(m.rng)
	m.disturbed = true
}

func (m *Model) frame(now time.Time) {
	dt := 0.0
	if !m.last.IsZero() {
		dt = min(now.Sub(m.last).Seconds(), maxFrameDT)
	}
	m.last = now
	if m.world == nil || m.tooSmall() {
		return
	}
	if m.squash != nil {
		m.stepSquash()
		return
	}
	for range m.spawn.release(dt) {
		m.spawnBody()
	}
	for m.acc += dt; m.acc >= physics.TimeStep; m.acc -= physics.TimeStep {
		m.world.Step(physics.TimeStep)
	}
	if top, ok := m.world.HighestRestingTop(); ok && top <= 0 {
		m.squash = &squash{scale: 1, counts: !m.disturbed}
	} else if m.world.Settled() {
		m.disturbed = false
	}
}

func (m *Model) stepSquash() {
	s := m.squash
	s.scale, s.vel = m.squashSpring.Update(s.scale, s.vel, 0)
	if math.Abs(s.scale) < 0.01 && math.Abs(s.vel) < 0.05 {
		m.world.Clear()
		m.skins = nil
		if s.counts {
			m.cleared++
		}
		m.squash = nil
		m.acc = 0
	}
}

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m *Model) render() string {
	if m.width == 0 {
		return ""
	}
	if m.tooSmall() {
		return "terminal too small for teddytokens"
	}
	m.canvas.Clear()
	_, floor := m.world.Size()
	for i, b := range m.world.Bodies() {
		x, y := b.Pos()
		if s := m.squash; s != nil {
			y = squashedTop(y, sprites.Claude.H, floor, s.scale)
		}
		sp := m.skins[i].claude
		if m.teddy {
			sp = m.skins[i].teddy
		}
		m.canvas.Blit(sp, x, y)
	}
	return m.canvas.Render() + "\n" + m.hud()
}

// squashedTop moves a body's top edge during the squash: its height above
// the floor shrinks with scale, and it sinks one body height into the floor
// as scale reaches 0, so the pile collapses and disappears.
func squashedTop(top, h, floor int, scale float64) int {
	bottom := float64(floor) - float64(floor-top-h)*scale
	return int(math.Round(bottom - float64(h)*scale))
}
