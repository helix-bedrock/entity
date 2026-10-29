package state

import (
	"encoding/json"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// Interaction is one of minecraft:interact's interactions. entity decides
// which interaction runs and fires on_interact; the item work (use_item,
// hurt_item, transform_to_item, spawn_items, add_items, equip and drop
// slots, sounds, particles, swing, health_amount and the rest) is the
// engine's, read from Raw.
type Interaction struct {
	// Cooldown and CooldownAfterAttacked are in ticks.
	Cooldown, CooldownAfterAttacked int32
	OnInteract                      *definition.Trigger
	// GiveItem gives the held item to the mob; the engine blocks the
	// interaction while both the mob and the player hold something.
	GiveItem, TakeItem bool
	InteractText       string
	// Raw is the interaction's JSON, for the engine.
	Raw json.RawMessage
}

// InteractConfig is minecraft:interact.
type InteractConfig struct{ Interactions []Interaction }

// DefaultInteract is BDS's interact: no interactions.
func DefaultInteract() InteractConfig { return InteractConfig{} }

// Interact is minecraft:interact (1.21.3 InteractComponent:
// getInteraction 0x1405E2C60, _runInteraction 0x1405D61F0,
// _tickInteractComponent 0x14075D490). What runs once an interaction is
// picked is a captured callback that was not read; Run sets the cooldown
// and fires on_interact.
type Interact struct {
	cfg     InteractConfig
	counter int32
}

// NewInteract makes the component.
func NewInteract(c InteractConfig) *Interact { return &Interact{cfg: c} }

// Config returns the settings.
func (in *Interact) Config() InteractConfig { return in.cfg }

// SetConfig changes the settings.
func (in *Interact) SetConfig(c InteractConfig) { in.cfg = c }

// Tick counts the cooldown down.
func (in *Interact) Tick() {
	if in.counter > 0 {
		in.counter--
	}
}

// Find returns the first interaction a player can run on the mob, or -1:
// not cooling down, the mob not hurt by a mob within
// cooldown_after_being_attacked, and on_interact's filters passing with
// the player as other.
func (in *Interact) Find(env *Env, player behavior.Actor) int {
	m := env.Self
	ctx := env.tester(m, player)
	now := env.World.CurrentTick()
	for i := range in.cfg.Interactions {
		it := &in.cfg.Interactions[i]
		if it.Cooldown > 0 && in.counter > 0 {
			continue
		}
		if it.CooldownAfterAttacked > 0 && m.IsMob() {
			if by, at := m.LastHurtBy(); by != behavior.NoID && now-at < uint64(it.CooldownAfterAttacked) {
				continue
			}
		}
		if it.OnInteract == nil || it.OnInteract.Filters.Eval(ctx) {
			return i
		}
	}
	return -1
}

// Run runs interaction i for a player: it starts the cooldown and fires
// on_interact. The engine then does the item work.
func (in *Interact) Run(env *Env, i int, player behavior.Actor) {
	if i < 0 || i >= len(in.cfg.Interactions) {
		return
	}
	it := &in.cfg.Interactions[i]
	in.counter = it.Cooldown
	env.fire(it.OnInteract, env.tester(env.Self, player))
}

// GiveTrigger is one of minecraft:giveable's triggers.
type GiveTrigger struct {
	Items []string
	// Cooldown is in seconds.
	Cooldown float32
	OnGive   *definition.Trigger
}

// GiveableConfig is minecraft:giveable.
type GiveableConfig struct{ Triggers []GiveTrigger }

// Giveable is minecraft:giveable: a player gives the mob an item into its
// inventory (1.21.3 GiveableComponent::getInteraction 0x1405B1E50,
// GiveableDefinition::initialize 0x1405B47B0). The captured give action
// was not read; Give starts the cooldown and fires on_give.
type Giveable struct {
	cfg  GiveableConfig
	next []uint64
}

// NewGiveable makes the component.
func NewGiveable(c GiveableConfig) *Giveable {
	return &Giveable{cfg: c, next: make([]uint64, len(c.Triggers))}
}

// Config returns the settings.
func (g *Giveable) Config() GiveableConfig { return g.cfg }

// SetConfig changes the settings; the cooldowns restart.
func (g *Giveable) SetConfig(c GiveableConfig) {
	g.cfg = c
	g.next = make([]uint64, len(c.Triggers))
}

// Give is a player giving the item in hand. It returns the trigger used,
// or -1: the first trigger not cooling down whose items match, when the
// mob's inventory accepts the item (accepts, the engine's answer). The
// engine moves the item.
func (g *Giveable) Give(env *Env, player behavior.Actor, h Hand, accepts bool) int {
	if h == nil || !accepts {
		return -1
	}
	now := env.World.CurrentTick()
	for i := range g.cfg.Triggers {
		t := &g.cfg.Triggers[i]
		if i < len(g.next) && now < g.next[i] {
			continue
		}
		for _, it := range t.Items {
			if !h.Matches(it) {
				continue
			}
			if i < len(g.next) {
				g.next[i] = now + uint64(max(t.Cooldown*ticksPerSecond, 0))
			}
			env.fire(t.OnGive, env.tester(env.Self, player))
			return i
		}
	}
	return -1
}

// NameAction is one of minecraft:nameable's name_actions.
type NameAction struct {
	NameFilter []string
	OnNamed    *definition.Trigger
}

// NameableConfig is minecraft:nameable.
type NameableConfig struct {
	AllowNameTagRenaming, AlwaysShow bool
	NameActions                      []NameAction
	DefaultTrigger                   *definition.Trigger
}

// DefaultNameable is BDS's nameable (1.26.60 schema 0x71373A0:
// allow_name_tag_renaming true, always_show false).
func DefaultNameable() NameableConfig { return NameableConfig{AllowNameTagRenaming: true} }

// Nameable is minecraft:nameable (1.21.3 NameableComponent:
// getInteraction 0x1405E35F0, nameEntity 0x1405E7DC0).
type Nameable struct{ cfg NameableConfig }

// NewNameable makes the component.
func NewNameable(c NameableConfig) *Nameable { return &Nameable{cfg: c} }

// Config returns the settings.
func (n *Nameable) Config() NameableConfig { return n.cfg }

// SetConfig changes the settings.
func (n *Nameable) SetConfig(c NameableConfig) { n.cfg = c }

// CanRename reports that a named name tag may rename the mob. The engine
// checks the item.
func (n *Nameable) CanRename() bool { return n.cfg.AllowNameTagRenaming }

// Named fires the trigger for a new name once the engine has set the name
// tag, made it visible and made the mob persistent: the first name action
// listing the exact name, else default_trigger.
func (n *Nameable) Named(env *Env, name string) {
	for i := range n.cfg.NameActions {
		for _, f := range n.cfg.NameActions[i].NameFilter {
			if f == name {
				env.fire(n.cfg.NameActions[i].OnNamed, env.tester(env.Self, nil))
				return
			}
		}
	}
	env.fire(n.cfg.DefaultTrigger, env.tester(env.Self, nil))
}
