package behavior

import "sort"

// The warden goals' actor flags, by BDS's actor flag number.
const (
	FlagCroaking  = 101
	FlagEmerging  = 104
	FlagSniffing  = 105
	FlagDigging   = 106
	FlagSonicBoom = 107
)

// The BDS actor types the warden goals look for (ActorType values).
const (
	actorTypeMob    = 256 // any mob, players too
	actorTypePlayer = 319
)

// Anger names the AngerLevelComponent value a goal raises a suspect's
// anger by.
type Anger uint8

// The anger amounts the warden goals use.
const (
	// AngerAnnoyingness is the component's default_annoyingness.
	AngerAnnoyingness Anger = iota
	// AngerBoost is the component's angry_boost.
	AngerBoost
)

// Warden is the part of the mob the warden goals need on top of Mob: the
// anger_level and suspect_tracking components.
type Warden interface {
	Mob
	// Suspects appends the anger_level suspects to dst in the component's
	// map order (by unique ID); none without the component.
	Suspects(dst []ID) []ID
	// ValidSuspect is the anger_level suspect test (alive, not removed,
	// the nuisance filter and the like); true without the component.
	ValidSuspect(a Actor) bool
	// Calm reports that the entity is not a suspect or its anger is below
	// angry_threshold.
	Calm(id ID) bool
	// AddAnger raises the entity's anger by the component's amount, playing
	// the on_increase_sounds when sound is set; nothing without the
	// component.
	AddAnger(a Actor, by Anger, sound bool)
	// Suspicion is the suspect_tracking location; ok is false without the
	// component or while no location is set.
	Suspicion() (p Pos, ok bool)
	// SuspicionTick is the tick the location was last set; ok is false
	// without the component or before one was set.
	SuspicionTick() (tick uint64, ok bool)
	// SetSuspicion sets the location and its tick; nothing without the
	// component.
	SetSuspicion(p Pos, tick uint64)
	// ClearSuspicion unsets the location and keeps its tick.
	ClearSuspicion()
	// EntitiesOfType appends the entities of the BDS actor type whose boxes
	// touch b to dst, excluding self.
	EntitiesOfType(dst []Actor, actorType int, self ID, b Box) []Actor
}

// sniffTarget is one entity found by nearestSuspect, with its squared
// distance.
type sniffTarget struct {
	a  Actor
	d2 float32
}

// suspectFinder is the shared search of sniff (0xBDCC790): the nearest
// valid suspect of a type within a box grown by r, players first.
type suspectFinder struct {
	near  []Actor
	found []sniffTarget
}

// nearest returns the nearest valid player, else the nearest valid mob.
func (f *suspectFinder) nearest(m Warden, r float32) Actor {
	for _, t := range [...]int{actorTypePlayer, actorTypeMob} {
		if a := f.nearestOf(m, t, r); a != nil {
			return a
		}
	}
	return nil
}

// nearestOf is 0xBDCCD20 on top of 0xB30BE90: entities sorted by squared
// distance from the mob, then the invalid suspects dropped.
func (f *suspectFinder) nearestOf(m Warden, actorType int, r float32) Actor {
	b := m.Box()
	box := sortedBox(sub(b.Min, Vec3{X: r, Y: r, Z: r}), add(b.Max, Vec3{X: r, Y: r, Z: r}))
	f.near = m.EntitiesOfType(f.near[:0], actorType, m.ID(), box)
	p := m.Pos()
	f.found = f.found[:0]
	for _, a := range f.near {
		f.found = append(f.found, sniffTarget{a: a, d2: dist2(p, a.Pos())})
	}
	sort.SliceStable(f.found, func(i, j int) bool { return f.found[i].d2 < f.found[j].d2 })
	for _, t := range f.found {
		if m.ValidSuspect(t.a) {
			return t.a
		}
	}
	return nil
}

// withinBox is 0xB2E2660: the other entity is closer than h on the flat
// and closer than v up or down.
func withinBox(a, b Vec3, h, v float32) bool {
	dx, dz := float64(b.X-a.X), float64(b.Z-a.Z)
	dy := b.Y - a.Y
	return dx*dx+dz*dz < float64(h*h) && dy*dy < v*v
}

// targetGone reports that the mob has no target or its target cannot be
// fetched, the start test of roar, sniff and investigate.
func targetGone(b *base) bool {
	return b.actor(b.mob.Target()) == nil
}

