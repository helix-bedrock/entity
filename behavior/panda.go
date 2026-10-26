package behavior

// The panda goals' actor flags, by BDS's actor flag number.
const (
	FlagLayingDown = 64
	FlagSneezing   = 65
	FlagRolling    = 67
	FlagScared     = 68
)

// LayDownConfig is the JSON of minecraft:behavior.lay_down. Both are in
// ticks: 1 in interval starts it, 1 in random_stop_interval stops it.
type LayDownConfig struct {
	Interval           int32
	RandomStopInterval int32
}

// DefaultLayDown is the goal's BDS defaults.
var DefaultLayDown = LayDownConfig{Interval: 120, RandomStopInterval: 120}

// LayDown is LayDownGoal: the panda lies down now and then.
type LayDown struct {
	base
	cfg LayDownConfig
}

// NewLayDown makes the goal.
func NewLayDown(w World, m Mob, c LayDownConfig) *LayDown {
	return &LayDown{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c}
}

// CanUse implements goal.Goal (0x5750550).
func (g *LayDown) CanUse() bool {
	if g.mob.InWater() {
		return false
	}
	return g.cfg.Interval == 0 || g.world.Random().NextInt(g.cfg.Interval) == 0
}

// CanContinueToUse implements goal.Goal (0x5750650).
func (g *LayDown) CanContinueToUse() bool {
	return g.cfg.RandomStopInterval != 0 && g.world.Random().NextInt(g.cfg.RandomStopInterval) != 0
}

// Start implements goal.Goal.
func (g *LayDown) Start() { g.mob.SetFlag(FlagLayingDown, true) }

// Stop implements goal.Goal.
func (g *LayDown) Stop() { g.mob.SetFlag(FlagLayingDown, false) }

// Tick implements goal.Goal.
func (g *LayDown) Tick() {}

// The scared goal's fixed numbers.
const (
	scaredThunder = 0.9 // the thunder level above which pandas are scared
	scaredSound   = 243
)

// ScaredConfig is the JSON of minecraft:behavior.scared. SoundInterval
// is sound_delay in ticks: 1 in it plays the sound each tick.
type ScaredConfig struct {
	SoundInterval int32
}

// DefaultScared is the goal's BDS defaults: no sound.
var DefaultScared = ScaredConfig{}

// Scared is ScaredGoal: the panda covers its eyes in a thunderstorm.
type Scared struct {
	base
	cfg ScaredConfig
}

// NewScared makes the goal.
func NewScared(w World, m Mob, c ScaredConfig) *Scared {
	return &Scared{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c}
}

// CanUse implements goal.Goal (0x5926D10).
func (g *Scared) CanUse() bool {
	return !g.mob.Flag(FlagSitting) && g.world.ThunderLevel() > scaredThunder
}

// CanContinueToUse implements goal.Goal (0x5926F50).
func (g *Scared) CanContinueToUse() bool { return g.world.ThunderLevel() > scaredThunder }

// Start implements goal.Goal (0x5926DE0).
func (g *Scared) Start() {
	m := g.mob
	m.SetFlag(FlagScared, true)
	if m.Flag(FlagTransitionSitting) {
		m.SetFlag(FlagSitting, false)
	}
	m.SetFlag(FlagTransitionSitting, true)
}

// Stop implements goal.Goal (0x5926E40): the panda is left sitting.
func (g *Scared) Stop() {
	m := g.mob
	m.SetFlag(FlagScared, false)
	m.SetFlag(FlagSitting, true)
	m.SetFlag(FlagTransitionSitting, true)
}

// Tick implements goal.Goal (0x5926E80).
func (g *Scared) Tick() {
	if g.cfg.SoundInterval > 0 && g.world.IRandom().NextInt(g.cfg.SoundInterval) == 0 {
		g.mob.PlaySoundAt(scaredSound, g.mob.AttachPos(attachBody))
	}
}

// The sneeze goal's fixed numbers.
const (
	sneezeParticleEvent = 2009 // LevelEvent ParticleGenericSpawn
	sneezeParticle      = 62   // the sneeze particle's type
	sneezeParticleDrop  = 0.1  // the particle sits this far below the eyes
)

// Sneezer is the part of the mob sneeze needs on top of Mob.
type Sneezer interface {
	Mob
	// MakeJump sets the entity's jump control to jump (JumpControlComponent
	// +0), so a startled mob hops.
	MakeJump(a Actor)
	// LevelEventData broadcasts the level event at p with a number as its
	// data (Level vtable +1688).
	LevelEventData(id int32, p Vec3, data int32)
	// DropLoot rolls the loot table and drops what it gives at the mob.
	DropLoot(table string)
}

