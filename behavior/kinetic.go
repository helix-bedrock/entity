package behavior

import "math"

// The kinetic weapon goal's fixed numbers.
const (
	kineticKeepPath  = 0.05 // below this roll a running path is made again
	kineticMinLength = 0.0001
)

// The kinetic weapon goal's states (goal +528).
const (
	kineticApproach = iota
	kineticCharge
	kineticReposition
	kineticCooldown
)

// KineticWielder is the part of the mob use_kinetic_weapon needs on top of
// Attacker. Items are the engine's: the held item's minecraft:kinetic_weapon
// item component and the attack it runs are reached through these hooks.
type KineticWielder interface {
	Attacker
	// KineticReach is the reach of the held item's minecraft:kinetic_weapon
	// component; ok is false when the mob holds no item with one.
	KineticReach() (reach Range[float32], ok bool)
	// StartKineticCharge starts the held weapon's kinetic attack with the
	// minimum speeds of its conditions multiplied by speedScale (BDS copies
	// the item component into an actor component and sets actor flag 4).
	// It does nothing while an attack already runs.
	StartKineticCharge(speedScale float32)
	// KineticCharging reports that the attack still runs (the actor
	// component exists).
	KineticCharging() bool
	// StopKineticCharge ends the attack and clears flag 4.
	StopKineticCharge()
}

// UseKineticWeaponConfig is the JSON of minecraft:behavior.use_kinetic_weapon:
// melee_attack's fields plus the charge's own.
type UseKineticWeaponConfig struct {
	MeleeConfig
	// ApproachDistance is how close, flat, the mob walks before it charges.
	ApproachDistance float32 `json:"approach_distance"`
	// RepositionDistance is how far past the target the mob runs after a
	// charge.
	RepositionDistance        Range[float32]
	RepositionSpeedMultiplier float32 `json:"reposition_speed_multiplier"`
	// CooldownDistance is how far past the target the mob runs once the
	// attack ends.
	CooldownDistance Range[float32]
	// CooldownSpeedMultiplier is read but never used by BDS: the cooldown
	// walks use reposition_speed_multiplier.
	CooldownSpeedMultiplier float32 `json:"cooldown_speed_multiplier"`
	// WeaponReachMultiplier scales the weapon's reach.
	WeaponReachMultiplier float32 `json:"weapon_reach_multiplier"`
	// WeaponMinSpeedMultiplier scales the minimum speeds of the weapon's
	// conditions.
	WeaponMinSpeedMultiplier float32 `json:"weapon_min_speed_multiplier"`
}

// DefaultUseKineticWeapon returns the goal's BDS defaults (definition
// default 0xE29AA90; the v1_21_120 one, 0xEF78A80, has the same numbers).
func DefaultUseKineticWeapon() UseKineticWeaponConfig {
	return UseKineticWeaponConfig{
		MeleeConfig:      DefaultMelee(),
		ApproachDistance: 8, RepositionDistance: Range[float32]{2, 2}, RepositionSpeedMultiplier: 1,
		CooldownDistance: Range[float32]{8, 8}, CooldownSpeedMultiplier: 1,
		WeaponReachMultiplier: 1, WeaponMinSpeedMultiplier: 1,
	}
}

// UseKineticWeapon is UseKineticWeaponGoal (canUse 0x56A0310, canContinue
// 0x56A03F0, start 0x569FFA0, stop 0x56A0260, tick 0x56A04D0): a
// melee_attack whose mob, holding a kinetic weapon, closes in, charges
// through the target, runs past it and turns for another charge, and backs
// off once the attack ends.
type UseKineticWeapon struct {
	Melee
	k     KineticWielder
	kcfg  UseKineticWeaponConfig
	state int
	// reach is the weapon's reach times the multiplier; BDS keeps (0, 3)
	// until a weapon was read.
	reach Range[float32]
	// spot is the block the mob walks to, while hasSpot.
	spot    Pos
	hasSpot bool
	// cooldownEnd is the tick the cooldown ends, while hasEnd.
	cooldownEnd uint64
	hasEnd      bool
}

