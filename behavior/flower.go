package behavior

// FlowerOfferer is the part of the mob offer_flower and take_flower need
// on top of Mob: the OfferFlowerTickComponent, which counts the ticks a
// mob has held a flower out.
type FlowerOfferer interface {
	Mob
	// HasGoal reports the entity has the named goal (Actor vt+56).
	HasGoal(id ID, name string) bool
	// OfferFlower adds the mob's OfferFlowerTickComponent at 0 (0x721C320).
	OfferFlower()
	// OfferFlowerTicks is the entity's OfferFlowerTickComponent; ok is
	// false without one.
	OfferFlowerTicks(id ID) (ticks int32, ok bool)
	// StopOfferingFlower removes the entity's OfferFlowerTickComponent and,
	// on the server, broadcasts the actor event to its viewers.
	StopOfferingFlower(id ID, event int32)
	// OfferDuration is the entity's offer_flower goal's
	// max_offer_flower_duration in ticks (its +40); ok is false without the
	// goal.
	OfferDuration(id ID) (ticks int32, ok bool)
}

// OfferFlowerConfig is the JSON of minecraft:behavior.offer_flower.
type OfferFlowerConfig struct {
	// MaxOfferFlowerDuration is in seconds.
	MaxOfferFlowerDuration float32 `json:"max_offer_flower_duration"`
	ChanceToStart          float32 `json:"chance_to_start"`
	Filters                Filter  `json:"-"`
	SearchArea             Vec3    `json:"search_area"`
	MaxRotationX           float32 `json:"max_rotation_x"`
	MaxHeadRotationY       float32 `json:"max_head_rotation_y"`
	// TakeGoal is the goal a mob must have to be offered the flower.
	TakeGoal string `json:"-"`
	// StopEvent is the actor event sent when the offer ends.
	StopEvent int32 `json:"-"`
}

// DefaultOfferFlower returns the goal's BDS defaults (TypeSchema
// 0xE0E6880; the goal name and event from 0xBDC6990 and 0xBDC6E10).
func DefaultOfferFlower() OfferFlowerConfig {
	return OfferFlowerConfig{MaxOfferFlowerDuration: 20, ChanceToStart: 0.000125, SearchArea: Vec3{X: 6, Y: 2, Z: 6},
		MaxRotationX: 30, MaxHeadRotationY: 30, TakeGoal: "minecraft:behavior.take_flower", StopEvent: 20}
}

// OfferFlower is OfferFlowerGoal: an iron golem holds a flower out to a
// mob that can take it.
type OfferFlower struct {
	base
	m      FlowerOfferer
	cfg    OfferFlowerConfig
	target ID
	end    uint64
	near   []Actor
}

// NewOfferFlower makes the goal (0xBDC68E0).
func NewOfferFlower(w World, m FlowerOfferer, c OfferFlowerConfig) *OfferFlower {
	return &OfferFlower{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, target: NoID}
}

// CanUse implements goal.Goal (0xBDC6990).
func (g *OfferFlower) CanUse() bool {
	if g.world.Random().NextFloat() > g.cfg.ChanceToStart {
		return false
	}
	m := g.m
	if g.cfg.Filters != nil && !g.cfg.Filters(m, m) {
		return false
	}
	p, a := m.Pos(), g.cfg.SearchArea
	g.near = g.world.Entities(g.near[:0], m.ID(), Box{Min: sub(p, a), Max: add(p, a)})
	for _, c := range g.near {
		if c.IsMob() && m.HasGoal(c.ID(), g.cfg.TakeGoal) {
			g.target = c.ID()
			return true
		}
	}
	return false
}

// CanContinueToUse implements goal.Goal (0xBDC6C80).
func (g *OfferFlower) CanContinueToUse() bool {
	return g.actor(g.target) != nil && g.world.CurrentTick() < g.end
}

// Start implements goal.Goal (0xBDC6DD0).
func (g *OfferFlower) Start() {
	g.end = g.world.CurrentTick() + uint64(int32(g.cfg.MaxOfferFlowerDuration*ticksPerSecond))
	g.m.OfferFlower()
}

// Stop implements goal.Goal (0xBDC6E10).
func (g *OfferFlower) Stop() {
	g.m.StopOfferingFlower(g.mob.ID(), g.cfg.StopEvent)
	g.target = NoID
}

