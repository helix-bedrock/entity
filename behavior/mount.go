package behavior

// The mount goals' fixed numbers.
const (
	mountRepath     = 7 // mount_pathing repaths every 4 to 10 ticks
	mountRepathBase = 4
	mountLookMax    = 30
	crazyXZ         = 5 // run_around_like_crazy's random position range
	crazyY          = 4
)

// MountPathingConfig is the JSON of minecraft:behavior.mount_pathing.
type MountPathingConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// TargetDist is target_dist, added to the squared reach.
	TargetDist  float32 `json:"target_dist"`
	TrackTarget bool    `json:"track_target"`
}

// DefaultMountPathing returns the goal's BDS defaults.
func DefaultMountPathing() MountPathingConfig {
	return MountPathingConfig{SpeedMultiplier: 1}
}

// MountPathing is MountPathingGoal: a mount carrying a mob walks toward its
// rider's target.
type MountPathing struct {
	base
	cfg    MountPathingConfig
	target ID
	repath int32
}

// NewMountPathing makes the goal.
func NewMountPathing(w World, m Mob, c MountPathingConfig) *MountPathing {
	return &MountPathing{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c, target: NoID}
}

// riderTarget is the first rider's target when the rider is a mob that does
// not steer the mount itself.
func (g *MountPathing) riderTarget() Actor {
	r := g.mob.Riders()
	if len(r) == 0 {
		return nil
	}
	a := g.actor(r[0])
	if a == nil || !a.IsMob() || a.HijacksMount() {
		return nil
	}
	return g.actor(a.Target())
}

// CanUse implements goal.Goal.
func (g *MountPathing) CanUse() bool {
	n := g.mob.Nav()
	if n == nil {
		return false
	}
	t := g.riderTarget()
	if t == nil {
		return false
	}
	g.target = t.ID()
	g.repath = g.world.IRandom().NextInt(mountRepath) + mountRepathBase
	return n.MoveToActor(t, g.cfg.SpeedMultiplier)
}

// CanContinueToUse implements goal.Goal.
func (g *MountPathing) CanContinueToUse() bool {
	if g.riderTarget() == nil {
		return false
	}
	t := g.actor(g.target)
	if t == nil || !t.Alive() {
		return false
	}
	if g.cfg.TrackTarget {
		return true
	}
	n := g.mob.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal.
func (g *MountPathing) Start() {
	g.mob.SetHijackable(true)
	g.repath = 0
}

// Stop implements goal.Goal.
func (g *MountPathing) Stop() {
	g.target = NoID
	g.mob.SetHijackable(false)
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
}

// Tick implements goal.Goal.
func (g *MountPathing) Tick() {
	if g.riderTarget() == nil {
		return
	}
	t := g.actor(g.target)
	if t == nil {
		return
	}
	g.mob.LookAt(lookPoint(t), mountLookMax, mountLookMax)
	if !g.cfg.TrackTarget {
		s, ok := g.mob.(CanSeer)
		if !ok || !s.CanSee(t) {
			return
		}
	}
	g.repath--
	if g.repath+1 > 1 {
		return
	}
	g.repath = g.world.IRandom().NextInt(mountRepath) + mountRepathBase
	b, tb := g.mob.Box(), t.Box()
	w := b.Max.X - b.Min.X
	reach := (w+w)*(w+w) + (tb.Max.X - tb.Min.X) + g.cfg.TargetDist
	p := g.mob.Pos()
	c := Vec3{X: (tb.Max.X-tb.Min.X)*0.5 + tb.Min.X, Y: tb.Min.Y, Z: (tb.Max.Z-tb.Min.Z)*0.5 + tb.Min.Z}
	if reach <= dist2(p, c) {
		if n := g.mob.Nav(); n != nil {
			n.MoveToActor(t, g.cfg.SpeedMultiplier)
		}
	}
}

// RunAroundLikeCrazy is RunAroundLikeCrazyGoal: an untamed horse with a
// player on its back runs to random spots nearby.
type RunAroundLikeCrazy struct {
	base
	speed float32
	to    Vec3
}

// NewRunAroundLikeCrazy makes the goal; speed is speed_multiplier, 1 by
// default.
func NewRunAroundLikeCrazy(w World, m Mob, speed float32) *RunAroundLikeCrazy {
	return &RunAroundLikeCrazy{base: base{mob: m, world: w, flags: flagsMove}, speed: speed}
}

// CanUse implements goal.Goal.
func (g *RunAroundLikeCrazy) CanUse() bool {
	m := g.mob
	if m.Nav() == nil || m.Flag(FlagTamed) {
		return false
	}
	r := m.Riders()
	if len(r) == 0 {
		return false
	}
	if a := g.actor(r[0]); a != nil && !a.IsPlayer() {
		return false
	}
	p, ok := landPos(g.world, m, crazyXZ, crazyY)
	g.to = p
	return ok
}

// CanContinueToUse implements goal.Goal.
func (g *RunAroundLikeCrazy) CanContinueToUse() bool {
	n := g.mob.Nav()
	return n != nil && !n.Done() && len(g.mob.Riders()) != 0
}

// Start implements goal.Goal.
func (g *RunAroundLikeCrazy) Start() {
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.to, g.speed)
	}
}

// Stop implements goal.Goal.
func (g *RunAroundLikeCrazy) Stop() {}

// Tick implements goal.Goal.
func (g *RunAroundLikeCrazy) Tick() {}
