package behavior

import "math"

// The ranged_attack goal's fixed numbers.
const (
	rangedLoadedSound = 44 // the crossbow loaded sound
	rangedBackOff     = 0.5
	rangedHoldPad     = 0.5 // how far past attack_radius mode 1 stops walking
	rangedMinPower    = 0.1
	rangedMaxPower    = 1
	// rangedWalkIn is the in-range movement mode that keeps walking until
	// the target is within attack_radius + 0.5.
	rangedWalkIn = 1
)

// Ranged is the part of the mob ranged_attack needs on top of Targeter.
type Ranged interface {
	Targeter
	// Swing swings the mob's arm.
	Swing()
	// Crossbow reports a crossbow in the mob's hand and whether it holds a
	// charged item.
	Crossbow() (held, charged bool)
	// UnchargeCrossbow takes the charged item out of the held crossbow.
	UnchargeCrossbow()
	// ChargeAmount and SetChargeAmount are the charge amount actor data
	// (item 22).
	ChargeAmount() int8
	SetChargeAmount(v int8)
	// Shoot fires ShooterComponent's projectile; nothing without one.
	Shoot()
	// RestartAngry restarts AngryComponent's duration; nothing without one.
	RestartAngry()
	// RangedAttack is Mob::performRangedAttack with a power of 0.1 to 1.
	RangedAttack(t Actor, power float32)
}

// RangedAttackConfig is the JSON of minecraft:behavior.ranged_attack.
// Times are in ticks.
type RangedAttackConfig struct {
	BurstShots        int32 `json:"burst_shots"`
	TargetInSightTime int32
	BurstInterval     int32
	// ChargeChargedTrigger and ChargeShootTrigger are the crossbow charge
	// ticks; a shoot trigger above 0 makes the goal charge a crossbow.
	ChargeChargedTrigger int32
	ChargeShootTrigger   int32
	// AttackIntervalMin and AttackIntervalMax are 0 when the JSON leaves
	// them out.
	AttackIntervalMin int32
	AttackIntervalMax int32
	AttackRadiusMin   float32 `json:"attack_radius_min"`
	AttackRadius      float32 `json:"attack_radius"`
	// XMaxRotation and YMaxHeadRotation are the look control's speeds
	// (map?: which JSON field feeds which).
	XMaxRotation     float32 `json:"x_max_rotation"`
	YMaxHeadRotation float32 `json:"y_max_head_rotation"`
	RangedFOV        float32 `json:"ranged_fov"`
	SpeedMultiplier  float32 `json:"speed_multiplier"`
	SetPersistent    bool    `json:"set_persistent"`
	Swing            bool    `json:"swing"`
	// InRangeMovement is the in-range movement mode; 1 keeps walking
	// until the target is within attack_radius + 0.5.
	InRangeMovement int32
}

// DefaultRangedAttack is the goal's BDS defaults.
var DefaultRangedAttack = RangedAttackConfig{
	BurstShots: 1, TargetInSightTime: 20, XMaxRotation: 30, YMaxHeadRotation: 30, RangedFOV: 90, SpeedMultiplier: 1,
}

// RangedAttack is RangedAttackGoal: the mob keeps its distance, faces the
// target and shoots in bursts, or charges and fires a crossbow.
type RangedAttack struct {
	base
	m          Ranged
	cfg        RangedAttackConfig
	crossbow   bool
	wasCharged bool
	target     ID
	seen       int32
	timer      int32
	burst      int32
	burstTimer int32
	charge     int32
}

// NewRangedAttack makes the goal.
func NewRangedAttack(w World, m Ranged, c RangedAttackConfig) *RangedAttack {
	return &RangedAttack{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c,
		crossbow: c.ChargeShootTrigger > 0, target: NoID}
}

// CanUse implements goal.Goal (0x5923950).
func (g *RangedAttack) CanUse() bool {
	if g.m.Admiring() || g.m.Nav() == nil {
		return false
	}
	if held, charged := g.m.Crossbow(); held && !charged {
		return false
	}
	t := g.actor(g.m.Target())
	if t == nil || t.Removed() {
		return false
	}
	g.target = t.ID()
	if g.cfg.SetPersistent && t.IsPlayer() {
		g.m.SetPersistent()
	}
	return true
}

// CanContinueToUse implements goal.Goal (0x5923BB0): BDS runs canUse even
// while a path is still being followed.
func (g *RangedAttack) CanContinueToUse() bool {
	if g.m.Admiring() {
		return false
	}
	n := g.m.Nav()
	walking := n != nil && !n.Done()
	return g.CanUse() || walking
}

// Start implements goal.Goal (0x5923DB0).
func (g *RangedAttack) Start() {
	held, charged := g.m.Crossbow()
	g.wasCharged = held && charged
	g.m.SetFlag(FlagRangedAttacking, true)
}

// Stop implements goal.Goal (0x5923E20).
func (g *RangedAttack) Stop() {
	g.target = NoID
	g.seen, g.charge = 0, 0
	if !g.wasCharged {
		if g.m.ChargeAmount() != 0 {
			g.m.SetChargeAmount(0)
		}
		g.m.SetFlag(FlagCharged, false)
	}
	g.wasCharged = false
	g.m.SetFlag(FlagRangedAttacking, false)
}

