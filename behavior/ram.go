package behavior

import "math"

// The ram_attack goal's fixed numbers.
const (
	ramPathTime     = 160 // ticks the run-up may take
	ramWindUp       = 20
	ramTime         = 140
	ramSettle       = 2
	ramAtStart2     = 0.1024 // 0.32 blocks, squared and flat
	ramAtStartY     = 0.5
	ramFaced        = 1 // degrees off the ram line that count as facing it
	ramTargetStill  = 1 // the target must stay within a block of the spot
	ramEndRadius2   = 0.01
	ramSlowDist2    = 4
	ramSlowSpeed    = 0.75
	ramArrived      = 0.7
	ramReachAhead   = 1.1 // the hit box reaches this far along the ram
	ramSpeedDiv     = 12  // metres per second per unit of knockback
	ramMinStrength  = 0.2
	ramMaxStrength  = 3
	ramShieldFactor = 0.4
	ramKnockPower   = 0.4
	ramKnockCap     = 3
	ramLookYaw      = 10
	ramLookPitch    = 80
	ramBabyMinKnock = 1
	ramDamageCause  = 30
	ramDirections   = 4
	ramDirMinusZ    = 0
	ramDirPlusZ     = 1
	ramDirMinusX    = 2
	ramDirPlusX     = 3
	ramBlockCentre  = 0.5
)

// ramState is RamAttackGoal's state.
type ramState uint8

const (
	ramToStart ramState = iota
	ramTurning
	ramChecking
	ramWindingUp
	ramRamming
	ramSettling
	ramDone
)

// Rammer is the part of the mob ram_attack needs on top of Targeter.
type Rammer interface {
	Targeter
	MoveController
	Knocker
	// HasLookControl reports LookControlComponent.
	HasLookControl() bool
	// ResetLook drops the look control's target (LookControl::initialize).
	ResetLook()
	// RamHit hurts the entity with the ram's damage cause (30).
	RamHit(a Actor)
	// RamBlockAhead, RamImpact and RamReset are the goat horn hooks BDS
	// runs for goats only: note the block past the target, drop a horn
	// when the ram ends against it, and forget the block.
	RamBlockAhead(p Vec3)
	RamImpact(p Vec3)
	RamReset()
}

// RamAttackConfig is the JSON of minecraft:behavior.ram_attack.
type RamAttackConfig struct {
	RamDistance     int32   `json:"ram_distance"`
	MinRamDistance  int32   `json:"min_ram_distance"`
	KnockbackForce  float32 `json:"knockback_force"`
	KnockbackHeight float32 `json:"knockback_height"`
	BabyModifier    float32 `json:"baby_knockback_modifier"`
	RunSpeed        float32 `json:"run_speed"`
	RamSpeed        float32 `json:"ram_speed"`
	PreRamSound     int
	RamImpactSound  int
	// Cooldown is cooldown_range in seconds.
	Cooldown Range[float32]
	OnStart  string
}

// DefaultRamAttack is the goal's BDS defaults.
var DefaultRamAttack = RamAttackConfig{
	RamDistance: 7, MinRamDistance: 4, KnockbackForce: 5, KnockbackHeight: 0.1, BabyModifier: 1.0 / 3,
	RunSpeed: 1, RamSpeed: 2, PreRamSound: NoSound, RamImpactSound: NoSound, Cooldown: Range[float32]{10, 20},
}

// ramSpot is a run-up spot in one direction from the target.
type ramSpot struct {
	pos   Vec3
	dist  float32
	dir   int32
	valid bool
}

// RamAttack is RamAttackGoal: the goat backs off in line with its target,
// turns, waits, and charges it.
type RamAttack struct {
	base
	m        Rammer
	cfg      RamAttackConfig
	state    ramState
	start    Vec3
	dest     Vec3
	dirX     float32
	dirZ     float32
	wait     uint64
	ramUntil uint64
	settle   uint64
	pathEnd  uint64
	spots    [ramDirections]ramSpot
	target   ID
	hit      bool
	cooldown uint64
	near     []Actor
}

