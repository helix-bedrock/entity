package behavior

import (
	"math"
	"slices"
)

// The look goals' fixed numbers.
const (
	lookYaw        = 10
	fullView       = 359 // an angle_of_view above this skips the view test
	degPerRad      = 57.295776
	radPerDeg      = 0.017453292
	sinScale       = 10430.378
	sinQuarter     = 16384
	randomLookTurn = 180
)

// Filter is a JSON filter test with the goal's mob as self and the
// candidate as other. A nil Filter passes everything.
type Filter func(self Mob, other Actor) bool

// LookAtConfig is the JSON of minecraft:behavior.look_at_player and
// look_at_entity.
type LookAtConfig struct {
	LookDistance float32 `json:"look_distance"`
	Probability  float32 `json:"probability"`
	// AngleVertical and AngleHorizontal are angle_of_view_vertical and
	// angle_of_view_horizontal in degrees.
	AngleVertical, AngleHorizontal int32
	// LookTime is in ticks.
	LookTime Range[int32]
	Filters  Filter
}

// DefaultLookAt is the goal's BDS defaults.
var DefaultLookAt = LookAtConfig{LookDistance: 8, Probability: 0.02, AngleVertical: 360, AngleHorizontal: 360, LookTime: Range[int32]{40, 80}}

// LookAt is LookAtActorGoal: now and then the mob looks at a nearby entity
// in its view.
type LookAt struct {
	base
	cfg    LookAtConfig
	target ID
	timer  int32
	buf    []Actor
	sorted []near
}

// NewLookAt makes the goal.
func NewLookAt(w World, m Mob, c LookAtConfig) *LookAt {
	return &LookAt{base: base{mob: m, world: w, flags: flagsLook}, cfg: c, target: NoID}
}

// near is one entity of a sorted nearby query.
type near struct {
	a  Actor
	d2 float32
}

// sortedNear is BDS's nearby query (0xB30BE90): the entities in the box
// grown by d, nearest first. It reuses dst and buf.
func sortedNear(w World, m Mob, d float32, dst []Actor, buf []near) ([]Actor, []near) {
	b := m.Box()
	b.Min = Vec3{X: b.Min.X - d, Y: b.Min.Y - d, Z: b.Min.Z - d}
	b.Max = Vec3{X: b.Max.X + d, Y: b.Max.Y + d, Z: b.Max.Z + d}
	dst = w.Entities(dst[:0], m.ID(), b)
	p := m.Pos()
	buf = buf[:0]
	for _, a := range dst {
		buf = append(buf, near{a: a, d2: dist2(p, a.Pos())})
	}
	sortNear(buf)
	for i := range buf {
		dst[i] = buf[i].a
	}
	clear(buf)
	return dst, buf
}

// sortNear sorts nearest first.
func sortNear(s []near) {
	slices.SortStableFunc(s, func(x, y near) int {
		switch {
		case x.d2 < y.d2:
			return -1
		case x.d2 > y.d2:
			return 1
		}
		return 0
	})
}

// wrap180 folds an angle into [0, 180] the way the view test does.
func wrap180(a float32) float32 {
	v := float32(math.Mod(float64(a), 360))
	if v < 0 {
		v += 360
	}
	return abs32(v - 180)
}

// inView is the angle_of_view test from the mob's eyes to the entity's.
func inView(m Mob, a Actor, horizontal, vertical int32) bool {
	if horizontal > fullView && vertical > fullView {
		return true
	}
	e, t := m.Eyes(), a.Eyes()
	dx, dy, dz := t.X-e.X, t.Y-e.Y, t.Z-e.Z
	flat := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	yaw := float32(math.Atan2(float64(dz), float64(dx)))*degPerRad - 90 - m.BodyYaw()
	pitch := float32(math.Atan2(float64(dy), float64(flat))) * degPerRad
	return float32(horizontal) >= wrap180(yaw+180) && float32(vertical) >= wrap180(180-pitch)
}

