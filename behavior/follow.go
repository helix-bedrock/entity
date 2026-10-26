package behavior

import "github.com/helix-bedrock/entity/nav"

// The follow goals' fixed numbers.
const (
	followRepath      = 10
	followLookYaw     = 10
	followTeleport2   = 144 // squared distance at which a lost pet teleports
	vibrationTicks    = 600 // a heard vibration stops follow_owner this long
	teleportRing      = 3   // the teleport spots are 3 to 4 blocks out
	teleportTries     = 9
	teleportJitter    = 0.1
	teleportJitterOff = -0.05
	parentBoxXZ       = 8
	parentBoxY        = 4
	parentMin2        = 9
	parentMax2        = 256
)

// FollowOwnerConfig is the JSON of minecraft:behavior.follow_owner.
type FollowOwnerConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	StartDistance   float32 `json:"start_distance"`
	StopDistance    float32 `json:"stop_distance"`
	MaxDistance     float32 `json:"max_distance"`
	CanTeleport     bool    `json:"can_teleport"`
	IgnoreVibration bool    `json:"ignore_vibration"`
	// RandomTeleport shuffles the teleport spots and checks them for
	// blocking entities; RepathAfterTeleport paths on after a teleport.
	// BDS ties both to game toggles that are on in 1.26.52.
	RandomTeleport, RepathAfterTeleport bool
}

// DefaultFollowOwner is the goal's BDS defaults.
var DefaultFollowOwner = FollowOwnerConfig{
	SpeedMultiplier: 1, StartDistance: 10, StopDistance: 2, MaxDistance: 60,
	CanTeleport: true, IgnoreVibration: true, RandomTeleport: true, RepathAfterTeleport: true,
}

// FollowOwner is FollowOwnerGoal: a pet walks after its owner and
// teleports to it when it falls far behind.
type FollowOwner struct {
	base
	cfg    FollowOwnerConfig
	owner  ID
	repath int32
	saved  uint32
	spots  spotFinder
}

// NewFollowOwner makes the goal.
func NewFollowOwner(w World, m Mob, c FollowOwnerConfig) *FollowOwner {
	return &FollowOwner{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c, owner: NoID}
}

// CanUse implements goal.Goal.
func (g *FollowOwner) CanUse() bool {
	n := g.mob.Nav()
	if n == nil {
		return false
	}
	o := g.actor(g.mob.Owner())
	if o == nil || spectator(g.world, o) || g.mob.Flag(FlagSitting) {
		return false
	}
	if o.Dimension() != g.mob.Dimension() {
		if g.mob.CanSit() && !g.mob.Flag(FlagLeashed) {
			g.mob.SetFlag(FlagSitting, true)
		}
		return false
	}
	if g.cfg.StartDistance*g.cfg.StartDistance > dist2(g.mob.Pos(), o.Pos()) {
		return false
	}
	if !g.reachable(o) {
		return false
	}
	if t, ok := g.mob.HeardVibration(); ok && !g.cfg.IgnoreVibration && g.world.CurrentTick()-t < vibrationTicks {
		return false
	}
	g.owner = o.ID()
	return true
}

// reachable is true for teleporting pets, otherwise the owner must be in
// range and have a path.
func (g *FollowOwner) reachable(o Actor) bool {
	if g.cfg.CanTeleport {
		return true
	}
	if dist2(g.mob.Pos(), o.Pos()) > g.cfg.MaxDistance*g.cfg.MaxDistance {
		return false
	}
	_, ok := g.mob.Nav().PathTo(o)
	return ok
}

// CanContinueToUse implements goal.Goal.
func (g *FollowOwner) CanContinueToUse() bool {
	if g.mob.Flag(FlagSitting) {
		return false
	}
	o := g.actor(g.owner)
	if o == nil || o.Removed() || spectator(g.world, o) {
		return false
	}
	if g.cfg.StopDistance*g.cfg.StopDistance >= dist2(g.mob.Pos(), o.Pos()) {
		return false
	}
	n := g.mob.Nav()
	return n != nil && !n.Done() && g.reachable(o)
}

