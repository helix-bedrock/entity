package behavior

// The stalk_and_pounce_on_target goal's fixed numbers.
const (
	stalkLookYaw      = 60
	stalkLookPitch    = 30
	stalkSamples      = 6 // points along the leap the clearance is checked at
	stalkClearHeight  = 3 // blocks above each point that must be free
	stalkParticleOdds = 0.2
	// stalkStuckEvent is the level event the stuck fox sends now and then
	// (block destroy particles).
	stalkStuckEvent = 2001
)

// stalkState is StalkAndPounceOnTargetGoal's state.
type stalkState uint8

const (
	stalkStalking stalkState = iota
	stalkInterested
	stalkLeaping
	stalkStuck
	stalkDone
)

// Pouncer is the part of the mob stalk_and_pounce_on_target needs on top
// of Attacker.
type Pouncer interface {
	Attacker
	// FaceActor is Mob::lookAt: turn the head and body towards a by at
	// most yawMax and pitchMax degrees.
	FaceActor(a Actor, yawMax, pitchMax float32)
	// JumpMultiplier is Mob::getJumpMultiplier.
	JumpMultiplier() float32
	// JumpPrevented emits the jump prevented event when the mob's jump is
	// blocked (Mob::getJumpPrevention, emitJumpPreventedEvent).
	JumpPrevented()
	// SetFallImmune adds or removes CurrentlyImmuneToFallDamageComponent.
	SetFallImmune(on bool)
}

// StalkAndPounceConfig is the JSON of
// minecraft:behavior.stalk_and_pounce_on_target. Distances are squared
// where noted and times in ticks.
type StalkAndPounceConfig struct {
	StalkSpeed float32 `json:"stalk_speed"`
	// MaxStalkDist, PounceMaxDist and StrikeDist are squared.
	MaxStalkDist  float32
	LeapHeight    float32 `json:"leap_height"`
	LeapDist      float32 `json:"leap_dist"`
	PounceMaxDist float32
	StrikeDist    float32
	InterestTime  int32
	StuckTime     int32
	SetPersistent bool `json:"set_persistent"`
	StuckBlocks   BlockFilter
}

// DefaultStalkAndPounce is the goal's BDS defaults.
var DefaultStalkAndPounce = StalkAndPounceConfig{
	StalkSpeed: 1.2, MaxStalkDist: 100, LeapHeight: 0.9, LeapDist: 0.8, PounceMaxDist: 25, StrikeDist: 4,
	InterestTime: 40, StuckTime: 40,
}

// StalkAndPounce is StalkAndPounceOnTargetGoal: the fox creeps up on its
// target, crouches, leaps on it, and may land stuck in snow.
type StalkAndPounce struct {
	base
	m     Pouncer
	cfg   StalkAndPounceConfig
	state stalkState
	until uint64
}

// NewStalkAndPounce makes the goal.
func NewStalkAndPounce(w World, m Pouncer, c StalkAndPounceConfig) *StalkAndPounce {
	return &StalkAndPounce{base: base{mob: m, world: w, flags: flagsMoveLook | flagsJump}, m: m, cfg: c, state: stalkDone}
}

// CanUse implements goal.Goal (0x5933BD0).
func (g *StalkAndPounce) CanUse() bool {
	if !g.m.OnGround() || g.m.Nav() == nil {
		return false
	}
	t := g.actor(g.m.Target())
	if t == nil {
		return false
	}
	d := dist2(t.Pos(), g.m.Pos())
	if d < g.cfg.PounceMaxDist || d > g.cfg.MaxStalkDist {
		return false
	}
	if g.cfg.SetPersistent && t.IsPlayer() {
		g.m.SetPersistent()
	}
	return true
}

// CanContinueToUse implements goal.Goal (0x5933DF0).
func (g *StalkAndPounce) CanContinueToUse() bool {
	switch g.state {
	case stalkDone:
		return false
	case stalkLeaping, stalkStuck:
		return true
	}
	t := g.actor(g.m.Target())
	if t == nil || dist2(t.Pos(), g.m.Pos()) > g.cfg.MaxStalkDist {
		return false
	}
	if g.state == stalkInterested {
		return true
	}
	n := g.m.Nav()
	return n != nil && !n.Done()
}

// CanBeInterrupted implements goal.Goal (0x5935760): not in the air or
// stuck.
func (g *StalkAndPounce) CanBeInterrupted() bool {
	return g.state != stalkLeaping && g.state != stalkStuck
}

// Start implements goal.Goal (0x5933FC0).
func (g *StalkAndPounce) Start() {
	g.state = stalkStalking
	g.m.SetFlag(FlagStalking, true)
	g.until = 0
	t := g.actor(g.m.Target())
	if t == nil {
		g.state = stalkDone
		return
	}
	g.m.FaceActor(t, stalkLookYaw, stalkLookPitch)
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
}

// Stop implements goal.Goal (0x5934150).
func (g *StalkAndPounce) Stop() {
	g.m.SetFallImmune(false)
	g.m.SetFlag(FlagStunned, false)
	g.m.SetFlag(FlagInterested, false)
	g.m.SetFlag(FlagStalking, false)
}