// NewRamAttack makes the goal. BDS rolls the first cooldown when it builds
// the goal.
func NewRamAttack(w World, m Rammer, c RamAttackConfig) *RamAttack {
	g := &RamAttack{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, state: ramDone, target: NoID}
	g.rollCooldown()
	return g
}

// rollCooldown is _resetCooldown.
func (g *RamAttack) rollCooldown() {
	c := g.cfg.Cooldown
	g.cooldown = g.world.CurrentTick() + uint64(int32(((c.Max-c.Min)*g.world.Random().NextFloat()+c.Min)*ticksPerSecond))
}

// parts reports the components every step needs.
func (g *RamAttack) parts() bool {
	_, ok := g.m.MoveControl()
	return g.m.Nav() != nil && ok && g.m.HasLookControl()
}

// CanUse implements goal.Goal (0x575D610).
func (g *RamAttack) CanUse() bool {
	if g.cooldown > g.world.CurrentTick() || g.actor(g.m.Target()) == nil {
		return false
	}
	return g.parts()
}

// CanContinueToUse implements goal.Goal (0x575D830).
func (g *RamAttack) CanContinueToUse() bool { return g.parts() && g.state != ramDone }

// Start implements goal.Goal (0x575DA00).
func (g *RamAttack) Start() {
	g.state = ramDone
	if g.cfg.OnStart != "" {
		g.m.Event(g.cfg.OnStart, g.m.Target())
	}
	t := g.actor(g.m.Target())
	if t == nil {
		return
	}
	g.target = t.ID()
	if g.initiate() {
		g.state = ramToStart
	}
	g.pathEnd = g.world.CurrentTick() + ramPathTime
}

// Stop implements goal.Goal (0x575E570).
func (g *RamAttack) Stop() {
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
	g.m.SetFlag(FlagRamAttack, false)
	g.target = NoID
	g.m.RamReset()
	g.state = ramDone
	g.spots = [ramDirections]ramSpot{}
	g.hit = false
	g.rollCooldown()
}

// initiate is _initiateRamAttack (0x575DB20): find the run-up spots in
// line with the target and walk to the nearest. Like BDS it drops the
// mob's target; the goal keeps its own.
func (g *RamAttack) initiate() bool {
	t := g.actor(g.target)
	if t == nil {
		return false
	}
	f := blockOf(t.AttachPos(attachFeet))
	g.dest = Vec3{X: float32(f.X) + ramBlockCentre, Y: float32(f.Y) + ramBlockCentre, Z: float32(f.Z) + ramBlockCentre}
	g.m.SetTarget(NoID)
	g.spots = [ramDirections]ramSpot{{dir: ramDirPlusX}, {dir: ramDirMinusX}, {dir: ramDirPlusZ}, {dir: ramDirMinusZ}}
	for i := range g.spots {
		if !g.walkOut(&g.spots[i]) {
			return false
		}
	}
	g.sortSpots()
	for _, s := range g.spots {
		if !s.valid {
			continue
		}
		g.start = s.pos
		n := g.m.Nav()
		if n == nil {
			g.state = ramDone
			return false
		}
		n.MoveTo(g.start, g.cfg.RunSpeed)
		n.SetLastNodeReach(ramEndRadius2)
		return true
	}
	return false
}

// walkOut steps away from the target in the spot's direction while the
// ground holds and the mob fits, keeping the farthest step at or past
// min_ram_distance. It reports false when the target's own block fails,
// which ends the whole search.
func (g *RamAttack) walkOut(s *ramSpot) bool {
	x, y, z := g.dest.X, g.dest.Y, g.dest.Z
	if !g.pathable(x, y, z) {
		return false
	}
	for i := int32(0); i < g.cfg.RamDistance; i++ {
		switch s.dir {
		case ramDirMinusZ:
			z--
		case ramDirPlusZ:
			z++
		case ramDirMinusX:
			x--
		case ramDirPlusX:
			x++
		}
		if !g.pathable(x, y, z) {
			return true
		}
		if i+1 < g.cfg.MinRamDistance {
			continue
		}
		s.pos = Vec3{X: x, Y: y, Z: z}
		c := blockOf(g.m.AttachPos(attachFeet))
		d := Vec3{X: float32(c.X) + ramBlockCentre - x, Y: float32(c.Y) + ramBlockCentre - y, Z: float32(c.Z) + ramBlockCentre - z}
		s.dist, s.valid = length(d), true
	}
	return true
}