// rides reports that b is one of a's riders.
func rides(a Actor, b ID) bool { return slices.Contains(a.Riders(), b) }

// CanUse implements goal.Goal.
func (g *LookAt) CanUse() bool {
	if g.world.IRandom().NextFloat() >= g.cfg.Probability {
		return false
	}
	g.buf, g.sorted = sortedNear(g.world, g.mob, g.cfg.LookDistance, g.buf, g.sorted)
	defer clear(g.buf)
	for _, a := range g.buf {
		if a == nil || a.Removed() || !a.Pickable() {
			continue
		}
		if g.cfg.Filters != nil && !g.cfg.Filters(g.mob, a) {
			continue
		}
		if !inView(g.mob, a, g.cfg.AngleHorizontal, g.cfg.AngleVertical) {
			continue
		}
		if rides(g.mob, a.ID()) || rides(a, g.mob.ID()) {
			continue
		}
		g.target = a.ID()
		break
	}
	t := g.actor(g.target)
	return t != nil && !t.Removed()
}

// CanContinueToUse implements goal.Goal.
func (g *LookAt) CanContinueToUse() bool {
	t := g.actor(g.target)
	if t == nil || !t.Alive() || !t.Pickable() {
		return false
	}
	if dist2(g.mob.Pos(), t.Pos()) > g.cfg.LookDistance*g.cfg.LookDistance {
		return false
	}
	return inView(g.mob, t, g.cfg.AngleHorizontal, g.cfg.AngleVertical) && g.timer > 0
}

// Start implements goal.Goal.
func (g *LookAt) Start() {
	g.timer = randomInt(g.world.IRandom(), g.cfg.LookTime.Min, g.cfg.LookTime.Max)
}

// Stop implements goal.Goal.
func (g *LookAt) Stop() { g.target = NoID }

// Tick implements goal.Goal.
func (g *LookAt) Tick() {
	if t := g.actor(g.target); t != nil && !g.mob.LookBusy() {
		g.mob.LookAt(t.Eyes(), lookYaw, g.mob.MaxHeadXRot())
	}
	g.timer--
}

// RandomLookConfig is the JSON of minecraft:behavior.random_look_around.
type RandomLookConfig struct {
	// LookTime is in ticks.
	LookTime Range[int32]
	// Angle is min_angle_of_view_horizontal and max_angle_of_view_horizontal.
	Angle       Range[float32]
	Probability float32 `json:"probability"`
}

// DefaultRandomLook is the goal's BDS defaults.
var DefaultRandomLook = RandomLookConfig{LookTime: Range[int32]{20, 40}, Angle: Range[float32]{-180, 180}, Probability: 0.02}

// RandomLook is RandomLookAroundGoal: the mob turns its head to a random
// direction for a while.
type RandomLook struct {
	base
	cfg   RandomLookConfig
	yaw   float32
	timer int32
}

// NewRandomLook makes the goal.
func NewRandomLook(w World, m Mob, c RandomLookConfig) *RandomLook {
	return &RandomLook{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c}
}

// CanUse implements goal.Goal.
func (g *RandomLook) CanUse() bool { return g.cfg.Probability > g.world.IRandom().NextFloat() }

// CanContinueToUse implements goal.Goal.
func (g *RandomLook) CanContinueToUse() bool { return g.timer >= 0 }

// Start implements goal.Goal.
func (g *RandomLook) Start() {
	r := g.world.IRandom()
	g.yaw = (g.cfg.Angle.Max-g.cfg.Angle.Min)*r.NextFloat() + g.cfg.Angle.Min + g.mob.BodyYaw()
	g.timer = randomInt(r, g.cfg.LookTime.Min, g.cfg.LookTime.Max)
}

// Stop implements goal.Goal.
func (g *RandomLook) Stop() {}