// SneezeConfig is the JSON of minecraft:behavior.sneeze. Cooldown is
// cooldown_time in ticks (BDS truncates the seconds times 20); PrepareTime
// is in whole seconds.
type SneezeConfig struct {
	Cooldown       int32
	Probability    float32 `json:"probability"`
	DropItemChance float32 `json:"drop_item_chance"`
	LootTable      string  `json:"loot_table"`
	Sound          int
	PrepareSound   int
	PrepareTime    float32 `json:"prepare_time"`
	// EntityTypes are the filters of the mobs that jump at the sneeze,
	// tested with the sneezing mob as self and the mob as other.
	EntityTypes  []Filter
	WithinRadius float32 `json:"within_radius"`
}

// DefaultSneeze is the goal's BDS defaults.
var DefaultSneeze = SneezeConfig{
	Probability: 0.02, DropItemChance: 1, Sound: NoSound, PrepareSound: NoSound, PrepareTime: 1,
}

// Sneeze is SneezeGoal: the baby panda gets ready, sneezes, startles the
// mobs around it and may drop its loot.
type Sneeze struct {
	base
	m        Sneezer
	cfg      SneezeConfig
	cooldown int32
	timer    int32
	near     []Actor
}

// NewSneeze makes the goal; the first sneeze waits a full cooldown.
func NewSneeze(w World, m Sneezer, c SneezeConfig) *Sneeze {
	return &Sneeze{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, cooldown: c.Cooldown}
}

// CanUse implements goal.Goal (0x5931D70).
func (g *Sneeze) CanUse() bool {
	if g.cooldown > 0 {
		g.cooldown--
		return false
	}
	return g.world.IRandom().NextFloat() <= g.cfg.Probability
}

// CanContinueToUse implements goal.Goal (0x5931DB0).
func (g *Sneeze) CanContinueToUse() bool { return g.timer > 0 }

// Start implements goal.Goal (0x5931DC0).
func (g *Sneeze) Start() {
	g.m.SetFlag(FlagSneezing, true)
	if g.cfg.PrepareSound != NoSound {
		g.m.PlaySoundAt(g.cfg.PrepareSound, g.m.AttachPos(attachHead))
	}
	g.timer = ticksPerSecond * int32(g.cfg.PrepareTime)
}

// Stop implements goal.Goal (0x5931EA0).
func (g *Sneeze) Stop() {
	g.m.SetFlag(FlagSneezing, false)
	g.cooldown = g.cfg.Cooldown
}

// Tick implements goal.Goal (0x5931ED0): when the timer runs out the
// sneeze happens.
func (g *Sneeze) Tick() {
	if g.timer > 0 {
		g.timer--
		if g.timer != 0 {
			return
		}
	}
	m := g.m
	if g.cfg.Sound != NoSound {
		m.PlaySoundAt(g.cfg.Sound, m.AttachPos(attachHead))
	}
	p, b := m.Pos(), m.Box()
	r := (b.Max.X - b.Min.X + 1) * 0.5
	a := m.BodyYaw() * radPerDeg * sinScale
	m.LevelEventData(sneezeParticleEvent, Vec3{
		X: p.X - tableSin(uint16(int32(a)))*r,
		Y: m.AttachPos(attachEyes).Y - sneezeParticleDrop,
		Z: r*tableSin(uint16(int32(a+sinQuarter))) + p.Z,
	}, sneezeParticle)
	if len(g.cfg.EntityTypes) != 0 {
		g.startle()
	}
	if g.cfg.DropItemChance > 0 && g.cfg.DropItemChance >= g.world.Random().NextFloat() && g.cfg.LootTable != "" {
		m.DropLoot(g.cfg.LootTable)
	}
}

// startle makes every matching mob near the sneeze jump, unless it rides
// the panda or the panda rides it.
func (g *Sneeze) startle() {
	m := g.m
	r := g.cfg.WithinRadius
	b := m.Box()
	g.near = g.world.Entities(g.near[:0], m.ID(), sortedBox(sub(b.Min, Vec3{X: r, Y: r, Z: r}), add(b.Max, Vec3{X: r, Y: r, Z: r})))
	for _, e := range g.near {
		if !e.IsMob() {
			continue
		}
		for _, f := range g.cfg.EntityTypes {
			if f != nil && !f(m, e) {
				continue
			}
			if !contains(m.Riders(), e.ID()) && !contains(e.Riders(), m.ID()) {
				m.MakeJump(e)
			}
		}
	}
}