// interval is the attack interval for the distance: attack_interval_min to
// _max scaled by how far into attack_radius the target is.
func (g *RangedAttack) interval(d2 float32) int32 {
	v := float32(g.cfg.AttackIntervalMin) + float32(g.cfg.AttackIntervalMax-g.cfg.AttackIntervalMin)*(sqrt32(d2)/g.cfg.AttackRadius)
	return int32(math.Floor(float64(v)))
}

// moverSpeed is the movement speed of the entity that moves the mob: its
// lowest vehicle when it rides (map?: 0 for a vehicle that is not a mob).
func (g *RangedAttack) moverSpeed() float32 {
	r := rootVehicle(g.world, g.m)
	if r.ID() == g.m.ID() {
		return g.m.Speed()
	}
	if v, ok := r.(Mob); ok && r.IsMob() {
		return v.Speed()
	}
	return 0
}

// Tick implements goal.Goal (0x5923F60).
func (g *RangedAttack) Tick() {
	t := g.actor(g.target)
	if t == nil || t.Removed() {
		return
	}
	p, q := g.m.Pos(), t.Pos()
	dx, dy, dz := p.X-q.X, p.Y-t.Box().Min.Y, p.Z-q.Z
	d2 := dz*dz + (dx*dx + dy*dy)
	seen := g.m.CanSee(t)
	if seen {
		g.seen++
	} else {
		g.seen = 0
	}
	g.move(t, d2)
	g.m.LookAt(lookPoint(t), g.cfg.XMaxRotation, g.cfg.YMaxHeadRotation)
	if g.burst <= 0 {
		g.load(d2, seen)
		return
	}
	g.fire(t, d2, seen)
}

// move backs away from a target inside attack_radius_min, holds still in
// range, and otherwise walks to the target.
func (g *RangedAttack) move(t Actor, d2 float32) {
	n, speed := g.m.Nav(), g.moverSpeed()
	r := g.cfg.AttackRadius
	switch {
	case d2 < g.cfg.AttackRadiusMin*g.cfg.AttackRadiusMin:
		if n == nil || speed <= 0 {
			return
		}
		k := (g.cfg.AttackRadiusMin - sqrt32(d2)) + rangedBackOff
		p, q := g.m.Pos(), t.Pos()
		a := float32(math.Atan2(float64(p.Z-q.Z), float64(p.X-q.X))) * sinScale
		// BDS takes x from the sine and z from the cosine.
		n.MoveTo(Vec3{X: tableSin(uint16(int32(a)))*k + p.X, Y: p.Y, Z: k*tableSin(uint16(int32(a+sinQuarter))) + p.Z}, g.cfg.SpeedMultiplier)
	case d2 <= r*r && g.seen >= g.cfg.TargetInSightTime:
		var hold float32
		if r > 0 {
			hold = r + rangedHoldPad
		}
		if g.cfg.InRangeMovement == rangedWalkIn && d2 > hold*hold {
			return
		}
		if n != nil {
			n.Stop()
		}
		if _, ok := g.m.MoveWanted(); !ok {
			return
		}
		g.m.ClearMoveWanted()
		if g.m.Vehicle() == NoID {
			p, q := g.m.Pos(), t.Pos()
			g.m.SetYaw(float32(math.Atan2(float64(q.X-p.X), float64(q.Z-p.Z))) * -degPerRad)
		}
	default:
		if n != nil && speed > 0 {
			n.MoveToActor(t, g.cfg.SpeedMultiplier)
		}
	}
}

// load counts down to the next burst, or charges the crossbow.
func (g *RangedAttack) load(d2 float32, seen bool) {
	if !g.crossbow {
		c := g.timer
		g.timer--
		switch {
		case c == 1:
			g.burst = g.cfg.BurstShots
		case c <= 0:
			g.timer = g.interval(d2)
		}
		return
	}
	if seen {
		g.charge++
		if g.charge >= g.cfg.ChargeShootTrigger {
			g.burst = g.cfg.BurstShots
			g.charge -= 2 * g.cfg.ChargeShootTrigger
		}
	} else {
		g.charge--
	}
	charged := g.charge > g.cfg.ChargeChargedTrigger
	var v int8
	if charged {
		v = 1
	}
	if g.m.ChargeAmount() != v {
		g.m.SetChargeAmount(v)
	}
	g.m.SetFlag(FlagCharged, charged)
	if g.charge == g.cfg.ChargeChargedTrigger {
		g.m.PlaySoundAt(rangedLoadedSound, g.m.Eyes())
	}
}

// fire shoots once the burst timer is up and the target is in range, seen
// and in the field of view.
func (g *RangedAttack) fire(t Actor, d2 float32, seen bool) {
	b := g.burstTimer
	g.burstTimer--
	r, lo := g.cfg.AttackRadius, g.cfg.AttackRadiusMin
	if !seen || d2 > r*r || d2 < lo*lo || b >= 2 || !inFOV(g.m, t.Pos(), g.cfg.RangedFOV) {
		return
	}
	g.m.Shoot()
	if !g.crossbow {
		g.m.RestartAngry()
		f := sqrt32(d2) / r
		g.timer = g.interval(d2)
		power := max32(f, rangedMinPower)
		if f > rangedMaxPower {
			power = rangedMaxPower
		}
		g.m.RangedAttack(t, power)
		if g.cfg.Swing {
			g.m.Swing()
		}
	}
	left := g.burst
	g.burst--
	if left >= 2 {
		g.burstTimer = g.cfg.BurstInterval
		return
	}
	if g.wasCharged {
		g.m.UnchargeCrossbow()
	}
}