// NewUseKineticWeapon makes the goal (create 0x56CAAF0).
func NewUseKineticWeapon(w World, m KineticWielder, c UseKineticWeaponConfig) *UseKineticWeapon {
	g := &UseKineticWeapon{Melee: *NewMelee(w, m, c.MeleeConfig), k: m, reach: Range[float32]{0, 3}}
	g.kcfg = c
	return g
}

// Config returns the goal's settings.
func (g *UseKineticWeapon) Config() UseKineticWeaponConfig { return g.kcfg }

// SetConfig replaces the goal's settings.
func (g *UseKineticWeapon) SetConfig(c UseKineticWeaponConfig) {
	g.kcfg, g.cfg = c, c.MeleeConfig
}

// CanUse implements goal.Goal: melee_attack's test and a kinetic weapon in
// hand.
func (g *UseKineticWeapon) CanUse() bool {
	if !g.Melee.CanUse() {
		return false
	}
	_, ok := g.k.KineticReach()
	return ok
}

// CanContinueToUse implements goal.Goal.
func (g *UseKineticWeapon) CanContinueToUse() bool {
	if !g.Melee.CanContinueToUse() {
		return false
	}
	_, ok := g.k.KineticReach()
	return ok
}

// Start implements goal.Goal. MeleeAttackBaseGoal's start in 1.26.60
// (0x5433220) sets the repath timer to the fail increase.
func (g *UseKineticWeapon) Start() {
	g.Melee.Start()
	g.repath = g.cfg.FailIncrease
	g.readWeapon()
	g.hasSpot = false
	g.state = kineticApproach
}

// Stop implements goal.Goal.
func (g *UseKineticWeapon) Stop() {
	g.k.StopKineticCharge()
	g.Melee.Stop()
}

// readWeapon is 0x569FFD0: the held weapon's reach times the multiplier.
func (g *UseKineticWeapon) readWeapon() {
	if r, ok := g.k.KineticReach(); ok {
		m := g.kcfg.WeaponReachMultiplier
		g.reach = Range[float32]{r.Min * m, r.Max * m}
	}
}

// mid2 is the squared middle of the reach.
func (g *UseKineticWeapon) mid2() float32 {
	h := (g.reach.Min + g.reach.Max) * 0.5
	return h * h
}

// flat2 is the flat squared distance from the mob to the target's vehicle,
// or the target.
func (g *UseKineticWeapon) flat2(t Actor) float32 {
	if v := g.actor(t.Vehicle()); v != nil {
		t = v
	}
	p, q := g.m.Pos(), t.Pos()
	return (q.X-p.X)*(q.X-p.X) + (q.Z-p.Z)*(q.Z-p.Z)
}

// Tick implements goal.Goal.
func (g *UseKineticWeapon) Tick() {
	t := g.actor(g.target)
	if t == nil {
		return
	}
	switch g.state {
	case kineticApproach:
		if a := g.kcfg.ApproachDistance; a*a <= g.flat2(t) {
			g.step(t, false, g.flat2(t))
			return
		}
		g.hasSpot = false
		g.state = kineticCharge
		g.readWeapon()
		g.k.StartKineticCharge(g.kcfg.WeaponMinSpeedMultiplier)
	case kineticCharge:
		if !t.Alive() || !g.k.KineticCharging() {
			g.cooldown()
			return
		}
		if g.mid2() <= g.flat2(t) {
			g.step(t, false, dist2(g.m.Pos(), t.Pos()))
			return
		}
		// 0x56A0BB0: run past the target.
		g.state = kineticReposition
		g.past(t, g.kcfg.RepositionDistance)
		g.walk()
	case kineticReposition:
		if !t.Alive() || !g.k.KineticCharging() {
			g.cooldown()
			return
		}
		g.follow()
		if !g.pathDone() {
			return
		}
		if g.mid2() <= dist2(g.m.Pos(), t.Pos()) {
			g.hasSpot = false
			g.state = kineticApproach
			return
		}
		g.wander(g.kcfg.RepositionDistance)
	case kineticCooldown:
		g.follow()
		over := !g.hasEnd || g.world.CurrentTick() > g.cooldownEnd
		done := g.pathDone()
		switch {
		case done && over:
			g.hasSpot = false
			g.state = kineticApproach
		case done:
			g.wander(g.kcfg.CooldownDistance)
		}
	}
}