// Start implements goal.Goal.
func (g *FollowOwner) Start() {
	g.repath = 0
	if n := g.mob.Nav(); n != nil {
		f := n.Flags()
		g.saved = f & uint32(nav.FlagAvoidWater|nav.FlagAvoidPortals)
		n.SetFlags(f&^uint32(nav.FlagAvoidWater) | uint32(nav.FlagAvoidPortals))
	}
}

// Stop implements goal.Goal.
func (g *FollowOwner) Stop() {
	g.owner = NoID
	if n := g.mob.Nav(); n != nil {
		n.Stop()
		n.SetFlags(n.Flags()&^uint32(nav.FlagAvoidWater|nav.FlagAvoidPortals) | g.saved)
	}
}

// Tick implements goal.Goal.
func (g *FollowOwner) Tick() {
	o := g.actor(g.owner)
	if o == nil {
		return
	}
	if !g.mob.LookBusy() && !o.Removed() {
		g.mob.LookAt(lookPoint(o), followLookYaw, g.mob.MaxHeadXRot())
	}
	if g.mob.Flag(FlagSitting) || g.mob.Vehicle() != NoID {
		return
	}
	g.repath--
	if g.repath+1 > 0 {
		return
	}
	g.repath = followRepath
	n := g.mob.Nav()
	if n != nil && n.MoveToActor(o, g.cfg.SpeedMultiplier) {
		return
	}
	if g.cfg.CanTeleport && !g.mob.Flag(FlagLeashed) && dist2(g.mob.Pos(), o.Pos()) >= followTeleport2 {
		g.teleport(o)
	}
}

// teleport moves the pet to a free spot 3 to 4 blocks from the owner.
func (g *FollowOwner) teleport(o Actor) {
	p, ok := g.spots.find(g.world, g.mob, o, teleportRing, g.cfg.RandomTeleport)
	if !ok {
		return
	}
	g.mob.Teleport(p)
	n := g.mob.Nav()
	if n != nil {
		n.Stop()
	}
	if g.cfg.RepathAfterTeleport && n != nil && dist2(g.mob.Pos(), o.Pos()) > g.cfg.StopDistance*g.cfg.StopDistance {
		n.MoveToActor(o, g.cfg.SpeedMultiplier)
	}
}

// spotFinder is BDS's teleport spot search (0xBDD0B10), with the buffers it
// reuses.
type spotFinder struct {
	spots []Vec3
	buf   []Actor
}

// find looks for a free spot ring to ring+1 blocks out from o, on the
// owner's feet level. With random set the spots are shuffled and checked
// for entities that block the mob's box.
func (g *spotFinder) find(w World, m Mob, o Actor, ring int32, random bool) (Vec3, bool) {
	op, ob := blockOf(o.Pos()), o.Box()
	feet := floorInt(ob.Min.Y)
	r, R := ring, ring+1
	g.spots = g.spots[:0]
	for x := -R; x <= R; x++ {
		outX := x < -r || x > r
		for z := -R; z <= R; z++ {
			if !outX && z >= -r && z <= r {
				continue
			}
			c := Pos{X: op.X + x, Y: feet, Z: op.Z + z}
			below := w.Block(Pos{X: c.X, Y: c.Y - 1, Z: c.Z})
			at, head := w.Block(c), w.Block(Pos{X: c.X, Y: c.Y + 1, Z: c.Z})
			if !below.Solid || at.Solid || at.Liquid || head.Solid || head.Liquid {
				continue
			}
			spot := Vec3{X: float32(c.X) + 0.5, Y: float32(c.Y), Z: float32(c.Z) + 0.5}
			if !random {
				return spot, true
			}
			g.spots = append(g.spots, spot)
		}
	}
	if len(g.spots) == 0 {
		return Vec3{}, false
	}
	rnd := w.Random()
	for i := 1; i < len(g.spots); i++ {
		if j := int(rnd.NextInt(int32(i + 1))); j != i {
			g.spots[i], g.spots[j] = g.spots[j], g.spots[i]
		}
	}
	mb := m.Box()
	half := Vec3{X: (mb.Max.X - mb.Min.X) * 0.5, Y: (mb.Max.Y - mb.Min.Y) * 0.5, Z: (mb.Max.Z - mb.Min.Z) * 0.5}
	for i := 0; i < min(len(g.spots), teleportTries); i++ {
		s := g.spots[i]
		b := Box{Min: Vec3{X: s.X - half.X, Y: s.Y - half.Y, Z: s.Z - half.Z}, Max: Vec3{X: s.X + half.X, Y: s.Y + half.Y, Z: s.Z + half.Z}}
		if g.blocked(w, b) {
			continue
		}
		jx := rnd.NextFloat()*teleportJitter + teleportJitterOff
		jz := rnd.NextFloat()*teleportJitter + teleportJitterOff
		return Vec3{X: s.X + jx, Y: s.Y, Z: s.Z + jz}, true
	}
	return Vec3{}, false
}