// Tick implements goal.Goal (0xBDC6F10).
func (g *OfferFlower) Tick() {
	if t := g.actor(g.target); t != nil {
		g.mob.LookAt(lookPoint(t), g.cfg.MaxHeadRotationY, g.cfg.MaxRotationX)
	}
}

// TakeFlowerConfig is the JSON of minecraft:behavior.take_flower.
type TakeFlowerConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	Filters         Filter  `json:"-"`
	// MinWaitTime and MaxWaitTime are taken from the offer's length as
	// whole numbers of ticks, the way BDS casts them.
	MinWaitTime         float32  `json:"min_wait_time"`
	MaxWaitTime         float32  `json:"max_wait_time"`
	SearchArea          Vec3     `json:"search_area"`
	MaxRotationX        float32  `json:"max_rotation_x"`
	MaxHeadRotationY    float32  `json:"max_head_rotation_y"`
	MinDistanceToTarget float32  `json:"min_distance_to_target"`
	OnTakeFlower        []string `json:"on_take_flower"`
	StopEvent           int32    `json:"-"`
}

// DefaultTakeFlower returns the goal's BDS defaults (TypeSchema
// 0xE0EFB30).
func DefaultTakeFlower() TakeFlowerConfig {
	return TakeFlowerConfig{SpeedMultiplier: 0.5, MinWaitTime: 4, MaxWaitTime: 20, SearchArea: Vec3{X: 6, Y: 2, Z: 6},
		MaxRotationX: 30, MaxHeadRotationY: 30, MinDistanceToTarget: 2, StopEvent: 20}
}

// TakeFlower is TakeFlowerGoal: a mob waits for a golem's offer, walks
// up and takes the flower.
type TakeFlower struct {
	base
	m       FlowerOfferer
	cfg     TakeFlowerConfig
	target  ID
	takeAt  int32
	walking bool
	near    []Actor
}

// NewTakeFlower makes the goal (0x5371FF0).
func NewTakeFlower(w World, m FlowerOfferer, c TakeFlowerConfig) *TakeFlower {
	return &TakeFlower{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, target: NoID}
}

// CanUse implements goal.Goal (0x5342340).
func (g *TakeFlower) CanUse() bool {
	m := g.m
	if m.Nav() == nil || g.cfg.Filters != nil && !g.cfg.Filters(m, m) {
		return false
	}
	b, a := m.Box(), g.cfg.SearchArea
	g.near = g.world.Entities(g.near[:0], m.ID(), Box{Min: sub(b.Min, a), Max: add(b.Max, a)})
	for _, c := range g.near {
		if !c.IsMob() {
			continue
		}
		if _, ok := m.OfferFlowerTicks(c.ID()); ok {
			g.target = c.ID()
			return true
		}
	}
	return false
}

// CanContinueToUse implements goal.Goal (0x5342700).
func (g *TakeFlower) CanContinueToUse() bool {
	if g.actor(g.target) == nil {
		return false
	}
	_, ok := g.m.OfferFlowerTicks(g.target)
	return ok
}

// Start implements goal.Goal (0x53428F0): the mob takes the flower some
// way into the offer.
func (g *TakeFlower) Start() {
	g.walking = false
	if g.actor(g.target) == nil {
		return
	}
	if d, ok := g.m.OfferDuration(g.target); ok {
		g.takeAt = d - randomInt(g.world.Random(), int32(g.cfg.MinWaitTime), int32(g.cfg.MaxWaitTime))
	}
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
}

// Stop implements goal.Goal (0x5342CF0).
func (g *TakeFlower) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.target = NoID
}

// Tick implements goal.Goal (0x5342E20).
func (g *TakeFlower) Tick() {
	t := g.actor(g.target)
	if t == nil {
		return
	}
	g.mob.LookAt(lookPoint(t), g.cfg.MaxHeadRotationY, g.cfg.MaxRotationX)
	if !g.walking {
		ticks, ok := g.m.OfferFlowerTicks(g.target)
		if !ok || ticks != g.takeAt {
			return
		}
		if n := g.mob.Nav(); n != nil {
			n.MoveToActor(t, g.cfg.SpeedMultiplier)
		}
		g.walking = true
	}
	// BDS compares the squared distance with the plain setting.
	if g.cfg.MinDistanceToTarget <= dist2(g.mob.Pos(), t.Pos()) {
		return
	}
	g.m.StopOfferingFlower(g.target, g.cfg.StopEvent)
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	for _, e := range g.cfg.OnTakeFlower {
		g.mob.Event(e, g.target)
	}
}