// cooldown is 0x56A1090: the attack ends and the mob runs past the target
// for the melee cooldown.
func (g *UseKineticWeapon) cooldown() {
	g.state = kineticCooldown
	g.cooldownEnd, g.hasEnd = g.world.CurrentTick()+uint64(max(g.cfg.Cooldown, 0)), true
	g.k.StopKineticCharge()
	if t := g.actor(g.target); t != nil {
		g.past(t, g.kcfg.CooldownDistance)
	}
	g.walk()
}

// pathDone is the walk's end test. BDS also ends it when the path made no
// progress for 21 ticks; the Navigator does not report that yet.
func (g *UseKineticWeapon) pathDone() bool {
	n := g.m.Nav()
	return n == nil || n.Done()
}

// walk counts the repath timer down and walks to the spot; a failed path
// adds the fail increase.
func (g *UseKineticWeapon) walk() {
	g.repath--
	n := g.m.Nav()
	if n == nil || !n.MoveTo(vecOf(g.spot), g.kcfg.RepositionSpeedMultiplier) {
		g.repath += g.cfg.FailIncrease
	}
}

// follow is 0x56A0F10: when the repath timer runs out the walk to the spot
// is made again, unless a path still runs and the roll keeps it. The timer
// is not reset.
func (g *UseKineticWeapon) follow() {
	old := g.repath
	g.repath--
	if old > 1 {
		return
	}
	if g.hasSpot && !g.pathDone() && g.world.Random().NextFloat() >= kineticKeepPath {
		return
	}
	n := g.m.Nav()
	if n == nil || !n.MoveTo(vecOf(g.spot), g.kcfg.RepositionSpeedMultiplier) {
		g.repath += g.cfg.FailIncrease
	}
}

// past is 0x56A0C40: a block a random distance in r from the target, meant
// to lie beyond it in the direction from the mob turned by up to half
// melee_fov. BDS turns the angle, already in radians, from degrees once
// more, so the spot ends up close to +z of the target whatever the
// direction.
func (g *UseKineticWeapon) past(t Actor, r Range[float32]) {
	r1 := g.world.Random()
	p, q := g.m.AttachPos(0), t.AttachPos(0)
	dx, dy, dz := q.X-p.X, q.Y-p.Y, q.Z-p.Z
	l := float32(math.Sqrt(float64(dz*dz + dx*dx + dy*dy)))
	var nx, nz float32
	if l >= kineticMinLength {
		nx, nz = dx/l, dz/l
	}
	a := float32(math.Atan2(float64(nz), float64(nx)))
	h := g.cfg.MeleeFOV * radPerDeg * 0.5
	a = (h+h)*r1.NextFloat() - h + a
	d := (r.Max-r.Min)*r1.NextFloat() + r.Min
	i := (-math.Pi - a*radPerDeg) * sinScale
	g.spot = Pos{
		X: floorInt(q.X - tableSin(uint16(int32(i)))*d),
		Y: floorInt(q.Y),
		Z: floorInt(q.Z - tableSin(uint16(int32(i+sinQuarter)))*d),
	}
	g.hasSpot = true
}

// wander is 0x56A1200: a random land spot around the mob, its ranges the
// distance's ends plus the reach's middle, then the walk there.
func (g *UseKineticWeapon) wander(r Range[float32]) {
	h := (g.reach.Min + g.reach.Max) * 0.5
	p, ok := landPos(g.world, g.m, int32(r.Min+h), int32(r.Max+h))
	if !ok {
		return
	}
	g.spot, g.hasSpot = blockOf(p), true
	g.walk()
}