func (g *spotFinder) blocked(w World, b Box) bool {
	g.buf = w.Entities(g.buf[:0], NoID, b)
	defer clear(g.buf)
	for _, e := range g.buf {
		if !e.Removed() && e.BlocksSpace() {
			return true
		}
	}
	return false
}

// FollowParent is FollowParentGoal: a baby walks after the nearest adult of
// its type.
type FollowParent struct {
	base
	speed float32
	// stopOnEnd is the game toggle that stops navigation when the goal
	// ends; on in 1.26.52.
	stopOnEnd bool
	parent    ID
	repath    int32
	buf       []Actor
}

// NewFollowParent makes minecraft:behavior.follow_parent; speed is its
// speed_multiplier (1 by default).
func NewFollowParent(w World, m Mob, speed float32) *FollowParent {
	return &FollowParent{base: base{mob: m, world: w, flags: flagsMove}, speed: speed, stopOnEnd: true, parent: NoID}
}

// CanUse implements goal.Goal.
func (g *FollowParent) CanUse() bool {
	if g.mob.Nav() == nil || !g.mob.Flag(FlagBaby) {
		return false
	}
	b := g.mob.Box()
	b.Min = Vec3{X: b.Min.X - parentBoxXZ, Y: b.Min.Y - parentBoxY, Z: b.Min.Z - parentBoxXZ}
	b.Max = Vec3{X: b.Max.X + parentBoxXZ, Y: b.Max.Y + parentBoxY, Z: b.Max.Z + parentBoxXZ}
	g.buf = g.world.Entities(g.buf[:0], g.mob.ID(), b)
	defer clear(g.buf)
	best := float32(temptFloatMaxValue)
	var found Actor
	for _, e := range g.buf {
		if !e.IsMob() || e.Type() != g.mob.Type() || e.Flag(FlagBaby) || !e.Alive() {
			continue
		}
		d := dist2(g.mob.Pos(), e.Pos())
		if d <= best {
			found = e
		}
		best = min(best, d)
	}
	if found == nil || best < parentMin2 {
		return false
	}
	g.parent = found.ID()
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *FollowParent) CanContinueToUse() bool {
	p := g.actor(g.parent)
	if p == nil || !p.Alive() {
		return false
	}
	d := dist2(g.mob.Pos(), p.Pos())
	return d >= parentMin2 && d <= parentMax2
}

// Start implements goal.Goal.
func (g *FollowParent) Start() { g.repath = 0 }

// Stop implements goal.Goal.
func (g *FollowParent) Stop() {
	g.parent = NoID
	if n := g.mob.Nav(); g.stopOnEnd && n != nil {
		n.Stop()
	}
}

// Tick implements goal.Goal.
func (g *FollowParent) Tick() {
	g.repath--
	if g.repath+1 > 1 {
		return
	}
	g.repath = followRepath
	p := g.actor(g.parent)
	if n := g.mob.Nav(); p != nil && !p.Removed() && n != nil {
		n.MoveToActor(p, g.speed)
	}
}
