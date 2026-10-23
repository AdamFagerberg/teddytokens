// Package ui is the Bubble Tea front end: it turns token counts into falling
// characters, steps the physics world and draws the pile plus a status bar.
package ui

import (
	"math"
	"math/rand/v2"
	"slices"
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
	Fresh   bool   // start with an empty pile; Initial only shows in the HUD
	Per     int64  // tokens per character, > 0
	Teddy   bool   // start with the teddy skin
	Scope   string // label shown in the status bar
	Seed    uint64
}

// Character tiers. Three bigs merge into a giant.
const (
	small = iota
	big
	giant
)

// tierScale is each tier's sprite scale when the terminal is big enough.
var tierScale = [...]int{small: 1, big: 5, giant: 8}

// skin caches one character's tinted sprites so toggling never re-tints.
type skin struct{ claude, teddy *sprites.Sprite }

// char is a character on screen. Its body's size is its sprites' size.
type char struct {
	body    *physics.Body
	tier    int
	tint    float64
	skin    skin // tinted at the tier's current scale
	merging bool
}

func (c *char) size() (w, h int) {
	return c.skin.claude.W, c.skin.claude.H
}

func (c *char) centre() (x, y float64) {
	bx, by := c.body.Pos()
	w, h := c.size()
	return float64(bx) + float64(w)/2, float64(by) + float64(h)/2
}

// merge is the animation of every character of one tier flying together:
// progress springs from 0 to 1 while each moves from its body to (x, y),
// where they become one character of the next tier, or join the giant if
// into is set. Physics is paused meanwhile.
type merge struct {
	tier          int     // tier of the result
	into          *char   // the existing giant, or nil
	x, y          float64 // centre of the result
	progress, vel float64
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
	chars         []*char
	last          time.Time
	acc           float64 // unsimulated time, seconds

	merge       *merge
	mergeSpring harmonica.Spring
	giants      int // giants made, all drawn as the one on screen

	bar  progress.Model
	help help.Model
	err  error
}

// New builds the model. Nothing is drawn until the first WindowSizeMsg.
func New(cfg Config) *Model {
	m := &Model{
		src:         cfg.Source,
		rng:         rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9e3779b97f4a7c15)),
		scope:       cfg.Scope,
		teddy:       cfg.Teddy,
		total:       cfg.Initial,
		spawn:       spawner{per: cfg.Per},
		mergeSpring: harmonica.NewSpring(harmonica.FPS(fps), 7, 0.8),
		bar: progress.New(progress.WithWidth(12), progress.WithoutPercentage(),
			progress.WithColors(accent)),
		help: help.New(),
	}
	if !cfg.Fresh {
		m.spawn.add(cfg.Initial)
		m.initial, m.spawn.queue = m.spawn.queue, 0
	}
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
		// A merge's target is in the old world's coordinates; drop it and
		// let the next frame start again if the pile is still full.
		m.merge = nil
		for _, c := range m.chars {
			c.merging = false
		}
		m.world.Resize(cols, 2*rows)
		for _, c := range m.chars {
			m.fit(c)
		}
		return
	}
	m.world = physics.NewWorld(cols, 2*rows)
	if m.initial > 0 {
		// A long history would bury the screen many times over; lay down
		// the piles it would have filled as bigs and giants instead.
		cs, rem := m.pileUp(m.initial)
		for range rem {
			cs = append(cs, m.newChar(small))
		}
		m.dropRows(cs, float64(2*rows))
		m.chars = cs
		m.world.Settle(settleSteps)
	}
	m.initial = -1
}

// pileUp turns n characters into full piles and the rest: every pile is a
// big and every three bigs a giant. Giants are counted and only drawn once.
// It returns the new characters to drop, without bodies, and the rest.
func (m *Model) pileUp(n int) ([]*char, int) {
	c := pileCapacity(m.world.Size())
	piles := n / c
	m.giants += piles / 3
	var cs []*char
	if m.giants > 0 && !slices.ContainsFunc(m.chars, func(c *char) bool { return c.tier == giant }) {
		cs = append(cs, m.newChar(giant))
	}
	for range piles % 3 {
		cs = append(cs, m.newChar(big))
	}
	return cs, n % c
}

// scale is tierScale[tier], reduced until the sprite is no wider than the
// world and no taller than half of it.
func (m *Model) scale(tier int) int {
	ww, wh := m.world.Size()
	k := tierScale[tier]
	for k > 1 && (k*sprites.Claude.W > ww || 2*k*sprites.Claude.H > wh) {
		k--
	}
	return k
}

// newChar returns a character of tier with a random tint and no body.
func (m *Model) newChar(tier int) *char {
	c := &char{tier: tier, tint: (m.rng.Float64()*2 - 1) * sprites.MaxVariation}
	m.reskin(c)
	return c
}

// reskin tints c's sprites at its tier's scale for the current world.
func (m *Model) reskin(c *char) {
	k := m.scale(c.tier)
	c.skin = skin{sprites.Claude.Tinted(c.tint).Scaled(k), sprites.Teddy.Tinted(c.tint).Scaled(k)}
}

// fit gives c a new body if the world's size changed its tier's scale.
func (m *Model) fit(c *char) {
	if c.skin.claude.W == m.scale(c.tier)*sprites.Claude.W {
		return
	}
	x, y := c.centre()
	m.world.Remove(c.body)
	m.reskin(c)
	m.place(c, x, y)
}