// EmergeConfig is the JSON of minecraft:behavior.emerge. Times are in
// ticks (BDS rounds the seconds times 20).
type EmergeConfig struct {
	Duration int32
	Cooldown int32
	OnDone   string
}

// DefaultEmerge returns the goal's BDS defaults: duration 5 s, cooldown_time
// 0.5 s.
func DefaultEmerge() EmergeConfig {
	return EmergeConfig{Duration: 100, Cooldown: 10}
}

// Emerge is EmergeGoal: the warden climbs out of the ground.
type Emerge struct {
	base
	cfg          EmergeConfig
	ready, until uint64
}

// NewEmerge makes the goal.
func NewEmerge(w World, m Mob, c EmergeConfig) *Emerge {
	return &Emerge{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c}
}

// CanUse implements goal.Goal (0xBDFB550).
func (g *Emerge) CanUse() bool { return g.world.CurrentTick() >= g.ready }

// CanContinueToUse implements goal.Goal (0xBDFB580).
func (g *Emerge) CanContinueToUse() bool { return g.world.CurrentTick() <= g.until }

// Start implements goal.Goal (0xBDFB5B0).
func (g *Emerge) Start() {
	g.until = g.world.CurrentTick() + uint64(uint16(g.cfg.Duration))
	g.mob.SetFlag(FlagEmerging, true)
}

// Stop implements goal.Goal (0xBDFB600).
func (g *Emerge) Stop() {
	if g.cfg.OnDone != "" {
		g.mob.Event(g.cfg.OnDone, NoID)
	}
	g.ready = g.world.CurrentTick() + uint64(uint16(g.cfg.Cooldown))
	g.mob.SetFlag(FlagEmerging, false)
}

// Tick implements goal.Goal.
func (g *Emerge) Tick() {}

// digLiquidSound is the level sound a warden plays when it digs down in
// water or lava (the warden's agitated sound).
const digLiquidSound = 370

// Digger is the part of the mob dig needs on top of Warden.
type Digger interface {
	Warden
	// Named reports a name tag (actor data item 4 not empty).
	Named() bool
	// InDaylight is Actor::isInDaylight (0xB2E0FE0).
	InDaylight() bool
	// Remove removes the actor from the level (Actor vtable +96).
	Remove()
}

// DigConfig is the JSON of minecraft:behavior.dig. Times are in ticks.
type DigConfig struct {
	Duration               int32
	IdleTime               int32
	AllowDigWhenNamed      bool `json:"allow_dig_when_named"`
	VibrationIsDisturbance bool `json:"vibration_is_disturbance"`
	SuspicionIsDisturbance bool `json:"suspicion_is_disturbance"`
	DigsInDaylight         bool `json:"digs_in_daylight"`
	OnStart                string
}

// DefaultDig returns the goal's BDS defaults: every value zero or off.
func DefaultDig() DigConfig {
	return DigConfig{}
}

// Dig is DigGoal: the warden digs back into the ground and is removed.
type Dig struct {
	base
	m   Digger
	cfg DigConfig
	// made is the tick the goal was made; idle_time counts from it.
	made, until uint64
}

// NewDig makes the goal; idle_time counts from now.
func NewDig(w World, m Digger, c DigConfig) *Dig {
	return &Dig{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, made: w.CurrentTick()}
}

// CanUse implements goal.Goal (0xBDFA6C0).
func (g *Dig) CanUse() bool {
	m := g.m
	if m.Flag(FlagDigging) {
		return false
	}
	if m.Named() && !g.cfg.AllowDigWhenNamed {
		return false
	}
	if !m.OnGround() && !m.InWater() && !m.InLava() {
		return false
	}
	if g.cfg.DigsInDaylight && m.InDaylight() {
		return true
	}
	now := g.world.CurrentTick()
	idle := uint64(uint32(g.cfg.IdleTime))
	if g.cfg.VibrationIsDisturbance {
		if t, ok := m.HeardVibration(); ok && now-t < idle {
			return false
		}
	}
	if g.cfg.SuspicionIsDisturbance {
		if t, ok := m.SuspicionTick(); ok && now-t < idle {
			return false
		}
	}
	return now-g.made >= idle
}

// CanContinueToUse implements goal.Goal (0xBDFAAE0).
func (g *Dig) CanContinueToUse() bool {
	return g.world.CurrentTick() <= g.until && !g.m.Removed()
}

