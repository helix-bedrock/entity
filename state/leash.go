package state

import (
	"math"
	"slices"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// ChunkState is how far the chunks around a box are loaded, as BDS's
// ChunksLoadedInfo::areAllChunksLoadedAndTicking (0x141EE26A0) answers.
type ChunkState uint8

// The chunk states, by BDS number.
const (
	ChunksMissing ChunkState = iota
	ChunksLoaded
	ChunksTicking
)

// LeashChunks is the engine's answer about the chunks a leashed mob's
// missing holder could be in.
type LeashChunks interface {
	// BoxState is the state of every chunk the box overlaps.
	BoxState(b behavior.Box) ChunkState
}

// LeashPreset is one of minecraft:leashable's presets: the leash distances
// used while the holder passes its filter. SpringType and
// RotationAdjustment are held for the engine's leash physics and
// rendering; entity does not use them.
type LeashPreset struct {
	// Filter is tested with the mob as self and the holder as other; nil
	// passes.
	Filter *definition.Filter
	// SoftDistance is where the mob starts walking to the holder,
	// HardDistance where the leash pulls it and MaxDistance where it
	// breaks.
	SoftDistance, HardDistance, MaxDistance float32
	SpringType                              string
	RotationAdjustment                      float32
}

// DefaultLeashPreset is a preset's defaults (1.26.60 parser 0x71258E0:
// soft 4, hard 6, max 10). rotation_adjustment's default was not read.
func DefaultLeashPreset() LeashPreset {
	return LeashPreset{SoftDistance: 4, HardDistance: 6, MaxDistance: 10}
}

// LeashableConfig is minecraft:leashable.
type LeashableConfig struct {
	// CanBeCut and UnleashOnRemoval are the engine's (shears, and dropping
	// the leash when the holder is removed).
	CanBeCut, UnleashOnRemoval bool
	// CanBeStolen lets another player take the leash from its holder.
	CanBeStolen bool
	OnLeash     *definition.Trigger
	OnUnleash   *definition.Trigger
	// OnUnleashInteractOnly fires OnUnleash only when a player unleashes
	// the mob by interacting.
	OnUnleashInteractOnly bool
	// Presets are tried in order; with none, or none passing, the default
	// preset is used.
	Presets []LeashPreset
	// LeadItem is the item descriptor a player leashes with.
	LeadItem string
	// UnstealableHolders are the entity types a leash is never stolen
	// from (BDS's player and leash knot, type IDs 319 and 88).
	UnstealableHolders []string
	// LeashText and UnleashText are the interact button texts.
	LeashText, UnleashText string
	// InteractEvent is the game event leashing and unleashing post.
	InteractEvent string
	// PathSpeed is the speed the mob walks to its holder at; PullFactor
	// scales the pull past the hard distance; ChunkMargin grows the mob's
	// box for the chunk check while the holder is missing.
	PathSpeed, PullFactor, ChunkMargin float32
}

// DefaultLeashable is BDS's leashable (1.26.60 parser 0x71258E0: can_be_cut,
// can_be_stolen and unleash_on_removal true; the lead, texts, speed 1, pull
// 0.2 and margin 16 from the 1.21.3 getInteraction 0x1405E2E80 and
// _tickLeashableComponent 0x14015DA60).
func DefaultLeashable() LeashableConfig {
	return LeashableConfig{
		CanBeCut: true, CanBeStolen: true, UnleashOnRemoval: true,
		LeadItem:           "minecraft:lead",
		UnstealableHolders: []string{"minecraft:player", "minecraft:leash_knot"},
		LeashText:          "action.interact.leash", UnleashText: "action.interact.unleash",
		InteractEvent: "minecraft:entity_interact",
		PathSpeed:     1, PullFactor: 0.2, ChunkMargin: 16,
	}
}

// Leashable is minecraft:leashable (1.21.3 _tickLeashableComponent
// 0x14015DA60, LeashableComponent::leash 0x1405E7300, unleash 0x1405EDBF0,
// getInteraction 0x1405E2E80). The leash holder itself is the engine's
// (Mob.LeashHolder).
type Leashable struct{ cfg LeashableConfig }

// NewLeashable makes the component.
func NewLeashable(c LeashableConfig) *Leashable { return &Leashable{cfg: c} }

// Config returns the settings.
func (l *Leashable) Config() LeashableConfig { return l.cfg }

// SetConfig changes the settings.
func (l *Leashable) SetConfig(c LeashableConfig) { l.cfg = c }

// Preset is the preset in use for a holder: the first whose filter
// passes, else the default. How 1.26.60 picks one was not read.
func (l *Leashable) Preset(env *Env, holder behavior.Actor) LeashPreset {
	ctx := env.tester(env.Self, holder)
	for i := range l.cfg.Presets {
		if l.cfg.Presets[i].Filter.Eval(ctx) {
			return l.cfg.Presets[i]
		}
	}
	return DefaultLeashPreset()
}

// Tick keeps a leashed mob near its holder. The leash breaks past the
// max distance or when the holder is dead or a spectator. Past the soft
// distance a mob that is not sitting and carries no player walks to the
// holder; past the hard distance it is pulled. With the holder not found,
// the leash breaks once every chunk around the mob is loaded and ticking;
// until then the mob stops and waits.
func (l *Leashable) Tick(env *Env, chunks LeashChunks) {
	m := env.Self
	id := m.LeashHolder()
	if id == behavior.NoID {
		return
	}
	h := env.World.Actor(id)
	if h == nil {
		b := m.Box()
		g := l.cfg.ChunkMargin
		b.Min = behavior.Vec3{X: b.Min.X - g, Y: b.Min.Y - g, Z: b.Min.Z - g}
		b.Max = behavior.Vec3{X: b.Max.X + g, Y: b.Max.Y + g, Z: b.Max.Z + g}
		if chunks != nil && chunks.BoxState(b) == ChunksTicking {
			l.drop(env, false, true)
			return
		}
		if n := m.Nav(); n != nil {
			n.Stop()
		}
		return
	}
	if !h.Alive() || behavior.Spectator(env.World, h) {
		l.drop(env, true, true)
		return
	}
	p := l.Preset(env, h)
	hp, mp := h.Pos(), m.Pos()
	d := behavior.Vec3{X: hp.X - mp.X, Y: hp.Y - mp.Y, Z: hp.Z - mp.Z}
	d2 := d.X*d.X + d.Y*d.Y + d.Z*d.Z
	if d2 > p.MaxDistance*p.MaxDistance {
		l.drop(env, true, true)
		return
	}
	if m.Flag(behavior.FlagSitting) || l.playerPassenger(env) {
		return
	}
	if d2 > p.SoftDistance*p.SoftDistance && m.IsMob() {
		if n := m.Nav(); n != nil {
			n.MoveTo(hp, l.cfg.PathSpeed)
		}
	}
	if d2 > p.HardDistance*p.HardDistance {
		inv := 1 / float32(math.Sqrt(float64(d2)))
		n := behavior.Vec3{X: d.X * inv, Y: d.Y * inv, Z: d.Z * inv}
		v := m.Velocity()
		v.X += abs32(n.X) * n.X * l.cfg.PullFactor
		v.Y += abs32(n.Y) * n.Y * l.cfg.PullFactor
		v.Z += abs32(n.Z) * n.Z * l.cfg.PullFactor
		m.SetVelocity(v)
	}
}

func (l *Leashable) playerPassenger(env *Env) bool {
	for _, id := range env.Self.Riders() {
		if r := env.World.Actor(id); r != nil && r.IsPlayer() {
			return true
		}
	}
	return false
}

// drop breaks the leash. BDS's tick calls Actor::dropLeash directly; that
// a broken leash fires on_unleash (unless on_unleash_interact_only) is
// read from the 1.26.60 member, not from code.
func (l *Leashable) drop(env *Env, dropItem, interact bool) {
	if !l.cfg.OnUnleashInteractOnly {
		env.fire(l.cfg.OnUnleash, env.tester(env.Self, nil))
	}
	env.Self.DropLeash(dropItem, interact)
}

// LeashAction is what a player's interaction with a leashable mob does.
type LeashAction uint8

// The leash actions.
const (
	LeashNone LeashAction = iota
	// LeashAttach leashes the mob to the player (stealing it from another
	// holder when allowed); the engine uses up the lead.
	LeashAttach
	// LeashDetach unleashes the mob from the player who holds it.
	LeashDetach
)

// Interaction is what a player holding h can do to the mob, and the
// button text. An unleashed or stealable mob is leashed with a lead; a
// mob the player holds is unleashed.
func (l *Leashable) Interaction(env *Env, player behavior.Actor, h Hand) (LeashAction, string) {
	m := env.Self
	id := m.LeashHolder()
	if id == behavior.NoID || l.stealable(env, id) {
		if h != nil && h.Matches(l.cfg.LeadItem) {
			return LeashAttach, l.cfg.LeashText
		}
		return LeashNone, ""
	}
	if player != nil && player.ID() == id {
		return LeashDetach, l.cfg.UnleashText
	}
	return LeashNone, ""
}

func (l *Leashable) stealable(env *Env, holder behavior.ID) bool {
	if !l.cfg.CanBeStolen {
		return false
	}
	h := env.World.Actor(holder)
	return h != nil && !slices.Contains(l.cfg.UnstealableHolders, h.Type())
}

// Run does an interaction Interaction returned.
func (l *Leashable) Run(env *Env, a LeashAction, player behavior.Actor) {
	switch a {
	case LeashAttach:
		if id := env.Self.LeashHolder(); id != behavior.NoID {
			env.Self.DropLeash(false, true)
		}
		l.Leash(env, player)
	case LeashDetach:
		l.Unleash(env, true)
	}
}

// Leash ties the mob to a holder: it stops riding, posts the interact
// game event and fires on_leash.
func (l *Leashable) Leash(env *Env, holder behavior.Actor) {
	m := env.Self
	if holder == nil {
		return
	}
	m.SetLeashHolder(holder.ID())
	if m.Vehicle() != behavior.NoID {
		m.StopRiding()
	}
	m.GameEvent(l.cfg.InteractEvent, holder.ID(), m.Pos())
	env.fire(l.cfg.OnLeash, env.tester(m, holder))
}

// Unleash frees the mob, firing on_unleash and dropping the lead. A
// player holder posts the interact game event. interact marks an unleash
// by a player's interaction.
func (l *Leashable) Unleash(env *Env, interact bool) {
	m := env.Self
	if interact || !l.cfg.OnUnleashInteractOnly {
		env.fire(l.cfg.OnUnleash, env.tester(m, nil))
	}
	if h := env.World.Actor(m.LeashHolder()); h != nil && h.IsPlayer() {
		m.GameEvent(l.cfg.InteractEvent, h.ID(), m.Pos())
	}
	m.DropLeash(true, false)
}

// LeashableToConfig is minecraft:leashable_to, which lets other mobs be
// leashed to this one (1.26.60 parser 0x6FDCE40: can_retrieve_from false,
// unleash_on_removal true). Both are the engine's.
type LeashableToConfig struct {
	CanRetrieveFrom, UnleashOnRemoval bool
}

// DefaultLeashableTo is BDS's leashable_to.
func DefaultLeashableTo() LeashableToConfig { return LeashableToConfig{UnleashOnRemoval: true} }
