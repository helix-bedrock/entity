package behavior

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// The actor events, level sound event and particle the dragon goals send.
const (
	actorEventDragonDeath   = 37
	actorEventDragonFlaming = 68
	soundMad                = 19
	particleHugeExplosion   = 17
)

// Dragon is the part of the mob the ender dragon goals need on top of Mob.
// The node graph, the path search, the crystals and the flight state are
// the engine's EnderDragon; the goals reach them through these hooks.
type Dragon interface {
	Mob
	// FlyTarget is the point the dragon flies to (EnderDragon +2000); the
	// zero vector means none. SetFlyTarget replaces it.
	FlyTarget() Vec3
	SetFlyTarget(p Vec3)
	// CrystalsAlive is the number of end crystals still standing
	// (EnderDragon +2112).
	CrystalsAlive() int32
	// FlameCount is how many times the dragon breathed fire since it last
	// landed (EnderDragon +2012).
	FlameCount() int32
	SetFlameCount(n int32)
	// SetSitting is EnderDragon::setSitting (vt+448): it sets FlagSitting
	// and clears the dragon's motion.
	SetSitting(on bool)
	// Nodes are the dragon's path nodes (EnderDragon +2024): an outer ring
	// of 12, an inner ring of 8 and the rest around the podium.
	Nodes() []Pos
	// ClosestNode is EnderDragon::findClosestNode (0xBD3A2E0), the node
	// nearest the dragon.
	ClosestNode() int
	// DragonPath is EnderDragon::findPath (0xBD3B5A0): the nodes from node
	// from to node to, appended to dst, followed by final when hasFinal.
	// It returns dst unchanged when there is no path.
	DragonPath(dst []Pos, from, to int, final Pos, hasFinal bool) []Pos
	// SetAirSpeed sets AirSpeedComponent; SetTurnSpeed sets EnderDragon
	// +1996, how fast the dragon turns toward its fly target.
	SetAirSpeed(v float32)
	SetTurnSpeed(v float32)
	// HeadLook is the head's look vector at the partial tick (vt+288).
	HeadLook(partial float32) Vec3
	// HeadBox is the box of the dragon's head part (EnderDragon +1920).
	HeadBox() Box
	// Yaw is the yaw of ActorRotationComponent.
	Yaw() float32
	// YawSpeed is MobRotationComponent, the yaw change of the last tick.
	YawSpeed() float32
	SetYawSpeed(v float32)
	// HorizontalCollision is HorizontalCollisionFlagComponent.
	HorizontalCollision() bool
	// StartLanding and StartTakeoff fire the dragon's
	// minecraft:on_start_landing and minecraft:on_start_takeoff triggers.
	StartLanding()
	StartTakeoff()
	// Health and SetHealth are the current value of the health attribute.
	Health() float32
	SetHealth(v float32)
	// ActorEvent broadcasts an actor event of the mob.
	ActorEvent(id, data int32)
	// ShootFireball lets ShooterComponent pick its projectile, then fires
	// it at t (Mob::performRangedAttack).
	ShootFireball(t Actor)
	// SpawnBreath spawns the area effect cloud of dragon's breath that
	// dragonflaming leaves on the ground, owned by the dragon.
	SpawnBreath(p Vec3, radius float32, duration int32, colour string)
	// Particle adds a level particle of the given type at p.
	Particle(id int32, p Vec3)
}

// DragonFlightConfig holds the fixed numbers the dragon goals share.
type DragonFlightConfig struct {
	// Podium is the block the dragon's podium height is read at; its height
	// is the column's top solid block.
	Podium Pos
	// NearDist and FarDist: a fly target nearer than NearDist or farther
	// than FarDist is done with.
	NearDist, FarDist float32
	// Rise is the most a path node's height is raised at random.
	Rise float32
	// AimDist and AimY place the node search point AimDist blocks out from
	// the podium, at height AimY.
	AimDist float32
	AimY    int32
	// NodeSearch is the squared distance a node must beat to be picked.
	NodeSearch float32
	// TurnAround is how many nodes around the ring the dragon jumps when it
	// changes direction.
	TurnAround int
}

// DefaultDragonFlight returns the BDS numbers.
func DefaultDragonFlight() DragonFlightConfig {
	return DragonFlightConfig{Podium: Pos{Y: 100}, NearDist: 10, FarDist: 150, Rise: 20, AimDist: 40, AimY: 105,
		NodeSearch: 10000, TurnAround: 6}
}

// The rings of the dragon's node graph.
const (
	dragonOuterRing = 12
	dragonInnerRing = 8
)

// dragonFlight is the path and node logic the dragon goals share.
type dragonFlight struct {
	base
	d    Dragon
	fcfg DragonFlightConfig
	// path is the node path being flown, while hasPath; next is the next
	// node to fly to.
	path    []Pos
	next    int
	hasPath bool
}

// podiumY is the podium's height.
func (f *dragonFlight) podiumY() int32 {
	return f.world.AboveTopSolid(f.fcfg.Podium.X, f.fcfg.Podium.Z)
}

// clearTarget drops the fly target.
func (f *dragonFlight) clearTarget() { f.d.SetFlyTarget(Vec3{}) }

// clearPath drops the path.
func (f *dragonFlight) clearPath() { f.path, f.next, f.hasPath = f.path[:0], 0, false }

// pathDone reports a path that has no node left.
func (f *dragonFlight) pathDone() bool { return f.next >= len(f.path) }

// needsTarget reports a fly target that is not set, reached, too far, or
// blocked.
func (f *dragonFlight) needsTarget() bool {
	t := f.d.FlyTarget()
	if t == (Vec3{}) {
		return true
	}
	d := dist2(f.d.Pos(), t)
	return d < f.fcfg.NearDist*f.fcfg.NearDist || d > f.fcfg.FarDist*f.fcfg.FarDist ||
		f.d.HorizontalCollision() || f.d.VerticalCollision()
}

// findPath replaces the path; it is dropped when there is none.
func (f *dragonFlight) findPath(from, to int, final Pos, hasFinal bool) {
	f.path = f.d.DragonPath(f.path[:0], from, to, final, hasFinal)
	f.next, f.hasPath = 0, len(f.path) > 0
}

