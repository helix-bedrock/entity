package nav

import "github.com/helix-bedrock/entity/path"

// Kind is a navigation type: which minecraft:navigation.* component the mob
// has.
type Kind uint8

// The navigation types. BDS's classes are in brackets.
const (
	KindWalk    Kind = iota // navigation.walk (PathNavigation)
	KindFloat               // navigation.float (FloatNavigation): never paths
	KindGeneric             // navigation.generic (GenericPathNavigation)
	KindClimb               // navigation.climb (WallClimberPathNavigation)
	KindSwim                // navigation.swim (WaterBoundPathNavigation)
	KindFly                 // navigation.fly (FlyingPathNavigation)
	KindHover               // navigation.hover (HoverPathNavigation)
)

// kindNames maps the JSON component names to kinds.
var kindNames = map[string]Kind{
	"minecraft:navigation.walk":    KindWalk,
	"minecraft:navigation.float":   KindFloat,
	"minecraft:navigation.generic": KindGeneric,
	"minecraft:navigation.climb":   KindClimb,
	"minecraft:navigation.swim":    KindSwim,
	"minecraft:navigation.fly":     KindFly,
	"minecraft:navigation.hover":   KindHover,
}

// KindOf returns the kind for a JSON component name.
func KindOf(component string) (Kind, bool) {
	k, ok := kindNames[component]
	return k, ok
}

// flyReach is the reach fly navigation's constructor sets (component +12).
const flyReach = 1.0

// NewKind returns a navigation of kind k for the JSON description d.
func NewKind(k Kind, d *Description) *Navigation {
	n := &Navigation{
		Flags:          d.Flags(),
		Kind:           k,
		StuckInterval:  DefaultStuckInterval,
		StuckDistance:  DefaultStuckDistance,
		canPathFromAir: d.CanPathFromAir,
		gravity:        true,
		canWalk:        d.CanWalk,
		canSwim:        d.CanSwim,
	}
	if k == KindSwim {
		n.Flags = n.Flags&^(FlagAvoidDamageBlocks|FlagAvoidWater) | FlagCanFloat
	}
	return n
}

// Init sets the movement flags the navigation type gives the mob, as BDS
// does when the component is added. Fly navigation also remembers the
// mob's gravity.
func (n *Navigation) Init(m *Mob) {
	swim, walk, fly := false, true, false
	switch n.Kind {
	case KindGeneric:
		swim, walk = n.canSwim, n.canWalk
	case KindSwim:
		swim, walk = true, false
	case KindFly, KindHover:
		walk, fly = false, true
	}
	setFlag(m, ActorCanSwim, swim)
	setFlag(m, ActorCanWalk, walk)
	if fly {
		setFlag(m, ActorCanFly, true)
	}
	if n.Kind == KindFly {
		n.gravity = m.Actor == nil || m.Actor.Flag(ActorHasGravity)
	}
}

func setFlag(m *Mob, f int, on bool) {
	if m.Actor != nil {
		m.Actor.SetFlag(f, on)
	}
}

// saveGravity remembers the mob's gravity before fly navigation takes a new
// path, unless it is following one (and has turned gravity off).
func (n *Navigation) saveGravity(m *Mob) {
	if n.Done() {
		n.gravity = m.Actor == nil || m.Actor.Flag(ActorHasGravity)
	}
}

func (n *Navigation) restoreGravity(m *Mob) {
	if m.Actor != nil && m.Actor.Flag(ActorHasGravity) != n.gravity {
		m.Actor.SetFlag(ActorHasGravity, n.gravity)
	}
}

// canUpdate is the navigation type's test for moving on along the path.
func (n *Navigation) canUpdate(m *Mob) bool {
	switch n.Kind {
	case KindClimb:
		return m.HorizontalCollision || CanUpdatePath(m)
	case KindSwim:
		return m.WasInWater || m.InLava
	case KindFly, KindHover:
		if m.Riding {
			return n.Flags&FlagCanFloat != 0 && m.WasInWater
		}
		return m.WasInWater || m.InLava || m.OnGround || n.canPathFromAir
	}
	return CanUpdatePath(m)
}

// mobPos is where the navigation type measures the mob from.
func (n *Navigation) mobPos(w World, m *Mob) path.Vec3 {
	switch n.Kind {
	case KindFly:
		return m.Pos
	case KindHover:
		return boxCentre(m.Box)
	case KindSwim:
		return path.Vec3{X: m.Pos.X, Y: m.Pos.Y + m.Height*0.5, Z: m.Pos.Z}
	case KindGeneric:
		if m.WasInWater {
			return m.Pos
		}
	}
	return n.walkMobPos(w, m)
}

func boxCentre(b path.Box) path.Vec3 {
	return path.Vec3{
		X: (b.Max.X-b.Min.X)*0.5 + b.Min.X,
		Y: (b.Max.Y-b.Min.Y)*0.5 + b.Min.Y,
		Z: (b.Max.Z-b.Min.Z)*0.5 + b.Min.Z,
	}
}

// dist2 is the squared distance from a to b.
func dist2(a, b path.Vec3) float32 {
	dx, dy, dz := b.X-a.X, b.Y-a.Y, b.Z-a.Z
	return dz*dz + (dx*dx + dy*dy)
}

// stuck runs the stuck check: every more than interval ticks, a mob that
// moved less than dist blocks stops.
func (n *Navigation) stuck(w World, m *Mob, interval int32, d float32, skip bool) {
	if n.ticks-n.lastCheck <= interval {
		return
	}
	if dist2(n.lastPos, n.mobPos(w, m)) < d*d && !skip {
		n.Stop(m)
	}
	n.lastCheck = n.ticks
	n.lastPos = n.mobPos(w, m)
}
