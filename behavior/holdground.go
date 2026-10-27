package behavior

// The hold_ground goal's fixed numbers.
const (
	holdLookYaw      = 10 // the look control's yaw speed
	holdAmbientOneIn = 50 // the chance per tick of an ambient sound
)

// Holder is the part of the mob hold_ground needs on top of Mob.
type Holder interface {
	Mob
	// HasLookControl reports LookControlComponent.
	HasLookControl() bool
	// AmbientSound plays the mob's ambient sound (Mob::playAmbientSound).
	AmbientSound()
}

// HoldGroundConfig is the JSON of minecraft:behavior.hold_ground.
type HoldGroundConfig struct {
	MinRadius      float32 `json:"min_radius"`
	Broadcast      bool    `json:"broadcast"`
	BroadcastRange float32 `json:"broadcast_range"`
	// WithinRadiusEvent is within_radius_event: fired on the mob and its
	// kind nearby when the target comes within min_radius.
	WithinRadiusEvent string
}

// DefaultHoldGround returns the goal's BDS defaults.
func DefaultHoldGround() HoldGroundConfig {
	return HoldGroundConfig{MinRadius: 10}
}

// HoldGround is HoldGroundGoal: the mob stands still and watches a target
// that is far away, and stops when the target comes close.
type HoldGround struct {
	base
	h      Holder
	cfg    HoldGroundConfig
	min2   float32
	target ID
	near   []Actor
}

// NewHoldGround makes the goal.
func NewHoldGround(w World, h Holder, c HoldGroundConfig) *HoldGround {
	return &HoldGround{base: base{mob: h, world: w, flags: flagsMoveLook}, h: h, cfg: c,
		min2: c.MinRadius * c.MinRadius, target: NoID}
}

// fighting reports that a is the mob's last attacker or the mob it last
// hurt; both IDs are cleared when their entity is gone.
func (g *HoldGround) fighting(a Actor) bool {
	if by := g.actor(g.h.LastHurtByMob()); by != nil && by.ID() == a.ID() {
		return true
	}
	id, _ := g.h.LastHurtMob()
	hurt := g.actor(id)
	return hurt != nil && hurt.ID() == a.ID()
}

// CanUse implements goal.Goal (0x574A270).
func (g *HoldGround) CanUse() bool {
	if t := g.actor(g.h.Target()); t != nil && !g.fighting(t) &&
		dist2(g.h.Pos(), t.Pos()) > g.min2 && t.ID() != g.target {
		g.target = t.ID()
	}
	t := g.actor(g.target)
	return t != nil && !t.Removed()
}

// CanContinueToUse implements goal.Goal (0x574A500).
func (g *HoldGround) CanContinueToUse() bool {
	t := g.actor(g.target)
	if t == nil || t.Removed() {
		return false
	}
	if m := t.GameMode(); m == GameModeCreative || m == GameModeDefault && g.world.DefaultGameMode() == GameModeCreative {
		return false
	}
	return !g.fighting(t) && g.min2 <= dist2(g.h.Pos(), t.Pos())
}

// kin appends the entities of the mob's type in the mob's box grown by
// broadcast_range.
func (g *HoldGround) kin(self ID) []Actor {
	b, r := g.h.Box(), g.cfg.BroadcastRange
	grown := Box{
		Min: Vec3{X: min(b.Min.X-r, b.Max.X+r), Y: min(b.Min.Y-r, b.Max.Y+r), Z: min(b.Min.Z-r, b.Max.Z+r)},
		Max: Vec3{X: max(b.Min.X-r, b.Max.X+r), Y: max(b.Min.Y-r, b.Max.Y+r), Z: max(b.Min.Z-r, b.Max.Z+r)},
	}
	g.near = g.world.Entities(g.near[:0], self, grown)
	n := 0
	for _, a := range g.near {
		if a.Type() == g.h.Type() {
			g.near[n] = a
			n++
		}
	}
	clear(g.near[n:])
	return g.near[:n]
}

// Start implements goal.Goal (0x574A680).
func (g *HoldGround) Start() {
	if n := g.h.Nav(); n != nil {
		n.Stop()
	}
	if !g.cfg.Broadcast {
		return
	}
	for _, a := range g.kin(g.h.ID()) {
		if m, ok := a.(Mob); ok {
			m.SetTarget(g.h.Target())
		}
	}
	clear(g.near)
}

// Stop implements goal.Goal (0x574AA10): with broadcast on and the target
// inside min_radius, the mob and its kind nearby fire within_radius_event.
func (g *HoldGround) Stop() {
	t := g.actor(g.target)
	g.target = NoID
	if !g.cfg.Broadcast || t == nil || t.Removed() || g.min2 <= dist2(g.h.Pos(), t.Pos()) {
		return
	}
	if g.cfg.WithinRadiusEvent == "" {
		return
	}
	// BDS's query includes the mob itself.
	for _, a := range g.kin(NoID) {
		if m, ok := a.(Mob); ok {
			m.Event(g.cfg.WithinRadiusEvent, NoID)
		}
	}
	clear(g.near)
}

// Tick implements goal.Goal (0x574ADF0).
func (g *HoldGround) Tick() {
	t := g.actor(g.target)
	if t == nil || !g.h.HasLookControl() {
		return
	}
	g.h.LookAt(t.Eyes(), holdLookYaw, g.h.MaxHeadXRot())
	if g.world.IRandom().NextInt(holdAmbientOneIn) == 0 {
		g.h.AmbientSound()
	}
}
