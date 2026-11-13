package behavior

import "math"

// The skeleton_horse_trap numbers that are pure maths.
const (
	horseTrapPushScale    = 0.5 // the share of each Gaussian in the horse's push
	horseTrapEnchantFloor = 5
	horseTrapEnchantTop   = 18
)

// HorseTrapper is the part of the mob skeleton_horse_trap needs on top of
// Mob. Spawning is the engine's: the goal says what to spawn and where.
type HorseTrapper interface {
	Mob
	// Remove is Actor vt+96.
	Remove()
	// Difficulty is the level difficulty (Level vt+296), 0 for peaceful.
	Difficulty() int
	// SpecialMultiplier is Level vt+320 for the mob's dimension, the
	// moon-phase difficulty share.
	SpecialMultiplier() float32
	// LightningRod is 0xC9E6BD0: the lightning rod within 64 blocks a bolt
	// at p is drawn to.
	LightningRod(p Pos) (Pos, bool)
	// SpawnLightning spawns a lightning bolt (actor type 93) at p with the
	// mob as its spawner, one that sets fires and does damage (+924 0).
	SpawnLightning(p Vec3)
	// Spawn spawns an entity of the kind with the spawn event at p; false
	// when none was made.
	Spawn(kind, event string, p Vec3) (ID, bool)
	// SetInvulnerableTime sets the entity's Actor +404 ticks (60 here).
	SetInvulnerableTime(id ID, ticks int32)
	// Push adds v to the entity's motion, the way Actor's push helper does
	// (some kinds ignore it).
	Push(id ID, v Vec3)
	// TameTo is Mob vt+1440 with false (Horse 0xBD318C0) on a spawned horse
	// for the nearest player.
	TameTo(id, player ID)
	// HeadEmpty reports nothing in the entity's head armour slot.
	HeadEmpty(id ID) bool
	// GiveEnchantedHead puts a stack of one of the item, randomly enchanted
	// at the level (0xBF20470), in the head slot.
	GiveEnchantedHead(id ID, item string, level int32)
	// EnchantHand randomly enchants the carried item at the level.
	EnchantHand(id ID, level int32)
	// Ride makes the rider ride the mount (Actor vt+224).
	Ride(rider, mount ID)
}

// SkeletonHorseTrapConfig is the JSON of
// minecraft:behavior.skeleton_horse_trap and the spawns BDS writes into
// the goal.
type SkeletonHorseTrapConfig struct {
	// Duration is in seconds.
	Duration     float32 `json:"duration"`
	WithinRadius float32 `json:"within_radius"`
	// Horses and Rider are the entities the trap spawns, each with Event.
	Horses       int32  `json:"-"`
	Horse        string `json:"-"`
	Rider        string `json:"-"`
	Event        string `json:"-"`
	Helmet       string `json:"-"`
	InvulnerTime int32  `json:"-"`
}

// DefaultSkeletonHorseTrap returns the goal's BDS defaults (TypeSchema
// 0xDCB8150; spawns from the tick 0x592B240).
func DefaultSkeletonHorseTrap() SkeletonHorseTrapConfig {
	return SkeletonHorseTrapConfig{Duration: 1, Horses: 4, Horse: "minecraft:skeleton_horse",
		Rider: "minecraft:skeleton", Event: "minecraft:spring_trap", Helmet: "minecraft:iron_helmet", InvulnerTime: 60}
}

// SkeletonHorseTrap is SkeletonHorseTrapGoal: the trap horse waits for a
// player, calls down lightning and turns into armed skeleton riders.
type SkeletonHorseTrap struct {
	base
	m       HorseTrapper
	cfg     SkeletonHorseTrapConfig
	timer   int32
	players []Actor
}

// NewSkeletonHorseTrap makes the goal (0x5304AA0). It has no control
// flags.
func NewSkeletonHorseTrap(w World, m HorseTrapper, c SkeletonHorseTrapConfig) *SkeletonHorseTrap {
	return &SkeletonHorseTrap{base: base{mob: m, world: w}, m: m, cfg: c, timer: int32(c.Duration * ticksPerSecond)}
}

// nearest is the nearest living player closer than within_radius (any
// distance when it is negative).
func (g *SkeletonHorseTrap) nearest() Actor {
	r2 := float32(math.MaxFloat32)
	if r := g.cfg.WithinRadius; r >= 0 {
		r2 = r * r
	}
	best := float32(math.MaxFloat32)
	var near Actor
	g.players = g.world.Players(g.players[:0])
	p := g.m.Pos()
	for _, a := range g.players {
		if !a.Alive() {
			continue
		}
		if d := dist2(a.Pos(), p); r2 > d && best > d {
			best, near = d, a
		}
	}
	return near
}

// CanUse implements goal.Goal (0x592CF50): each call counts the trap down;
// once it runs out the trap horse is removed.
func (g *SkeletonHorseTrap) CanUse() bool {
	t := g.timer
	g.timer--
	if t <= 0 {
		g.m.Remove()
		return false
	}
	return g.nearest() != nil
}

// CanContinueToUse implements goal.Goal: BDS asks canUse again.
func (g *SkeletonHorseTrap) CanContinueToUse() bool { return g.CanUse() }

// Tick implements goal.Goal (0x592B240).
func (g *SkeletonHorseTrap) Tick() {
	m := g.m
	player := g.nearest()
	diff := m.Difficulty()
	at := m.Pos()
	if rod, ok := m.LightningRod(blockOf(at)); ok {
		at = Vec3{X: float32(rod.X), Y: float32(rod.Y), Z: float32(rod.Z)}
	}
	m.SpawnLightning(at)
	for range g.cfg.Horses {
		g.spawnHorse(player, diff)
	}
	m.Remove()
}

// spawnHorse spawns one horse of the trap and, off peaceful, its rider.
func (g *SkeletonHorseTrap) spawnHorse(player Actor, diff int) {
	m := g.m
	horse, ok := m.Spawn(g.cfg.Horse, g.cfg.Event, m.Pos())
	if !ok {
		return
	}
	m.SetInvulnerableTime(horse, g.cfg.InvulnerTime)
	r := g.world.Random()
	x := r.NextGaussian() * horseTrapPushScale
	z := r.NextGaussian() * horseTrapPushScale
	m.Push(horse, Vec3{X: x, Z: z})
	if player != nil {
		m.TameTo(horse, player.ID())
	}
	if diff == 0 {
		return
	}
	h := g.world.Actor(horse)
	if h == nil {
		return
	}
	rider, ok := m.Spawn(g.cfg.Rider, g.cfg.Event, h.Pos())
	if !ok {
		return
	}
	if m.HeadEmpty(rider) {
		m.GiveEnchantedHead(rider, g.cfg.Helmet, g.enchantLevel())
	}
	m.EnchantHand(rider, g.enchantLevel())
	m.SetInvulnerableTime(rider, g.cfg.InvulnerTime)
	m.Ride(rider, horse)
}

// enchantLevel is the trap's enchanting level: five plus the special
// multiplier, topped up at random toward 18.
func (g *SkeletonHorseTrap) enchantLevel() int32 {
	l := int32(g.m.SpecialMultiplier() + horseTrapEnchantFloor)
	if l < horseTrapEnchantTop {
		l += g.world.Random().NextInt(horseTrapEnchantTop - l)
	}
	return l
}

// The base Goal methods SkeletonHorseTrap keeps (1.26.60 vtable: start and stop are no-ops).

// Start implements goal.Goal.
func (g *SkeletonHorseTrap) Start() {}

// Stop implements goal.Goal.
func (g *SkeletonHorseTrap) Stop() {}
