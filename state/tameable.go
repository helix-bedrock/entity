package state

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// TameableConfig is minecraft:tameable.
type TameableConfig struct {
	// Probability is the chance each tame item tames the mob.
	Probability float32
	TameItems   []Item
	TameEvent   *definition.Trigger
	// FailEvent and SuccessEvent are the actor events of a failed and a
	// successful try.
	FailEvent, SuccessEvent int
}

// DefaultTameable is BDS's tameable with a probability of 1. The 1.21.3
// schema (TameableDefinition::buildSchema 0x14062E080) reads the default
// from a data constant that was not read; 1 is Mojang's documented
// default. The event numbers are from _attemptToTame 0x140623CA0 and
// _becomeTame 0x140623EE0.
func DefaultTameable() TameableConfig {
	return TameableConfig{Probability: 1, FailEvent: EventTameFail, SuccessEvent: EventTameSuccess}
}

// Tameable is minecraft:tameable: a player tames the mob with a tame item
// (1.21.3 TameableComponent: _canTame 0x140624750, _attemptToTame
// 0x140623CA0, _becomeTame 0x140623EE0).
type Tameable struct{ cfg TameableConfig }

// NewTameable makes the component.
func NewTameable(c TameableConfig) *Tameable { return &Tameable{cfg: c} }

// Config returns the settings.
func (t *Tameable) Config() TameableConfig { return t.cfg }

// SetConfig changes the settings.
func (t *Tameable) SetConfig(c TameableConfig) { t.cfg = c }

// Feed is a player trying to tame the mob with the item in hand. It
// returns the tame item used, or -1 when the mob is tame already or the
// item is not a tame item. The engine uses up the item (outside creative)
// and hands back its result.
func (t *Tameable) Feed(env *Env, player behavior.Actor, h Hand) int {
	m := env.Self
	if m.Flag(behavior.FlagTamed) || player == nil {
		return -1
	}
	i := match(h, t.cfg.TameItems)
	if i < 0 {
		return -1
	}
	if env.World.Random().NextFloat() >= t.cfg.Probability {
		m.ActorEvent(t.cfg.FailEvent)
		return i
	}
	m.SetOwner(player.ID())
	t.Tame(env)
	if tg := env.World.Actor(m.Target()); tg != nil && (tg.ID() == player.ID() || tg.Owner() == player.ID()) {
		m.SetTarget(behavior.NoID)
	}
	return i
}

// Tame makes the mob tame, as _becomeTame: persistent, the success event,
// the tamed flag and tame_event. BDS also makes a vtable call (488) whose
// meaning was not read.
func (t *Tameable) Tame(env *Env) {
	m := env.Self
	m.SetPersistent()
	m.ActorEvent(t.cfg.SuccessEvent)
	m.SetFlag(behavior.FlagTamed, true)
	env.fire(t.cfg.TameEvent, env.tester(m, nil))
}

// TemperItem is one of minecraft:tamemount's feed_items.
type TemperItem struct {
	Item
	TemperMod int32
}

// TamemountConfig is minecraft:tamemount.
type TamemountConfig struct {
	MinTemper, MaxTemper int32
	// AttemptTemperMod is added to the temper after each failed ride.
	AttemptTemperMod int32
	FeedItems        []TemperItem
	AutoRejectItems  []Item
	// FeedText and RideText are the interact button texts.
	FeedText, RideText string
	TameEvent          *definition.Trigger
	// RideTicks is how long a player rides before a taming try.
	RideTicks int32
	// FailEvent and SuccessEvent are the actor events of a failed and a
	// successful try; HeartParticle and HeartCount the particles on
	// success.
	FailEvent, SuccessEvent   int
	HeartParticle, HeartCount int
}

