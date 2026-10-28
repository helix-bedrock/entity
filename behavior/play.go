package behavior

import "math"

// Playmate is a mob that runs minecraft:behavior.play. While it chases a
// friend it carries IsChasingDuringPlayFlagComponent, and friends that
// carry it are not picked.
type Playmate interface {
	Mob
	// ChasingDuringPlay reports the chase flag component on the entity.
	ChasingDuringPlay(id ID) bool
	// SetChasingDuringPlay adds or removes the flag component on the mob.
	SetChasingDuringPlay(on bool)
}

// PlayConfig is the JSON of minecraft:behavior.play.
type PlayConfig struct {
	SpeedMultiplier float32
	// MaxPlayDuration is max_play_duration_seconds in ticks (BDS truncates
	// seconds times 20).
	MaxPlayDuration int32
	// ChanceToStart is the per-tick chance to start.
	ChanceToStart float32
	// FollowDistance is how close the mob gets before it stops chasing
	// its friend; BDS keeps it as an integer.
	FollowDistance int32
	// FriendTypes are the filters a friend must pass; with none, no
	// friend is ever picked.
	FriendTypes []Filter
	// FriendSearchArea is how far around the mob's box friends are looked
	// for.
	FriendSearchArea Vec3
	// RandomPosSearchRange and RandomPosSearchHeight size the random
	// spot search used when there is no friend to chase.
	RandomPosSearchRange, RandomPosSearchHeight int32
}

// DefaultPlay returns the goal's BDS defaults (definition default
// 0xE905EE0, goal setup 0x5A2B090).
func DefaultPlay() PlayConfig {
	return PlayConfig{
		SpeedMultiplier: 1, MaxPlayDuration: 50 * ticksPerSecond, ChanceToStart: 0.0025, FollowDistance: 2,
		FriendSearchArea: Vec3{X: 6, Y: 3, Z: 6}, RandomPosSearchRange: 16, RandomPosSearchHeight: 3,
	}
}

// Play is PlayGoal (canUse 0x550B720, start 0x550BE00, stop 0x550BEF0,
// tick 0x550BFC0): the mob picks the nearest mob of its own type passing
// friend_types and chases it, or runs to random spots when there is none,
// for max_play_duration_seconds.
type Play struct {
	base
	m      Playmate
	cfg    PlayConfig
	friend ID
	timer  int32
	buf    []Actor
}

// NewPlay makes the goal.
func NewPlay(w World, m Playmate, c PlayConfig) *Play {
	return &Play{base: base{mob: m, world: w, flags: flagsMove}, m: m, cfg: c, friend: NoID}
}

// Config returns the goal's settings.
func (g *Play) Config() PlayConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *Play) SetConfig(c PlayConfig) { g.cfg = c }

// CanUse implements goal.Goal (0x550B720).
func (g *Play) CanUse() bool {
	if g.m.Nav() == nil || g.world.Random().NextFloat() > g.cfg.ChanceToStart {
		return false
	}
	if f := g.findFriend(); f != nil {
		g.friend = f.ID()
		return true
	}
	// The spot is only a test here; tick draws its own.
	if _, ok := landPos(g.world, g.m, g.cfg.RandomPosSearchRange, g.cfg.RandomPosSearchHeight); !ok {
		return false
	}
	g.friend = NoID
	return true
}

// findFriend is the nearest mob of the mob's type in the search box that
// is not chasing and passes one of friend_types; on equal distances the
// later one wins.
func (g *Play) findFriend() Actor {
	b, a := g.m.Box(), g.cfg.FriendSearchArea
	lo := Vec3{X: b.Min.X - a.X, Y: b.Min.Y - a.Y, Z: b.Min.Z - a.Z}
	hi := Vec3{X: b.Max.X + a.X, Y: b.Max.Y + a.Y, Z: b.Max.Z + a.Z}
	g.buf = g.world.Entities(g.buf[:0], g.m.ID(), Box{
		Min: Vec3{X: min(lo.X, hi.X), Y: min(lo.Y, hi.Y), Z: min(lo.Z, hi.Z)},
		Max: Vec3{X: max(lo.X, hi.X), Y: max(lo.Y, hi.Y), Z: max(lo.Z, hi.Z)},
	})
	defer clear(g.buf)
	var best Actor
	bestD := float32(math.MaxFloat32)
	for _, c := range g.buf {
		if c.ID() == g.m.ID() || c.Type() != g.m.Type() || g.m.ChasingDuringPlay(c.ID()) || !g.friendType(c) {
			continue
		}
		d := dist2(c.Pos(), g.m.Pos())
		if d <= bestD {
			best = c
		}
		bestD = min(bestD, d)
	}
	return best
}

func (g *Play) friendType(c Actor) bool {
	for _, f := range g.cfg.FriendTypes {
		if f == nil || f(g.m, c) {
			return true
		}
	}
	return false
}

// CanContinueToUse implements goal.Goal (0x550BDF0).
func (g *Play) CanContinueToUse() bool { return g.timer > 0 }

// Start implements goal.Goal (0x550BE00).
func (g *Play) Start() {
	if g.actor(g.friend) != nil {
		g.m.SetChasingDuringPlay(true)
	}
	g.timer = g.cfg.MaxPlayDuration
}

// Stop implements goal.Goal (0x550BEF0).
func (g *Play) Stop() {
	g.m.SetChasingDuringPlay(false)
	g.friend = NoID
}

// Tick implements goal.Goal (0x550BFC0): chase the friend while it is
// farther than follow_distance; with no friend, a new random spot each
// time the path ends.
func (g *Play) Tick() {
	g.timer--
	n := g.m.Nav()
	if n == nil {
		return
	}
	if f := g.actor(g.friend); f != nil {
		if d := float32(g.cfg.FollowDistance * g.cfg.FollowDistance); dist2(g.m.Pos(), f.Pos()) > d {
			n.MoveToActor(f, g.cfg.SpeedMultiplier)
		}
		return
	}
	if !n.Done() {
		return
	}
	if p, ok := landPos(g.world, g.m, g.cfg.RandomPosSearchRange, g.cfg.RandomPosSearchHeight); ok {
		n.MoveTo(p, g.cfg.SpeedMultiplier)
	}
}