// Tick implements goal.Goal.
func (g *RandomLook) Tick() {
	g.timer--
	if g.mob.LookBusy() {
		return
	}
	a := (g.yaw*-radPerDeg - math.Pi) * sinScale
	e := g.mob.Eyes()
	t := Vec3{X: e.X - tableSin(uint16(int32(a))), Y: e.Y, Z: e.Z - tableSin(uint16(int32(a+sinQuarter)))}
	g.mob.LookAt(t, randomLookTurn, randomLookTurn)
}

// tableSin is entry i of BDS's 65536-entry sine table.
func tableSin(i uint16) float32 { return float32(math.Sin(float64(i) * math.Pi * 2 / 65536)) }

// RandomLookSitConfig is the JSON of
// minecraft:behavior.random_look_around_and_sit.
type RandomLookSitConfig struct {
	RandomLookConfig
	// Looks is min_look_count and max_look_count.
	Looks Range[int32]
	// Cooldown is random_look_around_cooldown in seconds.
	Cooldown              int32
	ContinueIfLeashed     bool `json:"continue_if_leashed"`
	ContinueSittingReload bool `json:"continue_sitting_on_reload"`
}

// DefaultRandomLookSit is the goal's BDS defaults.
var DefaultRandomLookSit = RandomLookSitConfig{RandomLookConfig: DefaultRandomLook, Looks: Range[int32]{1, 2}}

// RandomLookSit is RandomLookAroundAndSitGoal: the mob sits down and looks
// around a few times, then waits out a cooldown.
type RandomLookSit struct {
	RandomLook
	sit   RandomLookSitConfig
	looks int32
	next  uint64
}

// NewRandomLookSit makes the goal.
func NewRandomLookSit(w World, m Mob, c RandomLookSitConfig) *RandomLookSit {
	return &RandomLookSit{RandomLook: *NewRandomLook(w, m, c.RandomLookConfig), sit: c}
}

// CanUse implements goal.Goal. A mob that sat down before a reload keeps
// sitting when continue_sitting_on_reload is set.
func (g *RandomLookSit) CanUse() bool {
	m := g.mob
	if g.cfg.Probability > g.world.IRandom().NextFloat() && !m.InWater() && g.actor(m.Target()) == nil &&
		!m.Flag(FlagLeashed) && !m.Flag(FlagSitting) && g.world.CurrentTick() > g.next {
		return true
	}
	return g.sit.ContinueSittingReload && m.Flag(FlagSitting)
}

// CanContinueToUse implements goal.Goal.
func (g *RandomLookSit) CanContinueToUse() bool {
	m := g.mob
	if m.Flag(FlagLeashed) {
		return g.sit.ContinueIfLeashed && !m.LeashStretched()
	}
	return g.looks > 0 && !m.InWater() && g.actor(m.Target()) == nil
}

// Start implements goal.Goal.
func (g *RandomLookSit) Start() {
	g.RandomLook.Start()
	r := g.world.IRandom()
	g.looks = randomInt(r, g.sit.Looks.Min, g.sit.Looks.Max)
	g.mob.SetFlag(FlagSitting, true)
	if g.sit.ContinueSittingReload {
		g.mob.SetPersistSit(true)
	}
}

// Stop implements goal.Goal.
func (g *RandomLookSit) Stop() {
	if g.sit.ContinueSittingReload {
		g.mob.SetPersistSit(false)
	}
	g.mob.SetFlag(FlagSitting, false)
	g.next = g.world.CurrentTick() + uint64(ticksPerSecond*g.sit.Cooldown)
}

// Tick implements goal.Goal: each finished look starts the next one.
func (g *RandomLookSit) Tick() {
	g.RandomLook.Tick()
	if g.timer > 0 {
		return
	}
	g.looks--
	r := g.world.IRandom()
	g.yaw = (g.cfg.Angle.Max-g.cfg.Angle.Min)*r.NextFloat() + g.cfg.Angle.Min + g.mob.BodyYaw()
	g.timer = randomInt(r, g.cfg.LookTime.Min, g.cfg.LookTime.Max)
}
