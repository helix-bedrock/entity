package behavior

// The find_mount numbers.
const (
	findMountRepathMin  = 4  // fewest ticks between paths to the mount
	findMountRepathSpan = 7  // the random part of the repath wait
	findMountBackoff    = 20 // start delay ticks per failed attempt
	findMountReachSlack = 2  // added to the widths for the default reach
	findMountLook       = 30
	findMountSpeed      = 1
)

// MountSeeker is the part of the mob find_mount needs on top of Mob.
type MountSeeker interface {
	Mob
	// AcceptsRider is the mount's canAddPassenger for the mob (Actor
	// vt+744).
	AcceptsRider(mount ID) bool
	// Sleeping is Actor vt+336 for another entity.
	Sleeping(id ID) bool
	// Wet is another entity's WasInWaterFlagComponent.
	Wet(id ID) bool
	// CanRide is 0xB2FC930: the mob can ride the mount now.
	CanRide(mount ID) bool
	// StartRiding mounts the entity (Actor vt+224).
	StartRiding(mount ID)
	// GiveUpRiding is Actor vt+408 with false, which find_mount calls when
	// the mount is lost.
	GiveUpRiding()
	// PathToActor makes a path to the entity for the goal to follow
	// later; complete is the path's status 2.
	PathToActor(a Actor) (ok, complete bool)
	// FollowPath walks the path PathToActor made, at the speed.
	FollowPath(speed float32)
	// EntitiesOfType appends the entities of the BDS actor type whose boxes
	// touch b to dst, excluding self.
	EntitiesOfType(dst []Actor, actorType int, self ID, b Box) []Actor
}

// FindMountConfig is the JSON of minecraft:behavior.find_mount.
type FindMountConfig struct {
	WithinRadius float32 `json:"within_radius"`
	// StartDelay is in ticks.
	StartDelay   int32 `json:"start_delay"`
	AvoidWater   bool  `json:"avoid_water"`
	TargetNeeded bool  `json:"target_needed"`
	// MountDistance below 0 uses the two widths plus two.
	MountDistance     float32 `json:"mount_distance"`
	MaxFailedAttempts int32   `json:"max_failed_attempts"`
}

// DefaultFindMount returns the goal's BDS defaults (0x5319A20).
func DefaultFindMount() FindMountConfig {
	return FindMountConfig{MountDistance: -1, MaxFailedAttempts: 20}
}

// FindMount is FindMountGoal: the mob looks for the nearest mob that takes
// it as a rider, walks there and gets on.
type FindMount struct {
	base
	m       MountSeeker
	cfg     FindMountConfig
	mount   ID
	delay   int32
	failed  int32
	repath  int32
	hasPath bool
	near    []Actor
}

// NewFindMount makes the goal.
func NewFindMount(w World, m MountSeeker, c FindMountConfig) *FindMount {
	return &FindMount{base: base{mob: m, world: w, flags: flagsMove}, m: m, cfg: c, mount: NoID}
}

// CanUse implements goal.Goal (0x573AB10).
func (g *FindMount) CanUse() bool {
	m := g.m
	if m.Nav() == nil || m.Vehicle() != NoID {
		return false
	}
	if g.cfg.TargetNeeded {
		t := g.actor(m.Target())
		if t == nil || t.Removed() || t.Vehicle() != NoID {
			return false
		}
	}
	if g.delay < g.cfg.StartDelay {
		g.delay++
		return false
	}
	r := g.cfg.WithinRadius
	b := m.Box()
	area := Box{Min: Vec3{X: b.Min.X - r, Y: b.Min.Y - r, Z: b.Min.Z - r}, Max: Vec3{X: b.Max.X + r, Y: b.Max.Y + r, Z: b.Max.Z + r}}
	g.near = m.EntitiesOfType(g.near[:0], actorTypeMob, m.ID(), area)
	best := float32(-1)
	g.mount = NoID
	for _, c := range g.near {
		if c.ID() == m.ID() || !c.Alive() || !m.AcceptsRider(c.ID()) || m.Sleeping(c.ID()) {
			continue
		}
		if g.cfg.AvoidWater && m.Wet(c.ID()) {
			continue
		}
		d := dist2(c.Pos(), m.Pos())
		if best >= 0 && best <= d {
			continue
		}
		best, g.mount = d, c.ID()
	}
	if g.mount == NoID {
		g.delay = 0
		return false
	}
	return true
}

// CanContinueToUse implements goal.Goal (0x573AEE0).
func (g *FindMount) CanContinueToUse() bool {
	m := g.m
	t := g.actor(g.mount)
	if t != nil && t.Alive() && m.AcceptsRider(t.ID()) && m.Vehicle() == NoID {
		return true
	}
	m.GiveUpRiding()
	return false
}

// reach2 is the squared distance at which the mob gets on.
func (g *FindMount) reach2(t Actor) float32 {
	if g.cfg.MountDistance >= 0 {
		return g.cfg.MountDistance
	}
	w := boxWidth(g.m.Box())
	d := w + w + boxWidth(t.Box()) + findMountReachSlack
	return d * d
}

func boxWidth(b Box) float32 { return b.Max.X - b.Min.X }

// Start implements goal.Goal (0x573AF90): a far mount needs a complete
// path, or the goal ends at once and backs off.
func (g *FindMount) Start() {
	t := g.actor(g.mount)
	g.hasPath = false
	var complete bool
	if t != nil {
		g.hasPath, complete = g.m.PathToActor(t)
	}
	if t == nil || g.reach2(t) <= dist2(g.m.Pos(), t.Pos()) {
		if !g.hasPath || !complete {
			g.Stop()
			g.delay = -findMountBackoff * g.failed
		}
	}
}

// Stop implements goal.Goal (0x573B2B0).
func (g *FindMount) Stop() {
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
	if g.failed < g.cfg.MaxFailedAttempts {
		g.failed++
	}
	g.hasPath = false
	g.mount = NoID
}

// Tick implements goal.Goal (0x573B430).
func (g *FindMount) Tick() {
	m := g.m
	t := g.actor(g.mount)
	if t == nil || !m.CanRide(t.ID()) {
		g.Stop()
		g.delay = -findMountBackoff * g.failed
		return
	}
	if t.ID() != m.ID() && g.reach2(t) > dist2(m.Pos(), t.Pos()) && t.Alive() {
		m.StartRiding(t.ID())
		g.Stop()
		g.failed = 0
		return
	}
	g.repath--
	if g.repath > 0 {
		return
	}
	g.repath = g.world.IRandom().NextInt(findMountRepathSpan) + findMountRepathMin
	if g.hasPath {
		m.LookAt(lookPoint(t), findMountLook, findMountLook)
		m.FollowPath(findMountSpeed)
		g.hasPath = false
	}
}
