package state

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// FeedItem is one of minecraft:ageable's feed_items.
type FeedItem struct {
	Item
	// Growth is the share of the time as a baby that feeding the item
	// takes off.
	Growth float32
}

// AgeableConfig is minecraft:ageable.
type AgeableConfig struct {
	// Duration is the time as a baby in seconds; -1 never grows up.
	Duration  float32
	FeedItems []FeedItem
	// DropItems are dropped when the mob grows up.
	DropItems []string
	GrowUp    *definition.Trigger
	// InteractFilters must pass, with the player as other, for feeding.
	InteractFilters *definition.Filter
	// TransformToItem is what the fed item turns into; the engine hands it
	// back.
	TransformToItem string
	// PauseGrowthItems and ResetGrowthItems are read from 1.26.60 data but
	// not used: their code was not read.
	PauseGrowthItems, ResetGrowthItems []string
}

// DefaultAgeable is BDS's ageable: 1200 seconds as a baby (1.21.3
// AgeableDefinition::buildSchema 0x14055F940).
func DefaultAgeable() AgeableConfig { return AgeableConfig{Duration: 1200} }

// DefaultFeedGrowth is a feed item's growth when it gives none (1.21.3
// AgeableDefinition::addFeedItemByName 0x14055D380).
func DefaultFeedGrowth() float32 { return 0.1 }

// Ageable is minecraft:ageable: a baby's age counts up to 0, when the mob
// grows up (1.21.3 _tickAgeableComponent 0x14015B630,
// AgeableDefinition::initialize 0x140569800).
type Ageable struct {
	cfg AgeableConfig
	age int32
}

// NewAgeable makes the component and sets the mob's baby flag, as BDS's
// initialize does: a mob that can grow up starts at minus its time as a
// baby.
func NewAgeable(c AgeableConfig, m Mob) *Ageable {
	a := &Ageable{cfg: c}
	if a.CanGrowUp() {
		a.age = min(-int32(abs32(c.Duration)*ticksPerSecond), 0)
	}
	if m != nil {
		m.SetFlag(behavior.FlagBaby, a.age < 0)
	}
	return a
}

// Config returns the settings.
func (a *Ageable) Config() AgeableConfig { return a.cfg }

// SetConfig changes the settings; the age is kept.
func (a *Ageable) SetConfig(c AgeableConfig) { a.cfg = c }

// Age is the age in ticks: below 0 for a baby.
func (a *Ageable) Age() int32 { return a.age }

// SetAge sets the age, as AgeableComponent::setAge: never above 0.
func (a *Ageable) SetAge(age int32) { a.age = min(age, 0) }

// CanGrowUp reports a duration other than -1.
func (a *Ageable) CanGrowUp() bool { return a.cfg.Duration != -1 }

// TicksAsBaby is the duration in ticks.
func (a *Ageable) TicksAsBaby() int32 { return int32(abs32(a.cfg.Duration) * ticksPerSecond) }

// Tick ages a baby by one tick. On growing up it clears the baby flag,
// fires grow_up and drops drop_items. BDS also makes a call with the value
// 76 (vtable 592) whose meaning was not read; it is left out.
func (a *Ageable) Tick(env *Env) {
	m := env.Self
	if !a.CanGrowUp() || a.age >= 0 && !m.Flag(behavior.FlagBaby) {
		return
	}
	next := a.age + 1
	a.SetAge(next)
	if next < 0 {
		return
	}
	m.SetFlag(behavior.FlagBaby, false)
	env.fire(a.cfg.GrowUp, env.tester(m, nil))
	for _, it := range a.cfg.DropItems {
		m.DropItem(it)
	}
}

// Feed is a player feeding the mob the item in hand. It returns the feed
// item used, or -1 when the mob does not take it (angry, cannot grow up,
// interact_filters fail, or no feed item matches). The engine uses up the
// item and hands back its result.
//
// BDS's feed action was not read (a captured callback); the age moves by
// Growth times the time as a baby, as Mojang's documentation describes
// growth.
func (a *Ageable) Feed(env *Env, player behavior.Actor, h Hand) int {
	m := env.Self
	if m.Flag(behavior.FlagAngry) || !a.CanGrowUp() {
		return -1
	}
	if !a.cfg.InteractFilters.Eval(env.tester(m, player)) {
		return -1
	}
	i := -1
	for j := range a.cfg.FeedItems {
		if h != nil && h.Matches(a.cfg.FeedItems[j].Item.Item) {
			i = j
			break
		}
	}
	if i < 0 {
		return -1
	}
	a.SetAge(a.age + int32(a.cfg.FeedItems[i].Growth*float32(a.TicksAsBaby())))
	return i
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
