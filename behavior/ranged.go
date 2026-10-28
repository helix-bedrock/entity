package behavior

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
)

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

// RangedAttackConfig is the JSON of minecraft:behavior.ranged_attack; its
// UnmarshalJSON reads both JSON formats. Times are in ticks.
type RangedAttackConfig struct {
	BurstShots        int32
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
	// AttackRadiusMin and AttackRadius are attack_range's min and max
	// (attack_radius_min and attack_radius in the old format).
	AttackRadiusMin float32
	AttackRadius    float32
	// XMaxRotation and YMaxHeadRotation are the look control's speeds
	// (map?: which JSON field feeds which).
	XMaxRotation     float32
	YMaxHeadRotation float32
	RangedFOV        float32
	SpeedMultiplier  float32
	SetPersistent    bool
	Swing            bool
	// InRangeMovement is the in-range movement mode; 1 keeps walking
	// until the target is within attack_radius + 0.5.
	InRangeMovement int32
}

// rangedMoveModes are the in_range_movement_mode names, in BDS's order.
func rangedMoveModes() []string { return []string{"hold_position", "follow_target"} }

// rangedJSON is the JSON of minecraft:behavior.ranged_attack in both of the
// formats BDS 1.26.60 reads: the old attack_interval_min/_max and
// attack_radius_min/attack_radius fields, and the attack_interval and
// attack_range {min, max} objects vanilla uses since 1.26.60. Times are in
// seconds.
type rangedJSON struct {
	BurstShots           *int32          `json:"burst_shots"`
	TargetInSightTime    *float32        `json:"target_in_sight_time"`
	BurstInterval        *float32        `json:"burst_interval"`
	ChargeChargedTrigger *float32        `json:"charge_charged_trigger"`
	ChargeShootTrigger   *float32        `json:"charge_shoot_trigger"`
	AttackInterval       json.RawMessage `json:"attack_interval"`
	AttackIntervalMin    *float32        `json:"attack_interval_min"`
	AttackIntervalMax    *float32        `json:"attack_interval_max"`
	AttackRange          *rangeJSON      `json:"attack_range"`
	AttackRadiusMin      *float32        `json:"attack_radius_min"`
	AttackRadius         *float32        `json:"attack_radius"`
	XMaxRotation         *float32        `json:"x_max_rotation"`
	YMaxHeadRotation     *float32        `json:"y_max_head_rotation"`
	RangedFOV            *float32        `json:"ranged_fov"`
	SpeedMultiplier      *float32        `json:"speed_multiplier"`
	SetPersistent        *bool           `json:"set_persistent"`
	Swing                *bool           `json:"swing"`
	InRangeMovementMode  *string         `json:"in_range_movement_mode"`
}

// rangeJSON is a {min, max} object; a missing side keeps its old value.
type rangeJSON struct {
	Min *float32 `json:"min"`
	Max *float32 `json:"max"`
}

// rangedUnset is the value BDS keeps in attack_interval's two sides until the
// JSON sets them; it reads as 0.
const rangedUnset = -1

// UnmarshalJSON reads minecraft:behavior.ranged_attack the way BDS 1.26.60
// does (definition 0xDFECB30, old format 0xE1DCB20, checks 0x5AE4630, goal
// setup 0x58AF3C0 in 1.26.52). Fields the JSON leaves out keep the values c
// already has, so decode into DefaultRangedAttack().
func (c *RangedAttackConfig) UnmarshalJSON(b []byte) error {
	var j rangedJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return fmt.Errorf("behavior: read ranged_attack: %w", err)
	}
	if j.BurstShots != nil {
		c.BurstShots = *j.BurstShots
	}
	setTicks(&c.TargetInSightTime, j.TargetInSightTime)
	setTicks(&c.BurstInterval, j.BurstInterval)
	setTicks(&c.ChargeChargedTrigger, j.ChargeChargedTrigger)
	setTicks(&c.ChargeShootTrigger, j.ChargeShootTrigger)
	if err := c.readInterval(&j); err != nil {
		return err
	}
	if j.AttackRange != nil {
		setFloat(&c.AttackRadiusMin, j.AttackRange.Min)
		setFloat(&c.AttackRadius, j.AttackRange.Max)
	}
	setFloat(&c.AttackRadiusMin, j.AttackRadiusMin)
	setFloat(&c.AttackRadius, j.AttackRadius)
	setFloat(&c.XMaxRotation, j.XMaxRotation)
	setFloat(&c.YMaxHeadRotation, j.YMaxHeadRotation)
	setFloat(&c.RangedFOV, j.RangedFOV)
	setFloat(&c.SpeedMultiplier, j.SpeedMultiplier)
	if j.SetPersistent != nil {
		c.SetPersistent = *j.SetPersistent
	}
	if j.Swing != nil {
		c.Swing = *j.Swing
	}
	if j.InRangeMovementMode != nil {
		i := slices.Index(rangedMoveModes(), *j.InRangeMovementMode)
		if i < 0 {
			return fmt.Errorf("behavior: ranged_attack in_range_movement_mode %q is not one of %v", *j.InRangeMovementMode, rangedMoveModes())
		}
		c.InRangeMovement = int32(i)
	}
	return nil
}

// readInterval reads attack_interval as a number (both sides, old format) or
// a {min, max} object, then attack_interval_min and _max. An unset side is 0
// and a max below the min is raised to the min, as BDS's load check does.
func (c *RangedAttackConfig) readInterval(j *rangedJSON) error {
	if j.AttackInterval == nil && j.AttackIntervalMin == nil && j.AttackIntervalMax == nil {
		return nil
	}
	lo, hi := float32(rangedUnset), float32(rangedUnset)
	if j.AttackInterval != nil {
		var r rangeJSON
		var v float32
		switch {
		case json.Unmarshal(j.AttackInterval, &v) == nil:
			lo, hi = v, v
		case json.Unmarshal(j.AttackInterval, &r) == nil:
			setFloat(&lo, r.Min)
			setFloat(&hi, r.Max)
		default:
			return fmt.Errorf("behavior: ranged_attack attack_interval %s is not a number or a {min, max} object", j.AttackInterval)
		}
	}
	setFloat(&lo, j.AttackIntervalMin)
	setFloat(&hi, j.AttackIntervalMax)
	if lo == rangedUnset {
		lo = 0
	}
	if hi == rangedUnset {
		hi = 0
	}
	if lo > hi {
		hi = lo
	}
	c.AttackIntervalMin = int32(lo * ticksPerSecond)
	c.AttackIntervalMax = int32(hi * ticksPerSecond)
	return nil
}

// setTicks stores a time in seconds as ticks, truncated as BDS does.
func setTicks(dst *int32, s *float32) {
	if s != nil {
		*dst = int32(*s * ticksPerSecond)
	}
}

func setFloat(dst *float32, v *float32) {
	if v != nil {
		*dst = *v
	}
}

// DefaultRangedAttack returns the goal's BDS defaults.
func DefaultRangedAttack() RangedAttackConfig {
	return RangedAttackConfig{
		BurstShots: 1, TargetInSightTime: 20, XMaxRotation: 30, YMaxHeadRotation: 30, RangedFOV: 90, SpeedMultiplier: 1,
	}
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