// Start implements goal.Goal (0xBDFAB20): in water or lava the warden is
// removed at once.
func (g *Dig) Start() {
	g.until = g.world.CurrentTick() + uint64(uint16(g.cfg.Duration))
	m := g.m
	if m.InWater() || m.InLava() {
		m.PlaySoundAt(digLiquidSound, m.AttachPos(attachHead))
		m.Remove()
		return
	}
	m.SetFlag(FlagDigging, true)
	if g.cfg.OnStart != "" {
		m.Event(g.cfg.OnStart, NoID)
	}
}

// Stop implements goal.Goal (0xBDFACB0).
func (g *Dig) Stop() { g.m.Remove() }

// Tick implements goal.Goal.
func (g *Dig) Tick() {}

// The roar's look speeds (LookControl +32 is 10).
const wardenLookYaw = 10

// RoarConfig is the JSON of minecraft:behavior.roar. Duration is in ticks.
type RoarConfig struct {
	Duration int32
}

// DefaultRoar returns the goal's BDS defaults: duration 0.
func DefaultRoar() RoarConfig {
	return RoarConfig{}
}

// Roar is RoarGoal: the warden roars at the angriest suspect, then makes
// it its target.
type Roar struct {
	base
	m       Warden
	cfg     RoarConfig
	at      ID
	until   uint64
	suspect []ID
}

// NewRoar makes the goal.
func NewRoar(w World, m Warden, c RoarConfig) *Roar {
	return &Roar{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, at: NoID}
}

// CanUse implements goal.Goal (0xBDCAA50): the first valid suspect, when
// the warden is angry at it.
func (g *Roar) CanUse() bool {
	if !targetGone(&g.base) {
		return false
	}
	g.suspect = g.m.Suspects(g.suspect[:0])
	for _, id := range g.suspect {
		a := g.actor(id)
		if a == nil || !g.m.ValidSuspect(a) {
			continue
		}
		if g.m.Calm(a.ID()) {
			return false
		}
		g.at = a.ID()
		return true
	}
	return false
}

// CanContinueToUse implements goal.Goal (0xBDCACD0).
func (g *Roar) CanContinueToUse() bool {
	return targetGone(&g.base) && g.world.CurrentTick() <= g.until
}

// Start implements goal.Goal (0xBDCAD30).
func (g *Roar) Start() {
	g.until = g.world.CurrentTick() + uint64(uint16(g.cfg.Duration))
	g.m.SetFlag(FlagRoaring, true)
}

// Stop implements goal.Goal (0xBDCAFF0).
func (g *Roar) Stop() {
	g.m.SetFlag(FlagRoaring, false)
	a := g.actor(g.at)
	if a == nil || a.Removed() {
		return
	}
	g.m.SetTarget(a.ID())
	g.m.AddAnger(a, AngerBoost, false)
}

// Tick implements goal.Goal (0xBDCAD70).
func (g *Roar) Tick() {
	if a := g.actor(g.at); a != nil {
		g.m.LookAt(a.AttachPos(attachHead), wardenLookYaw, g.m.MaxHeadXRot())
	}
}

// SniffConfig is the JSON of minecraft:behavior.sniff. Duration is in
// ticks; the cooldown is in seconds, drawn each time.
type SniffConfig struct {
	Duration                  int32
	SniffingRadius            float32 `json:"sniffing_radius"`
	SuspicionRadiusHorizontal float32 `json:"suspicion_radius_horizontal"`
	SuspicionRadiusVertical   float32 `json:"suspicion_radius_vertical"`
	Cooldown                  Range[float32]
}

// DefaultSniff returns the goal's BDS defaults: duration 1 s, radius 5,
// suspicion radii 3 and 3, cooldown 3 to 10 s.
func DefaultSniff() SniffConfig {
	return SniffConfig{
		Duration: 20, SniffingRadius: 5, SuspicionRadiusHorizontal: 3, SuspicionRadiusVertical: 3,
		Cooldown: Range[float32]{Min: 3, Max: 10},
	}
}

// Sniff is SniffGoal: the warden sniffs for the nearest suspect and marks
// where it is, getting angrier when it is close.
type Sniff struct {
	base
	m            Warden
	cfg          SniffConfig
	ready, until uint64
	find         suspectFinder
}