// pathable is _pathablePos: solid ground under the block and room for the
// mob's height.
func (g *RamAttack) pathable(x, y, z float32) bool {
	p := Pos{X: floorInt(x), Y: floorInt(y), Z: floorInt(z)}
	if !g.world.Block(Pos{X: p.X, Y: p.Y - 1, Z: p.Z}).BlockSolid {
		return false
	}
	b := g.m.Box()
	h := int32(math.Ceil(float64(b.Max.Y - b.Min.Y)))
	for k := int32(0); k < h; k++ {
		if !g.world.Passable(Pos{X: p.X, Y: p.Y + k, Z: p.Z}, g.m.ID()) {
			return false
		}
	}
	return true
}

// sortSpots orders the spots nearest first; like BDS's sort of four it
// keeps equal distances in their order.
func (g *RamAttack) sortSpots() {
	for i := 1; i < len(g.spots); i++ {
		for j := i; j > 0 && g.spots[j-1].dist > g.spots[j].dist; j-- {
			g.spots[j-1], g.spots[j] = g.spots[j], g.spots[j-1]
		}
	}
}

// Tick implements goal.Goal (0x575E750).
func (g *RamAttack) Tick() {
	t := g.actor(g.target)
	turn, ok := g.m.MoveControl()
	n := g.m.Nav()
	if n == nil || !ok || !g.m.HasLookControl() || t == nil {
		g.state = ramDone
		g.finish()
		return
	}
	now := g.world.CurrentTick()
	switch g.state {
	case ramToStart:
		g.toStart(t, n)
	case ramTurning:
		p := g.m.Pos()
		yaw := float32(math.Atan2(float64(g.dest.X-p.X), float64(g.dest.Z-p.Z))) * -degPerRad
		if wrap180(faceYaw(g.m, g.m, yaw, turn)-yaw+180) < ramFaced {
			g.state = ramChecking
		}
	case ramChecking:
		g.check(t, now)
	case ramWindingUp:
		if now <= g.wait {
			return
		}
		g.state = ramRamming
		g.m.ResetLook()
		n.MoveTo(g.dest, g.cfg.RamSpeed)
		n.SetLastNodeReach(ramEndRadius2)
		g.ramUntil = now + ramTime
		g.m.RamBlockAhead(g.blockAhead())
	case ramRamming:
		g.ram(n, now)
	case ramSettling:
		if g.hit {
			g.state = ramDone
		} else {
			g.knock()
		}
		if now > g.settle {
			g.impact()
		}
	case ramDone:
		g.finish()
	}
}

// finish is the done state's tick: the flag drops and the cooldown is
// rolled again.
func (g *RamAttack) finish() {
	g.m.SetFlag(FlagRamAttack, false)
	g.rollCooldown()
}

// toStart walks to the run-up spot, looking at the target, until the mob
// stands on it.
func (g *RamAttack) toStart(t Actor, n Navigator) {
	if !g.m.LookBusy() {
		g.m.LookAt(t.AttachPos(attachHead), ramLookYaw, ramLookPitch)
	}
	p := g.m.Pos()
	dx, dz := g.start.X-p.X, g.start.Z-p.Z
	if dx*dx+dz*dz <= ramAtStart2 && abs32(g.start.Y-boxCentre(g.m.Box()).Y) < ramAtStartY {
		d := normalize(sub(g.dest, p))
		g.dirX, g.dirZ = d.X, d.Z
		n.Stop()
		g.state = ramTurning
		return
	}
	if !n.Done() {
		return
	}
	g.state = ramDone
}

// check waits for the target to still be on the spot, then winds up; a
// target that moved gets a new run-up while there is time.
func (g *RamAttack) check(t Actor, now uint64) {
	q := t.Pos()
	dy := g.dest.Y - boxCentre(t.Box()).Y
	dx, dz := g.dest.X-q.X, g.dest.Z-q.Z
	if sqrt32(dz*dz+(dx*dx+dy*dy)) < ramTargetStill {
		g.wait = now + ramWindUp
		if g.cfg.PreRamSound != NoSound {
			g.m.PlaySoundAt(g.cfg.PreRamSound, g.m.Pos())
		}
		g.m.SetFlag(FlagRamAttack, true)
		g.state = ramWindingUp
		return
	}
	if now < g.pathEnd && g.initiate() {
		g.state = ramToStart
		return
	}
	g.state = ramDone
}