// flyNext flies to the path's next node, raised by a random height.
func (f *dragonFlight) flyNext() {
	n := f.path[f.next]
	f.next++
	y := float32(n.Y)
	h := f.world.IRandom().NextFloat()*f.fcfg.Rise + y
	for y > h {
		h = f.world.IRandom().NextFloat()*f.fcfg.Rise + y
	}
	f.d.SetFlyTarget(Vec3{X: float32(n.X), Y: h, Z: float32(n.Z)})
}

// skipAndFly skips the first node of a new path and flies to the one after
// it, as most goals do.
func (f *dragonFlight) skipAndFly() {
	if !f.hasPath {
		return
	}
	f.next++
	if !f.pathDone() {
		f.flyNext()
	}
}

// nearestNode is the inlined findClosestNode(x, y, z): the node nearest
// the point, the outer ring left out once the crystals are gone. Only a
// node nearer than NodeSearch counts; otherwise it is node 0.
func (f *dragonFlight) nearestNode(x, y, z int32) int {
	nodes := f.d.Nodes()
	start := dragonOuterRing
	if f.d.CrystalsAlive() != 0 {
		start = 0
	}
	best, most := 0, f.fcfg.NodeSearch
	for i := start; i < len(nodes); i++ {
		dx, dy, dz := int64(nodes[i].X-x), int64(nodes[i].Y-y), int64(nodes[i].Z-z)
		d := float32(dx*dx + dy*dy + dz*dz)
		if most > d {
			best = i
		}
		most = min(d, most)
	}
	return best
}

// ringNode keeps node i on the outer ring while crystals stand, and on the
// inner ring once they are gone.
func (f *dragonFlight) ringNode(i int) int {
	if f.d.CrystalsAlive() <= 0 {
		return (i&(dragonInnerRing-1) ^ 4) + dragonOuterRing
	}
	if i %= dragonOuterRing; i < 0 {
		i += dragonOuterRing
	}
	return i
}

// circle finds the path to the next node around the ring, turning around
// when turn says so.
func (f *dragonFlight) circle(clockwise *bool, turn bool) {
	c := f.d.ClosestNode()
	n := c
	if turn {
		*clockwise = !*clockwise
		n = c + f.fcfg.TurnAround
	}
	if *clockwise {
		n++
	} else {
		n--
	}
	f.findPath(c, f.ringNode(n), Pos{}, false)
}

// aimNode is the node nearest the point AimDist blocks out from the podium
// against the direction (x, z).
func (f *dragonFlight) aimNode(x, z float32) int {
	p := f.fcfg.Podium
	return f.nearestNode(p.X+floorInt(x*-f.fcfg.AimDist), f.fcfg.AimY, p.Z+floorInt(z*-f.fcfg.AimDist))
}

// nearestPlayer is Dimension's nearest attackable player (0xCF2C080): the
// nearest live, not invulnerable player within r of at, the range scaled
// for sneaking, invisibility and, with a source mob, its head being worn.
// A negative r has no limit.
func nearestPlayer(w World, buf []Actor, at Vec3, r float32, src Mob) (Actor, []Actor) {
	buf = w.Players(buf[:0])
	var best Actor
	most := float32(-1)
	for _, a := range buf {
		if a.Invulnerable() || !a.Alive() {
			continue
		}
		s := r
		if a.Flag(flagSneaking) {
			s *= nearestPlayerSneak
		}
		if a.Flag(flagInvisible) {
			s *= max32(minArmourSeen, a.ArmourCoverage()) * nearestPlayerInvisible
		}
		if src != nil && !src.Removed() && src.IsMob() && a.WearsHeadOf(src.Type()) {
			s *= headRangeScale
		}
		d := dist2(a.Pos(), at)
		if (r < 0 || s*s > d) && (most == -1 || most > d) {
			best, most = a, d
		}
	}
	return best, buf
}

// The range scales of nearestPlayer.
const (
	nearestPlayerSneak     = 0.8
	nearestPlayerInvisible = 0.7
)

// viewAngle is the angle in degrees between the dragon's yaw and the flat
// direction to p, plus half a degree.
func viewAngle(d Dragon, p Vec3) float32 {
	q := d.Pos()
	dx, dz := p.X-q.X, p.Z-q.Z
	l := float32(math.Sqrt(float64(dz*dz + (dx*dx + 0))))
	var nx, nz float32
	if l >= kineticMinLength {
		nx, nz = dx/l, dz/l
	}
	yaw := d.Yaw() * radPerDeg
	s, c := float32(math.Sin(float64(yaw))), float32(math.Cos(float64(yaw)))
	m := float32(math.Sqrt(float64(c*c + (s*s + 0))))
	var lx, lz float32
	if m >= kineticMinLength {
		lx, lz = s/m, -c/m
	}
	return float32(math.Acos(float64(lz*nz+(lx*nx+0))))*degPerRad + 0.5
}

// sees is the dragon's line of sight to t's head.
func sees(w World, d Dragon, t Actor) bool {
	return w.ClearLine(d.AttachPos(attachHead), t.AttachPos(attachHead))
}

// DragonHoldingPatternConfig is minecraft:behavior.dragonholdingpattern.
// The JSON has no fields; these are the goal's fixed BDS numbers.
type DragonHoldingPatternConfig struct {
	DragonFlightConfig
	// LandingChance: at the end of a path the dragon lands 1 in
	// (crystals + LandingChance).
	LandingChance int32
	// PlayerRange is how far from the podium players are looked for.
	PlayerRange float32
	// A found player is picked 1 in n, n being the squared distance to the
	// podium over PlayerChanceDiv plus PlayerChanceMin, or PlayerChanceMax
	// for a removed player; or 1 in (crystals + CrystalChance).
	PlayerChanceDiv, PlayerChanceMin, PlayerChanceMax int32
	CrystalChance                                     int32
	// TurnChance: a new path turns around 1 in TurnChance.
	TurnChance int32
}