// NewSniff makes the goal.
func NewSniff(w World, m Warden, c SniffConfig) *Sniff {
	return &Sniff{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0xBDCC630).
func (g *Sniff) CanUse() bool {
	if !targetGone(&g.base) || g.world.CurrentTick() < g.ready {
		return false
	}
	if _, ok := g.m.Suspicion(); ok {
		return false
	}
	return g.find.nearest(g.m, g.cfg.SniffingRadius) != nil
}

// CanContinueToUse implements goal.Goal (0xBDCC860).
func (g *Sniff) CanContinueToUse() bool { return g.world.CurrentTick() <= g.until }

// Start implements goal.Goal (0xBDCC890).
func (g *Sniff) Start() {
	g.until = g.world.CurrentTick() + uint64(uint16(g.cfg.Duration))
	g.m.SetFlag(FlagSniffing, true)
}

// Stop implements goal.Goal (0xBDCC8E0).
func (g *Sniff) Stop() {
	if a := g.find.nearest(g.m, g.cfg.SniffingRadius); a != nil {
		g.m.SetSuspicion(blockOf(a.Pos()), g.world.CurrentTick())
		if withinBox(g.m.Pos(), a.Pos(), g.cfg.SuspicionRadiusHorizontal, g.cfg.SuspicionRadiusVertical) {
			g.m.AddAnger(a, AngerAnnoyingness, true)
		}
	}
	g.m.SetFlag(FlagSniffing, false)
	g.ready = g.world.CurrentTick() + uint64(uint16(randomTicks(g.world.Random(), g.cfg.Cooldown)))
}

// Tick implements goal.Goal.
func (g *Sniff) Tick() {}

// InvestigateConfig is the JSON of
// minecraft:behavior.investigate_suspicious_location.
type InvestigateConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	GoalRadius      float32 `json:"goal_radius"`
}

// DefaultInvestigate returns the goal's BDS defaults.
func DefaultInvestigate() InvestigateConfig {
	return InvestigateConfig{SpeedMultiplier: 1, GoalRadius: 1.5}
}

// Investigate is InvestigateSuspiciousLocationGoal: the warden walks to
// the suspect_tracking location.
type Investigate struct {
	base
	m       Warden
	cfg     InvestigateConfig
	radius2 float32
	at      Vec3
	end     Pos
	failed  bool
	reached bool
}

// NewInvestigate makes the goal; BDS keeps goal_radius squared.
func NewInvestigate(w World, m Warden, c InvestigateConfig) *Investigate {
	return &Investigate{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, radius2: c.GoalRadius * c.GoalRadius}
}

// centre2 is the squared distance from the mob to the middle of block p.
func (g *Investigate) centre2(p Pos) float32 {
	return dist2(Vec3{X: float32(p.X) + 0.5, Y: float32(p.Y) + 0.5, Z: float32(p.Z) + 0.5}, g.m.Pos())
}

// CanUse implements goal.Goal (0xBDFC820): a suspicious location further
// away than goal_radius (0xBDFC980).
func (g *Investigate) CanUse() bool {
	if !targetGone(&g.base) || g.m.Nav() == nil || g.m.Vehicle() != NoID {
		return false
	}
	p, ok := g.m.Suspicion()
	if !ok || g.centre2(p) <= g.radius2 {
		return false
	}
	g.at = Vec3{X: float32(p.X), Y: float32(p.Y), Z: float32(p.Z)}
	return true
}

// same reports that the tracked location is still the one being walked to.
func (g *Investigate) same() bool {
	p, ok := g.m.Suspicion()
	return ok && p == blockOf(g.at)
}

// CanContinueToUse implements goal.Goal (0xBDFCB20).
func (g *Investigate) CanContinueToUse() bool {
	return targetGone(&g.base) && g.same() && !g.failed && !g.reached
}

// Start implements goal.Goal (0xBDFCCF0).
func (g *Investigate) Start() {
	n := g.m.Nav()
	if n == nil || !n.MoveTo(g.at, g.cfg.SpeedMultiplier) {
		g.failed = true
		return
	}
	if end, ok := n.PathEnd(); ok {
		g.end = end
	}
}

// Stop implements goal.Goal (0xBDFD050).
func (g *Investigate) Stop() {
	if g.same() {
		g.m.ClearSuspicion()
	}
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
	g.failed, g.reached = false, false
}

// Tick implements goal.Goal (0xBDFCE50): once the path is done, either
// close enough or walk to the path's end again.
func (g *Investigate) Tick() {
	n := g.m.Nav()
	if n != nil && !n.Done() {
		return
	}
	if g.failed {
		return
	}
	switch {
	case g.centre2(g.end) <= g.radius2:
		g.reached = true
	case n == nil || !n.MoveTo(Vec3{X: float32(g.end.X), Y: float32(g.end.Y), Z: float32(g.end.Z)}, g.cfg.SpeedMultiplier):
		g.failed = true
	}
}