// ram charges along the line and stops on a hit, on arrival or when the
// path or the time runs out.
func (g *RamAttack) ram(n Navigator, now uint64) {
	g.knock()
	f := blockOf(g.m.AttachPos(attachFeet))
	c := Vec3{X: float32(f.X) + ramBlockCentre, Y: float32(f.Y) + ramBlockCentre, Z: float32(f.Z) + ramBlockCentre}
	if dist2(g.dest, c) < ramSlowDist2 {
		n.SetSpeed(ramSlowSpeed)
	}
	p := g.m.Pos()
	at := Vec3{X: p.X, Y: boxCentre(g.m.Box()).Y, Z: p.Z}
	if g.hit || sqrt32(dist2(g.dest, at)) < ramArrived {
		n.Stop()
		if g.hit {
			g.impact()
			return
		}
		g.settle = now + ramSettle
		g.state = ramSettling
		return
	}
	if n.Done() || now > g.ramUntil {
		g.state = ramDone
	}
}

// impact ends the ram against the spot.
func (g *RamAttack) impact() {
	g.m.RamImpact(g.dest)
	g.state = ramDone
}

// blockAhead is the block past the target along the ram's main axis
// (0x575F390).
func (g *RamAttack) blockAhead() Vec3 {
	p := g.m.Pos()
	x, z := g.dest.X, g.dest.Z
	dx := int32(math.Round(float64(p.X - x)))
	dz := int32(math.Round(float64(p.Z - z)))
	switch {
	case abs32(float32(dx)) <= abs32(float32(dz)) && dz < 0:
		z++
	case abs32(float32(dx)) <= abs32(float32(dz)):
		z--
	case dx < 0:
		x++
	default:
		x--
	}
	return Vec3{X: x, Y: g.dest.Y, Z: z}
}

// knock is _tryKnockbackTarget (0x575F470): the first mob in the box the
// ram sweeps is hit and knocked along the ram.
func (g *RamAttack) knock() {
	if g.hit {
		return
	}
	p := g.m.Pos()
	ahead := add(p, scale(normalize(sub(g.dest, p)), ramReachAhead))
	b := g.m.Box()
	box := Box{
		Min: Vec3{X: min(b.Min.X, ahead.X), Y: min(b.Min.Y, ahead.Y), Z: min(b.Min.Z, ahead.Z)},
		Max: Vec3{X: max(b.Max.X, ahead.X), Y: max(b.Max.Y, ahead.Y), Z: max(b.Max.Z, ahead.Z)},
	}
	g.near = g.world.Entities(g.near[:0], g.m.ID(), box)
	for _, e := range g.near {
		if !e.IsMob() {
			continue
		}
		g.m.RamHit(e)
		g.m.RamReset()
		g.knockOne(e)
		return
	}
}

// knockOne knocks the hit mob back by the ram's speed.
func (g *RamAttack) knockOne(e Actor) {
	f := float32(1)
	if e.Flag(FlagBlockedUsingShield) {
		f = ramShieldFactor
	}
	v := scale(g.m.Velocity(), ticksPerSecond)
	s := min(ramMaxStrength, max32(length(v)/ramSpeedDiv, ramMinStrength))
	k := g.cfg.KnockbackForce
	if g.m.Flag(FlagBaby) {
		k = max32(k*g.cfg.BabyModifier, ramBabyMinKnock)
	}
	s *= f * k
	g.m.Knockback(e, Knockback{Power: ramKnockPower * s, Height: g.cfg.KnockbackHeight * s, HeightCap: ramKnockCap,
		Extra: knockbackExtra, DirX: -g.dirX, DirZ: -g.dirZ})
	g.hit = true
	if g.cfg.RamImpactSound != NoSound {
		g.m.PlaySoundAt(g.cfg.RamImpactSound, g.m.Pos())
	}
}