// Tick implements goal.Goal (0x5934220).
func (g *StalkAndPounce) Tick() {
	p := g.m.Pos()
	if t := g.actor(g.m.Target()); t != nil {
		g.chase(t, p)
	}
	if g.state == stalkLeaping && g.m.OnGround() && g.actor(g.m.Target()) == nil {
		if g.stuckAt(p, nil) {
			g.stick()
		} else {
			g.state = stalkDone
		}
	}
	if g.state == stalkStuck {
		g.struggle(p)
	}
}

// chase is the part of the tick that needs the target.
func (g *StalkAndPounce) chase(t Actor, p Vec3) {
	if g.state != stalkLeaping && g.state != stalkStuck {
		g.m.FaceActor(t, stalkLookYaw, stalkLookPitch)
	}
	q := t.Pos()
	switch g.state {
	case stalkInterested:
		g.prepare(p, q)
	case stalkLeaping:
		if !g.m.OnGround() && !g.m.InWater() {
			return
		}
		g.m.SetFallImmune(false)
		if dist2(p, q) <= g.cfg.StrikeDist {
			if t := g.actor(g.m.Target()); t != nil {
				g.m.Attack(t)
			}
			g.state = stalkDone
			return
		}
		if !g.stuckAt(p, t) {
			g.state = stalkDone
			return
		}
		g.stick()
		g.m.SetTarget(NoID)
	case stalkStalking:
		n := g.m.Nav()
		if n == nil {
			return
		}
		if dist2(q, p) <= g.cfg.PounceMaxDist {
			g.state = stalkInterested
			g.until = g.world.CurrentTick() + uint64(g.cfg.InterestTime)
			g.m.SetFlag(FlagInterested, true)
			g.m.SetFlag(FlagStalking, false)
			n.Stop()
			return
		}
		if n.Done() && !n.MoveTo(q, g.cfg.StalkSpeed) {
			g.state = stalkDone
			g.m.SetTarget(NoID)
		}
	}
}

// stuckAt is _isStuckBlock: the stuck_blocks filter at the mob's block.
func (g *StalkAndPounce) stuckAt(p Vec3, t Actor) bool {
	return g.cfg.StuckBlocks != nil && g.cfg.StuckBlocks(g.m, t, blockOf(p))
}

// stick starts the stuck state.
func (g *StalkAndPounce) stick() {
	g.until = g.world.CurrentTick() + uint64(g.cfg.StuckTime)
	g.state = stalkStuck
	g.m.SetFlag(FlagStunned, true)
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
}

// struggle holds the stuck fox still and sends block particles now and
// then until the stuck time is over.
func (g *StalkAndPounce) struggle(p Vec3) {
	b := g.world.Block(blockOf(p))
	g.m.SetVelocity(Vec3{})
	if g.world.CurrentTick() >= g.until {
		g.m.SetFlag(FlagStunned, false)
		g.state = stalkDone
		return
	}
	if g.world.Random().NextFloat() < stalkParticleOdds {
		g.world.LevelEvent(stalkStuckEvent, p, b.Name)
	}
}

// prepare is _preparePounce: once the interest time is over and the path
// to the target is clear, leap at it.
func (g *StalkAndPounce) prepare(p, q Vec3) {
	n := g.m.Nav()
	if !g.m.OnGround() || dist2(q, p) > g.cfg.PounceMaxDist {
		switch {
		case n == nil:
			g.state = stalkDone
		case n.Done():
			g.state = stalkStalking
			g.m.SetFlag(FlagInterested, false)
			g.m.SetFlag(FlagStalking, true)
		}
		return
	}
	if n == nil {
		g.state = stalkDone
	} else if !n.Done() {
		n.Stop()
	}
	if g.world.CurrentTick() <= g.until {
		return
	}
	if !g.clear(p, q) {
		g.state = stalkDone
		return
	}
	g.state = stalkLeaping
	d := normalize(sub(q, p))
	v := g.m.Velocity()
	g.m.SetVelocity(Vec3{X: d.X*g.cfg.LeapDist + v.X, Y: g.m.JumpMultiplier()*g.cfg.LeapHeight + v.Y, Z: g.cfg.LeapDist*d.Z + v.Z})
	g.m.JumpPrevented()
	g.m.SetFallImmune(true)
	g.m.SetFlag(FlagInterested, false)
}

// clear checks the three blocks above six points along the leap; BDS
// steps along x, or along z when the line has a slope.
func (g *StalkAndPounce) clear(p, q Vec3) bool {
	dx, dz := q.X-p.X, q.Z-p.Z
	if abs32(dx) < steerEpsilon {
		return false
	}
	slope := dz / dx
	for i := 0; i < stalkSamples; i++ {
		f := float32(i) / stalkSamples
		var ox, oz float32
		if slope == 0 {
			ox = f * dx
		} else {
			oz = f * dz
			ox = oz / slope
		}
		for h := float32(1); h <= stalkClearHeight; h++ {
			if !g.world.BuiltOver(blockOf(Vec3{X: ox + p.X, Y: h + p.Y, Z: oz + p.Z})) {
				return false
			}
		}
	}
	return true
}
