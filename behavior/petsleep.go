package behavior

import "math"

// The pet_sleep_with_owner numbers.
const (
	petSleepReach    = 0.1 // the walk's last node reach, capped by goal_radius
	petSleepHop      = 20  // ticks of the hop onto the bed
	petSleepTurn     = 30  // most degrees the pet turns per hop tick
	petSleepDamp     = 0.5 // velocity kept per hop tick
	petSleepSlowdown = 0.5 // speed share once the path's last node is reached
	navExactEnd      = 0x8000
)

// The pet_sleep_with_owner states (0xEEE6D70, 0xEEE6DA8, 0xEEE6DE0,
// 0xEEE6E18).
const (
	petSleepWalk = iota
	petSleepApproach
	petSleepHopping
	petSleepSleeping
)

// PetSleeper is the part of the mob pet_sleep_with_owner needs on top of
// Mob.
type PetSleeper interface {
	Mob
	// OwnerBed is the bed block the sleeping owner lies in; ok is false
	// when the owner is not a player asleep in a bed.
	OwnerBed() (bed Pos, ok bool)
	// OwnerSleeping reports the owner player asleep (Actor vt+336).
	OwnerSleeping() bool
	// ClaimBed gives the bed's block entity the pet; false when another pet
	// has it.
	ClaimBed(bed Pos) bool
	// TellOwner sends the owner the pet-sleeps event (event 14).
	TellOwner()
	// SetExactPathEnd sets the navigation's exact end switch (+0x8000)
	// and the squared reach of the last node.
	SetExactPathEnd(reach2 float32)
	// SetPathSpeed is the speed of the path being followed.
	SetPathSpeed(s float32)
	// Velocity and SetVelocity are the mob's motion.
	Velocity() Vec3
	SetVelocity(v Vec3)
	// Yaw is ActorRotationComponent's yaw; SetYaw sets it.
	Yaw() float32
	SetYaw(yaw float32)
	// SetBodyYaw sets the body rotation (MobBodyRotationComponent).
	SetBodyYaw(yaw float32)
}

// PetSleepWithOwnerConfig is the JSON of
// minecraft:behavior.pet_sleep_with_owner.
type PetSleepWithOwnerConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	SearchRange     int32   `json:"search_range"`
	SearchHeight    int32   `json:"search_height"`
	GoalRadius      float32 `json:"goal_radius"`
}

// DefaultPetSleepWithOwner returns the goal's BDS defaults (0x5755A70).
func DefaultPetSleepWithOwner() PetSleepWithOwnerConfig {
	return PetSleepWithOwnerConfig{SpeedMultiplier: 1, SearchHeight: 1, GoalRadius: 0.5}
}

// PetSleepWithOwner is PetSleepWithOwnerGoal: a tamed pet walks to its
// sleeping owner's bed, hops on and lies down.
type PetSleepWithOwner struct {
	base
	m      PetSleeper
	cfg    PetSleepWithOwnerConfig
	target Vec3
	state  int
	hop    int32
}

// NewPetSleepWithOwner makes the goal.
func NewPetSleepWithOwner(w World, m PetSleeper, c PetSleepWithOwnerConfig) *PetSleepWithOwner {
	return &PetSleepWithOwner{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x5755C90). BDS fails when the pet is
// closer than search_range, checked in the assembly.
func (g *PetSleepWithOwner) CanUse() bool {
	m := g.m
	bed, ok := m.OwnerBed()
	if !ok || !m.Flag(FlagTamed) || m.Flag(FlagSitting) || m.Nav() == nil {
		return false
	}
	o := g.actor(m.Owner())
	if o == nil {
		return false
	}
	p, op := m.Pos(), o.Pos()
	if abs32(p.Y-op.Y) > float32(g.cfg.SearchHeight) {
		return false
	}
	dx, dz := p.X-op.X, p.Z-op.Z
	if float32(g.cfg.SearchRange*g.cfg.SearchRange) > dz*dz+dx*dx {
		return false
	}
	if !m.ClaimBed(bed) {
		return false
	}
	g.target = Vec3{X: float32(bed.X) + 0.5, Y: float32(bed.Y) + 0.5, Z: float32(bed.Z) + 0.5}
	return true
}

// CanContinueToUse implements goal.Goal (0x5755F90).
func (g *PetSleepWithOwner) CanContinueToUse() bool {
	return g.actor(g.m.Owner()) != nil && !g.m.Flag(FlagSitting) && g.m.OwnerSleeping()
}

// Start implements goal.Goal: the walk state's start (0x57563D0).
func (g *PetSleepWithOwner) Start() {
	m := g.m
	if n := m.Nav(); n != nil && n.MoveTo(g.target, g.cfg.SpeedMultiplier) {
		r := min(petSleepReach, g.cfg.GoalRadius)
		m.SetExactPathEnd(r * r)
	}
	m.TellOwner()
	g.state = petSleepApproach
}

// Stop implements goal.Goal: every state stops the navigation and goes
// back to walking (0x5756090); the sleeping one first gets up.
func (g *PetSleepWithOwner) Stop() {
	m := g.m
	if g.state == petSleepSleeping {
		m.SetFlag(FlagResting, false)
	}
	if n := m.Nav(); n != nil {
		n.Stop()
	}
	g.state = petSleepWalk
}

// Tick implements goal.Goal.
func (g *PetSleepWithOwner) Tick() {
	switch g.state {
	case petSleepApproach:
		g.slowAtEnd()
		if g.cfg.GoalRadius > g.gap2() {
			g.state, g.hop = petSleepHopping, petSleepHop
		}
	case petSleepHopping:
		g.hopTick()
	}
}

// slowAtEnd is 0x5756210: half speed once the mob stands on the path's
// last node.
func (g *PetSleepWithOwner) slowAtEnd() {
	n := g.m.Nav()
	if n == nil || n.Done() {
		return
	}
	if end, ok := n.PathEnd(); ok && end == blockOf(g.m.Pos()) {
		g.m.SetPathSpeed(g.cfg.SpeedMultiplier * petSleepSlowdown)
	}
}

// gap2 is the squared distance from the box's middle height to the bed.
func (g *PetSleepWithOwner) gap2() float32 {
	p, b := g.m.Pos(), g.m.Box()
	mid := (b.Max.Y-b.Min.Y)*0.5 + b.Min.Y
	dx, dy, dz := g.target.X-p.X, g.target.Y-mid, g.target.Z-p.Z
	return dz*dz + (dx*dx + dy*dy)
}

// hopTick is 0x5756730: for the first three quarters the pet slows down
// and turns to the bed, then it waits and lies down.
func (g *PetSleepWithOwner) hopTick() {
	m := g.m
	g.slowAtEnd()
	if g.hop <= petSleepHop/4 {
		if g.hop > 0 {
			g.hop--
			return
		}
		if n := m.Nav(); n != nil {
			n.Stop()
		}
		m.SetFlag(FlagResting, true)
		g.state = petSleepSleeping
		return
	}
	g.hop--
	v := m.Velocity()
	m.SetVelocity(Vec3{X: v.X * petSleepDamp, Y: v.Y * petSleepDamp, Z: v.Z * petSleepDamp})
	p := m.Pos()
	face := float32(math.Atan2(float64(g.target.Z-p.Z), float64(g.target.X-p.X)))*degPerRad - 90
	m.SetYaw(approachDegrees(m.Yaw(), face, petSleepTurn))
	m.SetBodyYaw(approachDegrees(m.BodyYaw(), face, petSleepTurn))
}