// DefaultDragonHoldingPattern returns the BDS numbers.
func DefaultDragonHoldingPattern() DragonHoldingPatternConfig {
	return DragonHoldingPatternConfig{DragonFlightConfig: DefaultDragonFlight(), LandingChance: 3, PlayerRange: 64,
		PlayerChanceDiv: 512, PlayerChanceMin: 2, PlayerChanceMax: 66, CrystalChance: 2, TurnChance: 8}
}

// DragonHoldingPattern is DragonHoldingPatternGoal (vtable 0xF777BF0;
// canUse 0x5520150, canContinue 0x55201A0, start 0x55201E0, stop
// 0x5520260, tick 0x55202C0): with no target the dragon circles the ring
// of nodes, and at the end of each path may land or pick a player.
type DragonHoldingPattern struct {
	dragonFlight
	cfg       DragonHoldingPatternConfig
	clockwise bool
	buf       []Actor
}

// NewDragonHoldingPattern makes the goal.
func NewDragonHoldingPattern(w World, m Dragon, c DragonHoldingPatternConfig) *DragonHoldingPattern {
	return &DragonHoldingPattern{dragonFlight: dragonFlight{base: base{mob: m, world: w, flags: flagsMove}, d: m,
		fcfg: c.DragonFlightConfig}, cfg: c}
}

// Config returns the goal's settings.
func (g *DragonHoldingPattern) Config() DragonHoldingPatternConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *DragonHoldingPattern) SetConfig(c DragonHoldingPatternConfig) {
	g.cfg, g.fcfg = c, c.DragonFlightConfig
}

// CanUse implements goal.Goal: flying, with no target.
func (g *DragonHoldingPattern) CanUse() bool {
	return !g.d.Flag(FlagSitting) && g.actor(g.d.Target()) == nil
}

// CanContinueToUse implements goal.Goal.
func (g *DragonHoldingPattern) CanContinueToUse() bool { return g.actor(g.d.Target()) == nil }

// Start implements goal.Goal.
func (g *DragonHoldingPattern) Start() {
	g.clearTarget()
	g.d.SetTarget(NoID)
	g.clearPath()
	g.d.SetSitting(false)
}

// Stop implements goal.Goal.
func (g *DragonHoldingPattern) Stop() {
	g.clearTarget()
	g.clearPath()
}

// Tick implements goal.Goal.
func (g *DragonHoldingPattern) Tick() {
	if !g.needsTarget() {
		return
	}
	if g.hasPath && g.pathDone() && g.endOfPath() {
		return
	}
	if g.hasPath && !g.pathDone() {
		g.flyNext()
		return
	}
	r := g.world.IRandom()
	g.circle(&g.clockwise, r.NextInt(g.cfg.TurnChance) == 0)
	g.skipAndFly()
}

// endOfPath lands or picks a player at the end of a path; it reports that
// it did one of them.
func (g *DragonHoldingPattern) endOfPath() bool {
	crystals := g.d.CrystalsAlive()
	h := g.podiumY()
	r := g.world.IRandom()
	if r.NextInt(crystals+g.cfg.LandingChance) == 0 {
		g.d.StartLanding()
		return true
	}
	p := g.fcfg.Podium
	var t Actor
	t, g.buf = nearestPlayer(g.world, g.buf, Vec3{X: float32(p.X), Y: float32(h), Z: float32(p.Z)}, g.cfg.PlayerRange, nil)
	if t == nil {
		r.NextInt(g.cfg.PlayerChanceMax)
		return false
	}
	n := g.cfg.PlayerChanceMax
	if !t.Removed() {
		q := t.Pos()
		x, y, z := float32(p.X)+0.5-q.X, float32(h)+0.5-q.Y, float32(p.Z)+0.5-q.Z
		v := int32((z*z + (x*x + y*y)) / float32(g.cfg.PlayerChanceDiv))
		if v < 0 {
			v = -v
		}
		n = v + g.cfg.PlayerChanceMin
	}
	if r.NextInt(n) != 0 && r.NextInt(crystals+g.cfg.CrystalChance) != 0 {
		return false
	}
	if spectator(g.world, t) || !sees(g.world, g.d, t) {
		return false
	}
	g.d.SetTarget(t.ID())
	return true
}

// DragonTakeoffConfig is minecraft:behavior.dragontakeoff. The JSON has no
// fields; these are the goal's fixed BDS numbers.
type DragonTakeoffConfig struct {
	DragonFlightConfig
	// PodiumDist is how near the podium the goal keeps running.
	PodiumDist float32
}

// DefaultDragonTakeoff returns the BDS numbers.
func DefaultDragonTakeoff() DragonTakeoffConfig {
	return DragonTakeoffConfig{DragonFlightConfig: DefaultDragonFlight(), PodiumDist: 10}
}

// DragonTakeoff is DragonTakeoffGoal (canUse 0x55235D0, canContinue
// 0x5523620, start 0x55236E0, stop 0x5523760, tick 0x55237C0): the dragon
// leaves the podium the way its head points.
type DragonTakeoff struct {
	dragonFlight
	cfg     DragonTakeoffConfig
	pending bool
}

// NewDragonTakeoff makes the goal.
func NewDragonTakeoff(w World, m Dragon, c DragonTakeoffConfig) *DragonTakeoff {
	return &DragonTakeoff{dragonFlight: dragonFlight{base: base{mob: m, world: w, flags: flagsMove}, d: m,
		fcfg: c.DragonFlightConfig}, cfg: c}
}

// Config returns the goal's settings.
func (g *DragonTakeoff) Config() DragonTakeoffConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *DragonTakeoff) SetConfig(c DragonTakeoffConfig) { g.cfg, g.fcfg = c, c.DragonFlightConfig }

// CanUse implements goal.Goal: perched, with no target.
func (g *DragonTakeoff) CanUse() bool {
	return g.d.Flag(FlagSitting) && g.actor(g.d.Target()) == nil
}

// CanContinueToUse implements goal.Goal: while near the podium.
func (g *DragonTakeoff) CanContinueToUse() bool {
	p := g.fcfg.Podium
	c := Vec3{X: float32(p.X) + 0.5, Y: float32(g.podiumY()) + 0.5, Z: float32(p.Z) + 0.5}
	return dist2(c, g.d.Pos()) <= g.cfg.PodiumDist*g.cfg.PodiumDist
}

