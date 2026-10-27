package behavior

// The silverfish goals' fixed numbers.
const (
	mergeChance     = 10 // one in this many stroll starts tries to merge
	mergeFacings    = 6
	actorEventMerge = 27 // the silverfish disappearing into a block
	wakeDelay       = 20 // ticks from the hit to the friends waking up
	wakeRangeXZ     = 10
	wakeRangeY      = 5
)

// facingOffsets are BDS's six facings: down, up, north, south, west, east.
var facingOffsets = [mergeFacings]Pos{{Y: -1}, {Y: 1}, {Z: -1}, {Z: 1}, {X: -1}, {X: 1}}

// silverfishStroll is the merge goal's fixed stroll; the JSON has no
// fields of its own.
var silverfishStroll = StrollConfig{SpeedMultiplier: 1, XZDist: 10, YDist: 7, Interval: 120}

// Silverfish is the part of the mob the silverfish goals need on top of
// Mob.
type Silverfish interface {
	Mob
	// ActorEvent broadcasts an actor event of the mob.
	ActorEvent(id, data int32)
	// Remove removes the mob from the level.
	Remove()
	// LastHurtByPlayer is the player that last hurt the mob, NoID for
	// none.
	LastHurtByPlayer() ID
}

// SilverfishMerge is SilverfishMergeWithStoneGoal: a silverfish with
// nothing to do now and then hides in a stone block next to it, and
// strolls otherwise.
type SilverfishMerge struct {
	Stroll
	m       Silverfish
	facing  int32
	merging bool
}

// NewSilverfishMerge makes the goal.
func NewSilverfishMerge(w World, m Silverfish) *SilverfishMerge {
	g := &SilverfishMerge{m: m}
	g.init(w, m, silverfishStroll, g)
	return g
}

// spot is the block next to the silverfish on the chosen facing.
func (g *SilverfishMerge) spot() Pos {
	p := g.m.Pos()
	f := facingOffsets[g.facing]
	return Pos{X: floorInt(p.X) + f.X, Y: floorInt(p.Y+0.5) + f.Y, Z: floorInt(p.Z) + f.Z}
}

// CanUse implements goal.Goal (0x5929D50).
func (g *SilverfishMerge) CanUse() bool {
	m, w := g.m, g.world
	if g.actor(m.Target()) != nil {
		return false
	}
	if n := m.Nav(); n == nil || !n.Done() {
		return false
	}
	if w.MobGriefing() && w.Random().NextInt(mergeChance) == 0 {
		g.facing = w.Random().NextInt(mergeFacings)
		if w.Block(g.spot()).Infested != "" {
			g.merging = true
			return true
		}
	}
	g.merging = false
	return g.Stroll.CanUse()
}

// CanContinueToUse implements goal.Goal.
func (g *SilverfishMerge) CanContinueToUse() bool { return !g.merging && !g.done }

// Start implements goal.Goal (0x592A000).
func (g *SilverfishMerge) Start() {
	if !g.merging {
		g.Stroll.Start()
		return
	}
	p := g.spot()
	b := g.world.Block(p)
	if b.Infested == "" {
		return
	}
	if !g.world.GriefAllowed(p, g.m.ID()) {
		g.Stroll.Start()
		return
	}
	g.world.SetBlock(p, PlacedBlock{Name: b.Infested}, g.m.ID())
	g.m.ActorEvent(actorEventMerge, 0)
	g.m.Remove()
}

// SilverfishWake is SilverfishWakeUpFriendsGoal: a silverfish that was hit
// breaks the infested blocks around it a second later, letting out the
// silverfish inside.
type SilverfishWake struct {
	base
	m         Silverfish
	countdown int32
}

// NewSilverfishWake makes the goal.
func NewSilverfishWake(w World, m Silverfish) *SilverfishWake {
	return &SilverfishWake{base: base{mob: m, world: w}, m: m}
}

// CanUse implements goal.Goal (0x592A460).
func (g *SilverfishWake) CanUse() bool {
	if g.m.Nav() == nil {
		return false
	}
	if g.actor(g.m.LastHurtByMob()) != nil || g.actor(g.m.LastHurtByPlayer()) != nil {
		if g.countdown <= 0 {
			g.countdown = wakeDelay
		}
		return true
	}
	return g.countdown > 0
}

// CanContinueToUse implements goal.Goal: BDS's default, canUse again.
func (g *SilverfishWake) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *SilverfishWake) Start() {}

// Stop implements goal.Goal.
func (g *SilverfishWake) Stop() {}

// Tick implements goal.Goal (0x592A5D0): when the countdown runs out, every
// infested block in range gives up its silverfish, in BDS's order, until a
// coin flip after one of them says stop.
func (g *SilverfishWake) Tick() {
	left := g.countdown
	g.countdown--
	if left > 1 {
		return
	}
	m, w := g.m, g.world
	c := blockOf(m.Pos())
	for y := int32(0); y >= -wakeRangeY && y <= wakeRangeY; y = ringNext(y) {
		for x := int32(0); x >= -wakeRangeXZ && x <= wakeRangeXZ; x = ringNext(x) {
			for z := int32(0); z >= -wakeRangeXZ && z <= wakeRangeXZ; z = ringNext(z) {
				p := Pos{X: c.X + x, Y: c.Y + y, Z: c.Z + z}
				host := w.Block(p).Host
				if host == "" {
					continue
				}
				if w.MobGriefing() {
					w.BreakBlock(p, m.ID())
				} else {
					w.SetBlock(p, PlacedBlock{Name: host}, m.ID())
				}
				if w.Random().NextBool() {
					return
				}
			}
		}
	}
}