// place adds a body for c centred on (x, y), moved inside the walls and up
// above every other character in its columns so it overlaps nothing.
func (m *Model) place(c *char, x, y float64) {
	ww, _ := m.world.Size()
	w, h := c.size()
	left := max(0, min(ww-w, int(math.Round(x-float64(w)/2))))
	top := y - float64(h)/2
	for _, o := range m.chars {
		ox, oy := o.body.Pos()
		if ow, _ := o.size(); o != c && ox < left+w && left < ox+ow {
			top = min(top, float64(oy-h))
		}
	}
	c.body = m.world.AddBody(float64(left), top, w, h, 0)
}

// pileCapacity estimates how many characters fit before the pile reaches
// the top. Random packing leaves plenty of gaps, so use a third of the area.
func pileCapacity(w, h int) int {
	return max(1, (w/sprites.Claude.W)*(h/sprites.Claude.H)/3)
}

// dropRows adds bodies for cs in loose rows stacked upward from bottom,
// with random gaps so the rows don't settle into a flat wall.
func (m *Model) dropRows(cs []*char, bottom float64) {
	ww, _ := m.world.Size()
	for len(cs) > 0 {
		k, used, rowH := 0, 0, 0
		for ; k < len(cs); k++ {
			w, h := cs[k].size()
			if k > 0 && used+w > ww*2/3 {
				break
			}
			used, rowH = used+w, max(rowH, h)
		}
		slack := float64(ww - used)
		x := 0.0
		for _, c := range cs[:k] {
			w, h := c.size()
			x += m.rng.Float64() * slack / float64(k)
			c.body = m.world.AddBody(x, bottom-float64(h), w, h, 0)
			x += float64(w)
		}
		cs, bottom = cs[k:], bottom-float64(rowH+1)
	}
}

// spawnBody drops one new small character from just above the top.
func (m *Model) spawnBody() {
	ww, _ := m.world.Size()
	c := m.newChar(small)
	w, h := c.size()
	x := m.rng.Float64() * float64(ww-w)
	c.body = m.world.AddBody(x, -float64(h), w, h, (m.rng.Float64()*2-1)*spawnSpeed)
	m.chars = append(m.chars, c)
}

// replay drops every character again from above the top, keeping its tint.
func (m *Model) replay() {
	if m.world == nil || m.merge != nil {
		return
	}
	m.world.Clear()
	m.dropRows(m.chars, 0)
}

// shake throws every character up and sideways.
func (m *Model) shake() {
	if m.world == nil || m.merge != nil {
		return
	}
	m.world.Shake(m.rng)
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
	if m.merge != nil {
		m.stepMerge()
		return
	}
	// More than a pile's worth would rain for minutes; turn the full piles
	// it would fill into bigs and giants, as at startup, and rain the rest.
	if m.spawn.queue > pileCapacity(m.world.Size()) {
		var cs []*char
		cs, m.spawn.queue = m.pileUp(m.spawn.queue)
		m.dropRows(cs, 0)
		m.chars = append(m.chars, cs...)
	}
	for range m.spawn.release(dt) {
		m.spawnBody()
	}
	for m.acc += dt; m.acc >= physics.TimeStep; m.acc -= physics.TimeStep {
		m.world.Step(physics.TimeStep)
	}
	m.checkMerge()
}

// checkMerge starts a merge when three bigs exist or the resting pile has
// reached the top: the smalls merge into a big or, with no smalls left, the
// bigs merge into the giant early.
func (m *Model) checkMerge() {
	var n [len(tierScale)]int
	for _, c := range m.chars {
		n[c.tier]++
	}
	top, ok := m.world.HighestRestingTop()
	full := ok && top <= 0
	switch {
	case n[big] >= 3:
		m.startMerge(big, 3)
	case full && n[small] > 0:
		m.startMerge(small, n[small])
	case full && n[big] > 0:
		m.startMerge(big, n[big])
	}
}

// startMerge flies the first n characters of tier from towards the centre
// of their bounding box, or towards the giant when making one that exists.
func (m *Model) startMerge(from, n int) {
	g := &merge{tier: from + 1}
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, c := range m.chars {
		switch {
		case c.tier == from && n > 0:
			n--
			c.merging = true
			x, y := c.body.Pos()
			w, h := c.size()
			x0, y0 = min(x0, float64(x)), min(y0, float64(y))
			x1, y1 = max(x1, float64(x+w)), max(y1, float64(y+h))
		case c.tier == giant && g.tier == giant:
			g.into = c
		}
	}
	g.x, g.y = (x0+x1)/2, (y0+y1)/2
	if g.into != nil {
		g.x, g.y = g.into.centre()
	}
	m.merge = g
}

// stepMerge advances the merge and, once it has converged, replaces the
// merging characters with their result.
func (m *Model) stepMerge() {
	g := m.merge
	g.progress, g.vel = m.mergeSpring.Update(g.progress, g.vel, 1)
	if math.Abs(1-g.progress) > 0.01 || math.Abs(g.vel) > 0.05 {
		return
	}
	for _, c := range m.chars {
		if c.merging {
			m.world.Remove(c.body)
		}
	}
	m.chars = slices.DeleteFunc(m.chars, func(c *char) bool { return c.merging })
	if g.tier == giant {
		m.giants++
	}
	if g.into == nil {
		c := m.newChar(g.tier)
		m.place(c, g.x, g.y)
		m.chars = append(m.chars, c)
	}
	m.merge = nil
	m.acc = 0
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
	for _, c := range m.chars {
		x, y := c.body.Pos()
		if g := m.merge; c.merging {
			cx, cy := c.centre()
			x += int(math.Round((g.x - cx) * g.progress))
			y += int(math.Round((g.y - cy) * g.progress))
		}
		sp := c.skin.claude
		if m.teddy {
			sp = c.skin.teddy
		}
		m.canvas.Blit(sp, x, y)
	}
	return m.canvas.Render() + "\n" + m.hud()
}