// Start implements goal.Goal.
func (g *DragonTakeoff) Start() {
	g.clearTarget()
	g.clearPath()
	g.d.SetSitting(false)
	g.pending = true
}

// Stop implements goal.Goal.
func (g *DragonTakeoff) Stop() {
	g.clearTarget()
	g.clearPath()
}

// Tick implements goal.Goal: on the first tick it finds the path out; the
// first node is the closest one, not skipped.
func (g *DragonTakeoff) Tick() {
	if !g.pending {
		return
	}
	g.pending = false
	c := g.d.ClosestNode()
	l := g.d.HeadLook(1)
	to := g.ringNode(g.aimNode(l.X, l.Z))
	g.findPath(c, to, Pos{}, false)
	if g.hasPath && !g.pathDone() {
		g.flyNext()
	}
}

// DragonLandingConfig is minecraft:behavior.dragonlanding. The JSON has no
// fields; these are the goal's fixed BDS numbers.
type DragonLandingConfig struct {
	DragonFlightConfig
	// PlayerRange is how far from the podium players are looked for.
	PlayerRange float32
	// NoPlayerAim is how far along +x from the podium the dragon comes in
	// with no player around.
	NoPlayerAim int32
	// AirSpeed and TurnSpeed are set while the dragon comes in;
	// LandAirSpeed and LandTurnSpeed for the last drop to the podium.
	AirSpeed, TurnSpeed, LandAirSpeed, LandTurnSpeed float32
	// Landed is the squared distance to the podium that counts as landed.
	Landed float32
}

// DefaultDragonLanding returns the BDS numbers.
func DefaultDragonLanding() DragonLandingConfig {
	return DragonLandingConfig{DragonFlightConfig: DefaultDragonFlight(), PlayerRange: 128, NoPlayerAim: 40,
		AirSpeed: 0.6, TurnSpeed: 0.7, LandAirSpeed: 1.5, LandTurnSpeed: 0, Landed: 1}
}

// DragonLanding is DragonLandingGoal (canUse 0x5520BB0, canContinue
// 0x5520BD0, start 0x5520C50, stop 0x5520DA0, tick 0x5520EB0): the dragon
// flies a path to the podium, from the side away from the nearest player,
// and perches.
type DragonLanding struct {
	dragonFlight
	cfg     DragonLandingConfig
	landing bool
	buf     []Actor
}

// NewDragonLanding makes the goal.
func NewDragonLanding(w World, m Dragon, c DragonLandingConfig) *DragonLanding {
	return &DragonLanding{dragonFlight: dragonFlight{base: base{mob: m, world: w, flags: flagsMove}, d: m,
		fcfg: c.DragonFlightConfig}, cfg: c}
}

// Config returns the goal's settings.
func (g *DragonLanding) Config() DragonLandingConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *DragonLanding) SetConfig(c DragonLandingConfig) { g.cfg, g.fcfg = c, c.DragonFlightConfig }

// CanUse implements goal.Goal: not perched.
func (g *DragonLanding) CanUse() bool { return !g.d.Flag(FlagSitting) }

// CanContinueToUse implements goal.Goal: on the podium the dragon perches
// and the goal ends.
func (g *DragonLanding) CanContinueToUse() bool {
	if g.landing && dist2(g.d.Pos(), g.d.FlyTarget()) < g.cfg.Landed {
		g.d.SetFlameCount(0)
		g.d.SetSitting(true)
		return false
	}
	return true
}

// Start implements goal.Goal.
func (g *DragonLanding) Start() {
	g.clearTarget()
	g.clearPath()
	g.reset()
}

// Stop implements goal.Goal.
func (g *DragonLanding) Stop() {
	g.clearTarget()
	g.reset()
}

func (g *DragonLanding) reset() {
	g.d.SetAirSpeed(g.cfg.AirSpeed)
	g.d.SetTurnSpeed(g.cfg.TurnSpeed)
	g.landing = false
}

// Tick implements goal.Goal.
func (g *DragonLanding) Tick() {
	if g.landing || !g.needsTarget() {
		return
	}
	if !g.hasPath || g.pathDone() {
		g.findWay()
		if !g.hasPath {
			return
		}
		g.next++
		if g.pathDone() {
			g.land()
			return
		}
	}
	g.flyNext()
	if g.pathDone() {
		g.land()
	}
}

// findWay finds the path to the podium.
func (g *DragonLanding) findWay() {
	c := g.d.ClosestNode()
	h := g.podiumY()
	p := g.fcfg.Podium
	var t Actor
	t, g.buf = nearestPlayer(g.world, g.buf, Vec3{X: float32(p.X), Y: float32(h), Z: float32(p.Z)}, g.cfg.PlayerRange, nil)
	var to int
	if t != nil && !t.Removed() {
		// BDS uses the player's position as a direction from the origin.
		q := t.Pos()
		l := float32(math.Sqrt(float64(q.Z*q.Z + (q.X*q.X + 0))))
		var nx, nz float32
		if l >= kineticMinLength {
			nx, nz = q.X/l, q.Z/l
		}
		to = g.aimNode(nx, nz)
	} else {
		to = g.nearestNode(p.X+g.cfg.NoPlayerAim, h, p.Z)
	}
	g.findPath(c, to, Pos{X: p.X, Y: h, Z: p.Z}, true)
}

// land drops onto the podium.
func (g *DragonLanding) land() {
	g.landing = true
	p := g.fcfg.Podium
	g.d.SetFlyTarget(Vec3{X: float32(p.X), Y: float32(g.podiumY()), Z: float32(p.Z)})
	g.d.SetAirSpeed(g.cfg.LandAirSpeed)
	g.d.SetTurnSpeed(g.cfg.LandTurnSpeed)
}

