package behavior

import "github.com/helix-bedrock/entity/path"

// VexCopyOwnerTarget is VexCopyOwnerTargetGoal (vtable 0xFAEF5A8): the mob
// takes its owner's target as its own.
type VexCopyOwnerTarget struct {
	target
	c OwnerTargetCopier
}

// OwnerTargetCopier is the mob vex_copy_owner_target runs on.
type OwnerTargetCopier interface {
	Targeter
	// IgnoresCannotBeAttacked reports IgnoreCannotBeAttackedComponent on
	// the mob with filters that pass for a.
	IgnoresCannotBeAttacked(a Actor) bool
}

// NewVexCopyOwnerTarget makes the goal. It reads nothing from its JSON
// beyond the target goal defaults.
func NewVexCopyOwnerTarget(w World, m OwnerTargetCopier) *VexCopyOwnerTarget {
	return &VexCopyOwnerTarget{target: target{base: base{mob: m, world: w, flags: flagsMove}, m: m,
		forget: DefaultNearestAttackable().MustSeeForget}, c: m}
}

// ownerTarget is the owner's target, when both are there.
func (g *VexCopyOwnerTarget) ownerTarget() Actor {
	o := g.actor(g.m.Owner())
	if o == nil {
		return nil
	}
	return g.actor(o.Target())
}

// copyable is the narrow target test the goal calls (0xB399B30): no
// cannot-target flag, and a target that cannot be attacked only when the
// mob ignores that. It does not run the owner, team or reach checks of the
// other target goals.
func (g *VexCopyOwnerTarget) copyable(a Actor) bool {
	return a != nil && !a.Flag(FlagCannotTarget) && (a.Targetable() || g.c.IgnoresCannotBeAttacked(a))
}

// CanUse implements goal.Goal (0xBF32740).
func (g *VexCopyOwnerTarget) CanUse() bool { return g.copyable(g.ownerTarget()) }

// CanContinueToUse implements goal.Goal (the target goal's, 0xBF30AE0).
func (g *VexCopyOwnerTarget) CanContinueToUse() bool { return g.keep(g.actor(g.m.Target())) }

// Start implements goal.Goal (0xBF327C0): unlike the other target goals it
// keeps the sight and reach caches.
func (g *VexCopyOwnerTarget) Start() {
	if a := g.ownerTarget(); a != nil {
		g.m.SetTarget(a.ID())
	}
}

// Stop implements goal.Goal.
func (g *VexCopyOwnerTarget) Stop() {}

// Tick implements goal.Goal.
func (g *VexCopyOwnerTarget) Tick() {}

// Bounder is a mob that may have a bound origin (Mob +1081, the spot a
// summoned vex stays around).
type Bounder interface {
	Mob
	BoundOrigin() (p Pos, ok bool)
}

// VexRandomMoveConfig is minecraft:behavior.vex_random_move. The JSON has
// no fields; these are the goal's fixed BDS numbers.
type VexRandomMoveConfig struct {
	// Chance is the 1 in Chance roll to start.
	Chance int32
	// Tries is how many random spots are tested for air.
	Tries int
	// Spread is the half size of the box the spots are drawn from.
	Spread Pos
	Speed  float32
	// LookYaw and LookPitch are the look control limits in degrees.
	LookYaw, LookPitch float32
}

// DefaultVexRandomMove returns the goal's BDS numbers (tick 0x56C4640).
func DefaultVexRandomMove() VexRandomMoveConfig {
	return VexRandomMoveConfig{Chance: 7, Tries: 3, Spread: Pos{X: 7, Y: 5, Z: 7}, Speed: 0.25, LookYaw: 180, LookPitch: 20}
}

// VexRandomMove is VexRandomMoveGoal (canUse 0x56C4530, tick 0x56C4640): a
// flying mob with a move control picks an air block near its bound origin
// and flies there, looking at it while it has no target.
type VexRandomMove struct {
	base
	m   Bounder
	cfg VexRandomMoveConfig
}

// NewVexRandomMove makes the goal.
func NewVexRandomMove(w World, m Bounder, c VexRandomMoveConfig) *VexRandomMove {
	return &VexRandomMove{base: base{mob: m, world: w, flags: flagsMove}, m: m, cfg: c}
}

// Config returns the goal's settings.
func (g *VexRandomMove) Config() VexRandomMoveConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *VexRandomMove) SetConfig(c VexRandomMoveConfig) { g.cfg = c }

// CanUse implements goal.Goal (0x56C4530): a move control with no wanted
// position, then a 1 in Chance roll on the level random.
func (g *VexRandomMove) CanUse() bool {
	wanted, ok := g.m.MoveWanted()
	if !ok || wanted {
		return false
	}
	return g.world.Random().NextInt(g.cfg.Chance) == 0
}

// CanContinueToUse implements goal.Goal: the goal runs for one tick.
func (g *VexRandomMove) CanContinueToUse() bool { return false }

// Start implements goal.Goal.
func (g *VexRandomMove) Start() {}

// Stop implements goal.Goal.
func (g *VexRandomMove) Stop() {}

// Tick implements goal.Goal (0x56C4640).
func (g *VexRandomMove) Tick() {
	o, ok := g.m.BoundOrigin()
	if !ok {
		o = blockOf(g.m.Pos())
	}
	r, s := g.world.Random(), g.cfg.Spread
	for range g.cfg.Tries {
		p := Pos{
			X: o.X + r.NextInt(2*s.X+1) - s.X,
			Y: o.Y + r.NextInt(2*s.Y+1) - s.Y,
			Z: o.Z + r.NextInt(2*s.Z+1) - s.Z,
		}
		// BDS compares the block with the air block; the material is the
		// engine-neutral form of that test.
		if g.world.Block(p).Material != path.MaterialAir {
			continue
		}
		at := Vec3{X: float32(p.X) + 0.5, Y: float32(p.Y) + 0.5, Z: float32(p.Z) + 0.5}
		g.m.SetMoveWanted(at, g.cfg.Speed)
		if g.actor(g.m.Target()) == nil {
			g.m.LookAt(at, g.cfg.LookYaw, g.cfg.LookPitch)
		}
		return
	}
}