// DefaultTamemount is BDS's tamemount (1.26.60 schema 0xC2BB260:
// min_temper 0, max_temper 100, attempt_temper_mod 5; the 60-tick ride
// from 1.21.3 MountTameableDefinition::initialize 0x1405E6A70).
func DefaultTamemount() TamemountConfig {
	return TamemountConfig{
		MaxTemper: 100, AttemptTemperMod: 5, RideTicks: 60,
		FailEvent: EventTameFail, SuccessEvent: EventTameSuccess, HeartParticle: ParticleHeart, HeartCount: 20,
	}
}

// Tamemount is minecraft:tamemount: a mount tamed by riding it until its
// temper gives in (1.21.3 tickMountTamingComponent 0x140181F00,
// MountTamingComponent::getInteraction 0x1405E3470, _feedItem 0x1405D3940,
// tameToPlayer 0x1405ED540).
type Tamemount struct {
	cfg     TamemountConfig
	temper  int32
	counter int32
}

// NewTamemount makes the component with a temper drawn between min and
// max.
func NewTamemount(c TamemountConfig, r behavior.Random) *Tamemount {
	t := &Tamemount{cfg: c, temper: c.MinTemper}
	if r != nil {
		t.temper = randRange(r, c.MinTemper, c.MaxTemper)
	}
	return t
}

// Config returns the settings.
func (t *Tamemount) Config() TamemountConfig { return t.cfg }

// SetConfig changes the settings; the temper is kept.
func (t *Tamemount) SetConfig(c TamemountConfig) { t.cfg = c }

// Temper is the current temper.
func (t *Tamemount) Temper() int32 { return t.temper }

// Tick counts a player's ride on an untamed mount. After RideTicks a draw
// between min and max temper at or above the temper throws the rider and
// raises the temper; a lower draw tames the mount to the rider.
func (t *Tamemount) Tick(env *Env) {
	m := env.Self
	riders := m.Riders()
	if len(riders) == 0 {
		t.counter = 0
		return
	}
	if m.Flag(behavior.FlagTamed) {
		return
	}
	p := env.World.Actor(riders[0])
	if p == nil || !p.IsPlayer() {
		return
	}
	t.counter++
	if t.counter < t.cfg.RideTicks {
		return
	}
	if randRange(env.World.Random(), t.cfg.MinTemper, t.cfg.MaxTemper) >= t.temper {
		t.temper += t.cfg.AttemptTemperMod
		m.DismountPassengers()
		m.MakeMad()
		m.ActorEvent(t.cfg.FailEvent)
		return
	}
	t.TameTo(env, p)
}

// TameTo tames the mount to a player: owner, persistent, the success event
// and particles, the tamed flag and tame_event. As in Tameable.Tame, a
// vtable call (488) is left out.
func (t *Tamemount) TameTo(env *Env, player behavior.Actor) {
	m := env.Self
	m.SetOwner(player.ID())
	m.SetPersistent()
	m.ActorEvent(t.cfg.SuccessEvent)
	m.Particles(t.cfg.HeartParticle, t.cfg.HeartCount)
	m.SetFlag(behavior.FlagTamed, true)
	env.fire(t.cfg.TameEvent, env.tester(m, nil))
}

// MountResult is what a player's item does to a tamemount.
type MountResult uint8

// The tamemount interactions.
const (
	InteractNone MountResult = iota
	// InteractFeed added the feed item's temper_mod.
	InteractFeed
	// InteractReject is an auto_reject_items item: the engine rejects it
	// (BDS's captured reject action was not read).
	InteractReject
)

// Feed is a player using the item in hand on the mount, which must have
// no rider. It returns what happened and the feed item used (-1 for
// none). The engine uses up a fed item.
func (t *Tamemount) Feed(env *Env, h Hand) (MountResult, int) {
	if len(env.Self.Riders()) > 0 || h == nil {
		return InteractNone, -1
	}
	for i := range t.cfg.FeedItems {
		if h.Matches(t.cfg.FeedItems[i].Item.Item) {
			t.temper += t.cfg.FeedItems[i].TemperMod
			return InteractFeed, i
		}
	}
	if match(h, t.cfg.AutoRejectItems) >= 0 {
		return InteractReject, -1
	}
	return InteractNone, -1
}