// DragonScanningConfig is minecraft:behavior.dragonscanning. The JSON has
// no fields; these are the goal's fixed BDS numbers.
type DragonScanningConfig struct {
	// NearRange is how near a player must come to be noticed; after
	// NoticeTime ticks a noticed player becomes the target.
	NearRange  float32
	NoticeTime float32
	// After GiveUpTime ticks with no one near, a player within FarRange the
	// dragon can see becomes the target and the dragon takes off.
	GiveUpTime float32
	FarRange   float32
	// ViewAngle is how far, in degrees, a player may be off the dragon's
	// yaw before it turns.
	ViewAngle float32
	// MaxTurn is the largest angle in degrees a turn aims for; TurnDistMin
	// caps the distance that slows it; TurnDamping keeps that much of the
	// last turn.
	MaxTurn, TurnDistMin, TurnDamping float32
}

// DefaultDragonScanning returns the BDS numbers.
func DefaultDragonScanning() DragonScanningConfig {
	return DragonScanningConfig{NearRange: 20, NoticeTime: 25, GiveUpTime: 100, FarRange: 150, ViewAngle: 10,
		MaxTurn: 100, TurnDistMin: 40, TurnDamping: 0.8}
}

// DragonScanning is DragonScanningGoal (canUse 0x55217D0, canContinue
// 0x5521820, start 0x5521C90, stop 0x5521CE0, tick 0x5521D00): perched,
// the dragon turns to face the nearest player and targets one that stays
// near, or takes off when no one comes.
type DragonScanning struct {
	base
	d    Dragon
	cfg  DragonScanningConfig
	time float32
	buf  []Actor
}

// NewDragonScanning makes the goal.
func NewDragonScanning(w World, m Dragon, c DragonScanningConfig) *DragonScanning {
	return &DragonScanning{base: base{mob: m, world: w, flags: flagsMove}, d: m, cfg: c}
}

// Config returns the goal's settings.
func (g *DragonScanning) Config() DragonScanningConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *DragonScanning) SetConfig(c DragonScanningConfig) { g.cfg = c }

// CanUse implements goal.Goal: perched, with no target.
func (g *DragonScanning) CanUse() bool {
	return g.d.Flag(FlagSitting) && g.actor(g.d.Target()) == nil
}

// near is the nearest player within NearRange.
func (g *DragonScanning) near() Actor {
	var t Actor
	t, g.buf = nearestPlayer(g.world, g.buf, vecOf(blockOf(g.d.Pos())), g.cfg.NearRange, g.d)
	return t
}

// CanContinueToUse implements goal.Goal.
func (g *DragonScanning) CanContinueToUse() bool {
	if t := g.near(); t != nil {
		if !t.Removed() && g.time > g.cfg.NoticeTime {
			g.d.SetTarget(t.ID())
			return false
		}
		return true
	}
	if g.time < g.cfg.GiveUpTime {
		return true
	}
	var t Actor
	t, g.buf = nearestPlayer(g.world, g.buf, vecOf(blockOf(g.d.Pos())), g.cfg.FarRange, g.d)
	if t != nil && !spectator(g.world, t) && sees(g.world, g.d, t) {
		g.d.SetTarget(t.ID())
	}
	g.d.StartTakeoff()
	return false
}

// Start implements goal.Goal.
func (g *DragonScanning) Start() {
	g.time = 0
	g.d.SetFlyTarget(Vec3{})
	g.d.SetTarget(NoID)
	g.d.SetSitting(true)
}

// Stop implements goal.Goal.
func (g *DragonScanning) Stop() { g.d.SetFlyTarget(Vec3{}) }

// Tick implements goal.Goal: the dragon turns toward a near player it does
// not face.
func (g *DragonScanning) Tick() {
	g.time++
	t := g.near()
	if t == nil || t.Removed() {
		return
	}
	q := t.Pos()
	if a := viewAngle(g.d, q); !(a < 0 || a > g.cfg.ViewAngle) {
		return
	}
	c := boxCentre(g.d.HeadBox())
	dx, dz := q.X-float32(floorInt(c.X)), q.Z-float32(floorInt(c.Z))
	yaw := g.d.Yaw()
	a := float32(math.Atan2(float64(dx), float64(dz)))
	turn := float32(math.Mod(float64(((a*-degPerRad+180)-yaw)+180), 360))
	if turn < 0 {
		turn += 360
	}
	turn = min(max(turn-180, -g.cfg.MaxTurn), g.cfg.MaxTurn)
	d := float32(math.Sqrt(float64(dx*dx+dz*dz))) + 1
	spin := turn*((1/min(g.cfg.TurnDistMin, d))/d) + g.d.YawSpeed()*g.cfg.TurnDamping
	g.d.SetYaw(yaw + spin)
	g.d.SetYawSpeed(spin)
}

// DragonFlamingConfig is the JSON of minecraft:behavior.dragonflaming.
type DragonFlamingConfig struct {
	// GroundFlameCount is how many times the dragon breathes fire before
	// it takes off again.
	GroundFlameCount int32 `json:"ground_flame_count"`
	// Cooldown, FlameTime, RoarTime and SmokeTime are cooldown_time,
	// flame_time, roar_time and smoke_time in ticks. Cooldown is how long
	// the goal runs after the roar.
	Cooldown, FlameTime, RoarTime, SmokeTime int32
	SmokeRadius                              float32 `json:"smoke_radius"`
	SmokeColour                              string  `json:"smoke_color"`
	// RoarSound and FlameEvent are the level sound event of the roar and
	// the actor event of the flames; BreathOffset scales how far toward the
	// head the breath lands.
	RoarSound    int
	FlameEvent   int32
	BreathOffset float32
}

// DefaultDragonFlaming returns the goal's BDS defaults (definition default
// 0xF263050).
func DefaultDragonFlaming() DragonFlamingConfig {
	return DragonFlamingConfig{GroundFlameCount: 4, Cooldown: 200, FlameTime: 10, RoarTime: 40, SmokeTime: 200,
		SmokeRadius: 5, SmokeColour: "#DC00EF", RoarSound: soundMad, FlameEvent: actorEventDragonFlaming,
		BreathOffset: 0.5}
}

// DragonFlaming is DragonFlamingGoal (canUse 0x551F220, canContinue
// 0x551F260, start 0x551F3F0, stop 0x551F440, tick 0x551F480): perched with
// a target, the dragon roars, breathes fire and leaves a cloud of dragon's
// breath on the ground.
type DragonFlaming struct {
	base
	d           Dragon
	cfg         DragonFlamingConfig
	flame, roar int32
}

