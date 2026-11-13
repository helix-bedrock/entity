package behavior

import "math"

// The play_dead goal's fixed numbers.
const (
	FlagPlayingDead      = 98
	effectRegeneration   = 10
	playDeadNoDamage     = -1 // the last damage cause before any damage
	playDeadNoAmplifier  = 0
	playDeadEffectMinDur = -1 // shorter durations become 0
)

// PlayDeader is the part of the mob play_dead needs on top of Mob.
type PlayDeader interface {
	Mob
	// LastDamage is the mob's last damage: a stamp that changes with each
	// hit (Mob +792), its cause (-1 for none) and its amount.
	LastDamage() (stamp uint64, cause int, amount float32)
	// Health is the health attribute's current value as an integer.
	Health() int32
	// AddEffect gives the mob a mob effect for the ticks at the amplifier.
	AddEffect(id int, ticks, amplifier int32)
}

// PlayDeadConfig is the JSON of minecraft:behavior.play_dead. Duration is
// in ticks (BDS truncates the seconds times 20).
type PlayDeadConfig struct {
	Duration          int32
	RandomStartChance float32 `json:"random_start_chance"`
	ForceBelowHealth  int32   `json:"force_below_health"`
	RandomDamageRange Range[int32]
	// DamageSources are the damage cause values; empty or DamageAll matches
	// any damage.
	DamageSources     []int
	ApplyRegeneration bool `json:"apply_regeneration"`
	// Filters is tested with the mob as self and no other entity.
	Filters Filter
}

// DefaultPlayDead returns the goal's BDS defaults: duration 1 s.
func DefaultPlayDead() PlayDeadConfig {
	return PlayDeadConfig{Duration: 20, RandomStartChance: 1}
}

// PlayDead is PlayDeadGoal: a hurt axolotl plays dead, sometimes healing
// as it does.
type PlayDead struct {
	base
	m   PlayDeader
	cfg PlayDeadConfig
	// any is a damage_sources list with "all" in it (or none at all).
	any   bool
	stamp uint64
	until uint64
}

// NewPlayDead makes the goal.
func NewPlayDead(w World, m PlayDeader, c PlayDeadConfig) *PlayDead {
	g := &PlayDead{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, any: true}
	if len(c.DamageSources) != 0 {
		g.any = contains(c.DamageSources, DamageAll)
	}
	return g
}

// CanUse implements goal.Goal (0x575B0E0): once per hit, a matching cause,
// the filters, and below force_below_health or by chance with enough
// damage.
func (g *PlayDead) CanUse() bool {
	stamp, cause, amount := g.m.LastDamage()
	if stamp == g.stamp {
		return false
	}
	g.stamp = stamp
	if g.any {
		if cause == playDeadNoDamage {
			return false
		}
	} else if !contains(g.cfg.DamageSources, cause) {
		return false
	}
	if g.cfg.Filters != nil && !g.cfg.Filters(g.m, nil) {
		return false
	}
	if g.m.Health() < g.cfg.ForceBelowHealth {
		return true
	}
	r := g.world.Random()
	if g.cfg.RandomStartChance <= r.NextFloat() {
		return false
	}
	d := g.cfg.RandomDamageRange.Min
	if g.cfg.RandomDamageRange.Max > d {
		d += r.NextInt(g.cfg.RandomDamageRange.Max - d + 1)
	}
	return amount > float32(d)
}

// CanContinueToUse implements goal.Goal (0x575B560).
func (g *PlayDead) CanContinueToUse() bool { return g.world.CurrentTick() < g.until }

// Start implements goal.Goal (0x575B2C0).
func (g *PlayDead) Start() {
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
	if g.cfg.ApplyRegeneration {
		d := g.cfg.Duration
		if d < playDeadEffectMinDur {
			d = 0
		}
		g.m.AddEffect(effectRegeneration, d, playDeadNoAmplifier)
	}
	g.m.SetFlag(FlagPlayingDead, true)
	g.until = g.world.CurrentTick() + uint64(int64(g.cfg.Duration))
}

// Stop implements goal.Goal (0x575B520).
func (g *PlayDead) Stop() {
	g.m.SetFlag(FlagPlayingDead, false)
	g.m.SetTarget(NoID)
}

// Tick implements goal.Goal.
func (g *PlayDead) Tick() {}

// Begger is the part of the mob beg needs on top of Mob.
type Begger interface {
	Mob
	// Carried is the item the entity holds in its hand, ok false for none.
	Carried(a Actor) (item string, ok bool)
}

// BegConfig is the JSON of minecraft:behavior.beg. LookTime is in ticks
// (BDS multiplies the JSON's whole seconds by 20).
type BegConfig struct {
	Items        []string
	LookDistance float32 `json:"look_distance"`
	LookTime     Range[int32]
}

// DefaultBeg returns the goal's BDS defaults: look_time 2 to 4 s.
func DefaultBeg() BegConfig {
	return BegConfig{LookDistance: 8, LookTime: Range[int32]{Min: 40, Max: 80}}
}

// Beg is BegGoal: the wolf stares at a nearby player holding food it
// likes.
type Beg struct {
	base
	m       Begger
	cfg     BegConfig
	player  ID
	timer   int32
	players []Actor
}

// NewBeg makes the goal. It has no control flags unless the JSON gives
// some.
func NewBeg(w World, m Begger, c BegConfig) *Beg {
	return &Beg{base: base{mob: m, world: w}, m: m, cfg: c, player: NoID}
}

// wants reports a held item from the list.
func (g *Beg) wants(p Actor) bool {
	it, ok := g.m.Carried(p)
	return ok && contains(g.cfg.Items, it)
}

// CanUse implements goal.Goal (0x48C98A0): the nearest pickable player
// within look_distance, holding a listed item.
func (g *Beg) CanUse() bool {
	d := g.cfg.LookDistance
	r2 := d * d
	if d < 0 {
		r2 = math.MaxFloat32
	}
	var near Actor
	best := float32(math.MaxFloat32)
	g.players = g.world.Players(g.players[:0])
	for _, p := range g.players {
		if !p.Pickable() {
			continue
		}
		if d2 := dist2(p.Pos(), g.m.Pos()); r2 > d2 && best > d2 {
			best, near = d2, p
		}
	}
	if near == nil || near.Removed() || len(g.cfg.Items) == 0 || !g.wants(near) {
		return false
	}
	g.player = near.ID()
	return true
}

// CanContinueToUse implements goal.Goal (0x48C9AC0).
func (g *Beg) CanContinueToUse() bool {
	p := g.actor(g.player)
	if p == nil || !p.Alive() || !p.Pickable() {
		return false
	}
	d := g.cfg.LookDistance
	return dist2(g.m.Pos(), p.Pos()) <= d*d && g.timer > 0 && g.wants(p)
}

// Start implements goal.Goal (0x48C9CA0).
func (g *Beg) Start() {
	g.m.SetFlag(FlagInterested, true)
	g.timer = randomInt(g.world.Random(), g.cfg.LookTime.Min, g.cfg.LookTime.Max)
}

// Stop implements goal.Goal (0x48C9D90).
func (g *Beg) Stop() {
	g.m.SetFlag(FlagInterested, false)
	g.player = NoID
}

// Tick implements goal.Goal (0x48C9DE0).
func (g *Beg) Tick() {
	p := g.actor(g.player)
	if p == nil || !p.Alive() {
		return
	}
	g.m.LookAt(p.AttachPos(attachHead), lookYaw, g.m.MaxHeadXRot())
	g.timer--
}