// NewDragonFlaming makes the goal. BDS gives it no control flags.
func NewDragonFlaming(w World, m Dragon, c DragonFlamingConfig) *DragonFlaming {
	return &DragonFlaming{base: base{mob: m, world: w}, d: m, cfg: c}
}

// Config returns the goal's settings.
func (g *DragonFlaming) Config() DragonFlamingConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *DragonFlaming) SetConfig(c DragonFlamingConfig) { g.cfg = c }

// CanUse implements goal.Goal: a target.
func (g *DragonFlaming) CanUse() bool { return g.actor(g.d.Target()) != nil }

// CanContinueToUse implements goal.Goal: when the time is up and the dragon
// breathed fire often enough, it takes off.
func (g *DragonFlaming) CanContinueToUse() bool {
	if g.flame >= g.cfg.Cooldown && g.d.FlameCount() >= g.cfg.GroundFlameCount {
		g.d.StartTakeoff()
	}
	return g.flame < g.cfg.Cooldown
}

// Start implements goal.Goal.
func (g *DragonFlaming) Start() {
	g.roar = 0
	g.d.SetFlyTarget(Vec3{})
	g.d.SetSitting(true)
	g.d.SetFlameCount(g.d.FlameCount() + 1)
}

// Stop implements goal.Goal.
func (g *DragonFlaming) Stop() {
	g.flame, g.roar = 0, 0
	g.d.SetFlyTarget(Vec3{})
	g.d.SetTarget(NoID)
}

// Tick implements goal.Goal.
func (g *DragonFlaming) Tick() {
	r := g.roar
	g.roar++
	if r < g.cfg.RoarTime {
		g.d.PlaySoundAt(g.cfg.RoarSound, g.d.AttachPos(attachBody))
		return
	}
	f := g.flame
	g.flame++
	if f&1 == 1 && g.flame < g.cfg.FlameTime {
		g.d.ActorEvent(g.cfg.FlameEvent, 0)
	}
	if g.flame == g.cfg.FlameTime {
		g.breathe()
	}
}

// breathe leaves the cloud on the ground in front of the head.
func (g *DragonFlaming) breathe() {
	c := boxCentre(g.d.HeadBox())
	hx, hy, hz := float32(floorInt(c.X)), floorInt(c.Y), float32(floorInt(c.Z))
	p := g.d.Pos()
	dx, dz := hx-p.X, hz-p.Z
	l := float32(math.Sqrt(float64(dz*dz + (dx*dx + 0))))
	var nx, nz float32
	if l >= kineticMinLength {
		nx, nz = dx/l, dz/l
	}
	x := g.cfg.SmokeRadius*nx*g.cfg.BreathOffset + hx
	z := g.cfg.SmokeRadius*nz*g.cfg.BreathOffset + hz
	bx, bz := floorInt(x), floorInt(z)
	y := hy
	if g.world.Block(Pos{X: bx, Y: hy, Z: bz}).Material == path.MaterialAir {
		for v := hy - 1; v >= g.world.MinHeight(); v-- {
			if g.world.Block(Pos{X: bx, Y: v, Z: bz}).Material != path.MaterialAir {
				y = v
				break
			}
		}
	}
	g.d.SpawnBreath(Vec3{X: x, Y: float32(y) + 1, Z: z}, g.cfg.SmokeRadius, g.cfg.SmokeTime, g.cfg.SmokeColour)
}

// DragonChargePlayerConfig is the JSON of
// minecraft:behavior.dragonchargeplayer.
type DragonChargePlayerConfig struct {
	// ActiveSpeed and TurnSpeed are set while the goal runs, FlightSpeed
	// when it stops.
	ActiveSpeed float32 `json:"active_speed"`
	FlightSpeed float32 `json:"flight_speed"`
	TurnSpeed   float32 `json:"turn_speed"`
	// TargetZone is the distance from the charge point the charge counts as
	// on course.
	TargetZone Range[float32]
	// ContinueChargeThreshold is continue_charge_threshold_time in ticks:
	// how long the charge goes on once off course.
	ContinueChargeThreshold int32
}

// DefaultDragonChargePlayer returns the goal's BDS defaults (definition
// default 0xEA911D0).
func DefaultDragonChargePlayer() DragonChargePlayerConfig {
	return DragonChargePlayerConfig{ActiveSpeed: 3, FlightSpeed: 0.6, TurnSpeed: 0.7,
		TargetZone: Range[float32]{10, 150}, ContinueChargeThreshold: 10}
}

// DragonChargePlayer is DragonChargePlayerGoal (canUse 0x551E410,
// canContinue 0x551E460, start 0x551E4B0, stop 0x551E730, tick 0x551E850):
// fresh off the podium with a target, the dragon charges at the nearest
// player.
type DragonChargePlayer struct {
	base
	d   Dragon
	cfg DragonChargePlayerConfig
	off int32
	buf []Actor
}

// NewDragonChargePlayer makes the goal.
func NewDragonChargePlayer(w World, m Dragon, c DragonChargePlayerConfig) *DragonChargePlayer {
	return &DragonChargePlayer{base: base{mob: m, world: w, flags: flagsMove}, d: m, cfg: c}
}

// Config returns the goal's settings.
func (g *DragonChargePlayer) Config() DragonChargePlayerConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *DragonChargePlayer) SetConfig(c DragonChargePlayerConfig) { g.cfg = c }

// CanUse implements goal.Goal: perched, with a target.
func (g *DragonChargePlayer) CanUse() bool {
	return g.d.Flag(FlagSitting) && g.actor(g.d.Target()) != nil
}

// CanContinueToUse implements goal.Goal: while there is a charge point and
// the charge has not been off course for the threshold. Each call once off
// course counts a tick more.
func (g *DragonChargePlayer) CanContinueToUse() bool {
	if g.d.FlyTarget() == (Vec3{}) {
		return false
	}
	o := g.off
	if o <= 0 {
		return true
	}
	g.off = o + 1
	return o < g.cfg.ContinueChargeThreshold
}

// Start implements goal.Goal: the charge point is the nearest player's
// position.
func (g *DragonChargePlayer) Start() {
	g.off = 0
	g.d.SetSitting(false)
	g.d.SetAirSpeed(g.cfg.ActiveSpeed)
	g.d.SetTurnSpeed(g.cfg.TurnSpeed)
	var t Actor
	t, g.buf = nearestPlayer(g.world, g.buf, vecOf(blockOf(g.d.Pos())), g.cfg.TargetZone.Max, g.d)
	if t == nil {
		g.d.SetFlyTarget(Vec3{})
		return
	}
	g.d.SetFlyTarget(t.Pos())
}

// Stop implements goal.Goal.
func (g *DragonChargePlayer) Stop() {
	g.off = 0
	g.d.SetFlyTarget(Vec3{})
	g.d.SetAirSpeed(g.cfg.FlightSpeed)
	g.d.SetTarget(NoID)
}

// Tick implements goal.Goal: out of the zone or blocked counts as off
// course.
func (g *DragonChargePlayer) Tick() {
	d := dist2(g.d.Pos(), g.d.FlyTarget())
	z := g.cfg.TargetZone
	if z.Min*z.Min > d || d > z.Max*z.Max || g.d.HorizontalCollision() || g.d.VerticalCollision() {
		g.off++
	}
}

// DragonStrafePlayerConfig is the JSON of
// minecraft:behavior.dragonstrafeplayer.
type DragonStrafePlayerConfig struct {
	DragonFlightConfig
	// ActiveSpeed and TurnSpeed are set while the goal runs, FlightSpeed
	// when it stops.
	ActiveSpeed float32 `json:"active_speed"`
	FlightSpeed float32 `json:"flight_speed"`
	TurnSpeed   float32 `json:"turn_speed"`
	// TargetZone is the distance from the fly target the dragon keeps
	// flying it.
	TargetZone                 Range[float32]
	SwitchDirectionProbability float32 `json:"switch_direction_probability"`
	// InViewTime is target_in_range_and_in_view_time in ticks.
	InViewTime    int32
	FireballRange float32 `json:"fireball_range"`
	ViewAngle     float32 `json:"view_angle"`
	// The dragon flies AboveMax at most over the target, less nearer it:
	// distance / AboveDiv + AboveAdd - AboveSub.
	AboveMax, AboveDiv, AboveAdd, AboveSub float32
}

// DefaultDragonStrafePlayer returns the goal's BDS defaults (definition
// default 0xEA921A0).
func DefaultDragonStrafePlayer() DragonStrafePlayerConfig {
	return DragonStrafePlayerConfig{DragonFlightConfig: DefaultDragonFlight(), ActiveSpeed: 0.6, FlightSpeed: 0.6,
		TurnSpeed: 0.7, TargetZone: Range[float32]{10, 150}, SwitchDirectionProbability: 0.125, InViewTime: 5,
		FireballRange: 64, ViewAngle: 10, AboveMax: 10, AboveDiv: 80, AboveAdd: 0.4, AboveSub: 1}
}

// DragonStrafePlayer is DragonStrafePlayerGoal (canUse 0x5522340,
// canContinue 0x5522390, start 0x5522570, stop 0x5522B30, tick 0x5522C50):
// flying with a target, the dragon flies at it and shoots one fireball once
// it has had it in range and in view long enough.
type DragonStrafePlayer struct {
	dragonFlight
	cfg       DragonStrafePlayerConfig
	victim    ID
	clockwise bool
	fired     bool
	seen      int32
}

// NewDragonStrafePlayer makes the goal.
func NewDragonStrafePlayer(w World, m Dragon, c DragonStrafePlayerConfig) *DragonStrafePlayer {
	return &DragonStrafePlayer{dragonFlight: dragonFlight{base: base{mob: m, world: w, flags: flagsMove}, d: m,
		fcfg: c.DragonFlightConfig}, cfg: c, victim: NoID}
}

// Config returns the goal's settings.
func (g *DragonStrafePlayer) Config() DragonStrafePlayerConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *DragonStrafePlayer) SetConfig(c DragonStrafePlayerConfig) {
	g.cfg, g.fcfg = c, c.DragonFlightConfig
}

// CanUse implements goal.Goal: flying, with a target.
func (g *DragonStrafePlayer) CanUse() bool {
	return !g.d.Flag(FlagSitting) && g.actor(g.d.Target()) != nil
}

// strafed is the target the goal started with, while it is there.
func (g *DragonStrafePlayer) strafed() Actor {
	if t := g.actor(g.victim); t != nil && !t.Removed() {
		return t
	}
	return nil
}

// CanContinueToUse implements goal.Goal: until the fireball is shot.
func (g *DragonStrafePlayer) CanContinueToUse() bool { return !g.fired && g.strafed() != nil }

// above is the height over p the dragon aims for, from the flat distance.
func (g *DragonStrafePlayer) above(dx, dz float32) float32 {
	c := g.cfg
	return min(c.AboveMax, (float32(math.Sqrt(float64(dx*dx+dz*dz)))/c.AboveDiv+c.AboveAdd)-c.AboveSub)
}

// Start implements goal.Goal: a path toward the target that ends above it.
func (g *DragonStrafePlayer) Start() {
	g.fired, g.seen = false, 0
	g.clearPath()
	g.victim = NoID
	if t := g.actor(g.d.Target()); t != nil && !t.Removed() {
		g.victim = t.ID()
		g.d.SetTarget(t.ID())
		q, p := t.Pos(), g.d.Pos()
		c := g.d.ClosestNode()
		to := g.nearestNode(floorInt(q.X), floorInt(q.Y), floorInt(q.Z))
		x, z := float32(floorInt(q.X)), float32(floorInt(q.Z))
		y := g.above(x-p.X, z-p.Z) + q.Y
		g.findPath(c, to, Pos{X: int32(x), Y: floorInt(y), Z: int32(z)}, true)
		g.skipAndFly()
	}
	g.d.SetSitting(false)
	g.d.SetAirSpeed(g.cfg.ActiveSpeed)
	g.d.SetTurnSpeed(g.cfg.TurnSpeed)
}

// Stop implements goal.Goal.
func (g *DragonStrafePlayer) Stop() {
	g.d.SetTarget(NoID)
	g.clearTarget()
	g.d.SetAirSpeed(g.cfg.FlightSpeed)
	g.d.SetTurnSpeed(g.cfg.TurnSpeed)
}

// Tick implements goal.Goal.
func (g *DragonStrafePlayer) Tick() {
	t := g.strafed()
	if t == nil {
		return
	}
	q, p := t.Pos(), g.d.Pos()
	if g.hasPath && g.pathDone() {
		g.d.SetFlyTarget(Vec3{X: q.X, Y: g.above(q.X-p.X, q.Z-p.Z) + q.Y, Z: q.Z})
	}
	var d float32
	if f := g.d.FlyTarget(); f != (Vec3{}) {
		d = dist2(p, f)
	}
	if z := g.cfg.TargetZone; z.Min*z.Min > d || d > z.Max*z.Max {
		g.repath()
	}
	g.aim(t)
}

// repath flies the path's next node, or a new path around the ring.
func (g *DragonStrafePlayer) repath() {
	if g.hasPath && !g.pathDone() {
		g.flyNext()
		return
	}
	turn := false
	if s := g.cfg.SwitchDirectionProbability; s != 0 {
		turn = s >= g.world.Random().NextFloat()
	}
	g.circle(&g.clockwise, turn)
	g.skipAndFly()
}

// aim counts the ticks the target is in range and in view, and shoots once
// there are enough and the dragon faces it.
func (g *DragonStrafePlayer) aim(t Actor) {
	r := g.cfg.FireballRange
	if r*r <= dist2(t.Pos(), g.d.Pos()) || spectator(g.world, t) || !sees(g.world, g.d, t) {
		if g.seen > 0 {
			g.seen--
		}
		return
	}
	g.seen++
	a := viewAngle(g.d, t.Pos())
	if a >= 0 && g.seen >= g.cfg.InViewTime && g.cfg.ViewAngle > a && !g.fired {
		g.d.ShootFireball(t)
		g.seen = 0
		if g.hasPath {
			g.next = max(g.next, len(g.path))
		}
		g.fired = true
	}
}

// DragonDeathConfig is minecraft:behavior.dragondeath. The JSON has no
// fields; these are the goal's fixed BDS numbers.
type DragonDeathConfig struct {
	DragonFlightConfig
	// AirSpeed and TurnSpeed are set while the dragon dies, StopAirSpeed
	// when the goal stops.
	AirSpeed, TurnSpeed, StopAirSpeed float32
	// Every ParticleEvery ticks a particle appears within Spread of the
	// dragon, raised by Lift.
	ParticleEvery int32
	Spread        Vec3
	Lift          float32
	Particle      int32
	// DeathEvent is the actor event sent when the dragon reaches the
	// podium.
	DeathEvent int32
}

// DefaultDragonDeath returns the BDS numbers.
func DefaultDragonDeath() DragonDeathConfig {
	return DragonDeathConfig{DragonFlightConfig: DefaultDragonFlight(), AirSpeed: 3, TurnSpeed: 0.7, StopAirSpeed: 0.6,
		ParticleEvery: 10, Spread: Vec3{X: 8, Y: 4, Z: 8}, Lift: 2, Particle: particleHugeExplosion,
		DeathEvent: actorEventDragonDeath}
}

// DragonDeath is DragonDeathGoal (canUse 0x551EA80, canContinue 0x551EAA0,
// start 0x551EAC0, stop 0x551EBE0, tick 0x551ECE0): the dying dragon flies
// to the podium with explosions around it and dies there.
type DragonDeath struct {
	dragonFlight
	cfg  DragonDeathConfig
	time int32
}

// NewDragonDeath makes the goal. BDS gives it no control flags.
func NewDragonDeath(w World, m Dragon, c DragonDeathConfig) *DragonDeath {
	return &DragonDeath{dragonFlight: dragonFlight{base: base{mob: m, world: w}, d: m,
		fcfg: c.DragonFlightConfig}, cfg: c}
}

// Config returns the goal's settings.
func (g *DragonDeath) Config() DragonDeathConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *DragonDeath) SetConfig(c DragonDeathConfig) { g.cfg, g.fcfg = c, c.DragonFlightConfig }

// CanUse implements goal.Goal: while there is health left.
func (g *DragonDeath) CanUse() bool { return g.d.Health() > 0 }

// CanContinueToUse implements goal.Goal.
func (g *DragonDeath) CanContinueToUse() bool { return g.d.Health() > 0 }

// Start implements goal.Goal.
func (g *DragonDeath) Start() {
	g.clearTarget()
	g.d.SetAirSpeed(g.cfg.AirSpeed)
	g.d.SetTurnSpeed(g.cfg.TurnSpeed)
	g.time = 0
}

// Stop implements goal.Goal.
func (g *DragonDeath) Stop() {
	g.clearTarget()
	g.d.SetAirSpeed(g.cfg.StopAirSpeed)
}

// Tick implements goal.Goal: at the podium, or when it cannot get there,
// the health drops to 0; until then it is held at 1.
func (g *DragonDeath) Tick() {
	g.time++
	if e := g.cfg.ParticleEvery; e > 0 && g.time%e == 0 {
		r := g.world.IRandom()
		s := g.cfg.Spread
		x := (r.NextFloat() - 0.5) * s.X
		y := (r.NextFloat()-0.5)*s.Y + g.cfg.Lift
		z := (r.NextFloat() - 0.5) * s.Z
		p := g.d.Pos()
		g.d.Particle(g.cfg.Particle, Vec3{X: p.X + x, Y: p.Y + y, Z: p.Z + z})
	}
	if g.d.FlyTarget() == (Vec3{}) {
		p := g.fcfg.Podium
		g.d.SetFlyTarget(Vec3{X: float32(p.X), Y: float32(g.podiumY()), Z: float32(p.Z)})
	}
	if g.needsTarget() {
		g.d.SetHealth(0)
		g.d.ActorEvent(g.cfg.DeathEvent, 0)
		return
	}
	g.d.SetHealth(1)
}
