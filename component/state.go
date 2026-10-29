package component

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/state"
)

// The state component names.
const (
	AgeableName   = "minecraft:ageable"
	BreedableName = "minecraft:breedable"
	OffspringName = "minecraft:offspring"
	TameableName  = "minecraft:tameable"
	TamemountName = "minecraft:tamemount"
	AngryName     = "minecraft:angry"
	HealableName  = "minecraft:healable"
	DespawnName   = "minecraft:despawn"
)

// States makes a mob's state components from its components. A component
// whose JSON is the same in prevComps keeps its state from prev, so an
// unrelated group change does not reset a baby's age or a mount's temper;
// BDS's rule for this was not read. It also sets the flags of the flag
// components the mob gained or lost. env.Self gets the baby flag of a new
// ageable component and env.World draws the starting tempers and timers.
func States(c Context, env *state.Env, prev *state.Set, prevComps, comps definition.Group) (*state.Set, []error) {
	var errs []error
	c.warn = func(e error) { errs = append(errs, e) }
	if prev == nil {
		prev = &state.Set{}
	}
	same := func(names ...string) bool {
		for _, n := range names {
			old, ok := prevComps[n]
			now, ok2 := comps[n]
			if ok != ok2 || string(old) != string(now) {
				return false
			}
		}
		return true
	}
	read := func(name string) (Object, bool) {
		raw, ok := comps[name]
		if !ok {
			return nil, false
		}
		c.Name = name
		j, err := ParseObject(raw)
		if err != nil {
			c.Warn(err)
		}
		return j, true
	}
	var r behavior.Random
	if env.World != nil {
		r = env.World.Random()
	}
	s := &state.Set{}
	if j, ok := read(AgeableName); ok {
		if s.Ageable = prev.Ageable; s.Ageable == nil || !same(AgeableName) {
			s.Ageable = state.NewAgeable(c.AgeableConfig(j), env.Self)
		}
	}
	if j, ok := read(BreedableName); ok {
		cfg := c.BreedableConfig(j)
		if o, ok := read(OffspringName); ok {
			c.Offspring(o, &cfg)
		}
		if s.Breedable = prev.Breedable; s.Breedable == nil || !same(BreedableName, OffspringName) {
			s.Breedable = state.NewBreedable(cfg)
		}
	}
	if j, ok := read(TameableName); ok {
		s.Tameable = state.NewTameable(c.TameableConfig(j))
	}
	if j, ok := read(TamemountName); ok {
		if s.Tamemount = prev.Tamemount; s.Tamemount == nil || !same(TamemountName) {
			s.Tamemount = state.NewTamemount(c.TamemountConfig(j), r)
		}
	}
	if j, ok := read(AngryName); ok {
		if s.Angry = prev.Angry; s.Angry == nil || !same(AngryName) {
			s.Angry = state.NewAngry(c.AngryConfig(j), r)
		}
	}
	if j, ok := read(HealableName); ok {
		s.Healable = state.NewHealable(c.HealableConfig(j))
	}
	if j, ok := read(DespawnName); ok {
		s.Despawn = state.NewDespawn(c.DespawnConfig(j))
	}
	if j, ok := read(InteractName); ok {
		if s.Interact = prev.Interact; s.Interact == nil || !same(InteractName) {
			s.Interact = state.NewInteract(c.InteractConfig(j))
		}
	}
	if j, ok := read(GiveableName); ok {
		if s.Giveable = prev.Giveable; s.Giveable == nil || !same(GiveableName) {
			s.Giveable = state.NewGiveable(c.GiveableConfig(j))
		}
	}
	if j, ok := read(NameableName); ok {
		s.Nameable = state.NewNameable(c.NameableConfig(j))
	}
	if j, ok := read(RideableName); ok {
		s.Rideable = state.NewRideable(c.RideableConfig(j))
	}
	if j, ok := read(AddRiderName); ok {
		a := c.AddRiderConfig(j)
		s.AddRider = &a
	}
	if j, ok := read(TransformationName); ok {
		if s.Transformation = prev.Transformation; s.Transformation == nil || !same(TransformationName) {
			s.Transformation = state.NewTransformation(c.TransformationConfig(j), env.Self, r)
		}
	}
	if j, ok := read(HurtOnConditionName); ok {
		s.HurtOnCondition = state.NewHurtOnCondition(c.HurtOnConditionConfig(j))
	}
	if j, ok := read(BreathableName); ok {
		if s.Breathable = prev.Breathable; s.Breathable == nil || !same(BreathableName) {
			s.Breathable = state.NewBreathable(c.BreathableConfig(j), env.Self)
		}
	}
	if j, ok := read(BurnsInDaylightName); ok {
		s.BurnsInDaylight = state.NewBurnsInDaylight(c.BurnsInDaylightConfig(j))
	}
	if env.Self != nil {
		has := func(m definition.Group) func(string) bool {
			return func(n string) bool { _, ok := m[n]; return ok }
		}
		state.SetFlags(env.Self, state.FlagComponents(), has(prevComps), has(comps))
	}
	return s, errs
}

// alias returns the member name present: BDS reads several members under
// two names (broadcast_anger|broadcastAnger).
func alias(j Object, names ...string) string {
	for _, n := range names {
		if j.Has(n) {
			return n
		}
	}
	return names[0]
}

// filter reads a filter member, nil when missing.
func (c *Context) filter(j Object, k string) *definition.Filter {
	if !j.Has(k) {
		return nil
	}
	f, err := c.parser().ParseFilter(j[k])
	if err != nil {
		c.Warn(fmt.Errorf("%s: %w", k, err))
	}
	return f
}

// Items reads an item list: one entry or a list, each a descriptor or an
// object with item and result_item. Descriptors stay as written.
func (c *Context) Items(j Object, k string) []state.Item {
	if !j.Has(k) {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(j[k], &list) != nil {
		list = []json.RawMessage{j[k]}
	}
	out := make([]state.Item, 0, len(list))
	for _, raw := range list {
		out = append(out, c.item(raw))
	}
	return out
}

func first(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

// rawList splits a member that is one value or a list of them.
func rawList(raw json.RawMessage) []json.RawMessage {
	if raw == nil {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		list = []json.RawMessage{raw}
	}
	return list
}

func (c *Context) item(raw json.RawMessage) state.Item {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return state.Item{Item: s}
	}
	o, err := ParseObject(raw)
	if err != nil || !o.Has("item") {
		return state.Item{Item: strings.TrimSpace(string(raw))}
	}
	it := state.Item{Item: first(c.Descriptors(o, "item"))}
	it.Result = first(c.Descriptors(o, "result_item"))
	return it
}

// AgeableConfig reads minecraft:ageable. A feed item is a descriptor or
// an object with item, growth and result_item.
func (c *Context) AgeableConfig(j Object) state.AgeableConfig {
	a := state.DefaultAgeable()
	a.Duration = c.Float(j, "duration", a.Duration)
	if j.Has("feed_items") {
		for _, raw := range rawList(j["feed_items"]) {
			f := state.FeedItem{Item: c.item(raw), Growth: state.DefaultFeedGrowth()}
			if o, err := ParseObject(raw); err == nil {
				f.Growth = c.Float(o, "growth", f.Growth)
			}
			a.FeedItems = append(a.FeedItems, f)
		}
	}
	a.DropItems = c.Descriptors(j, "drop_items")
	a.GrowUp = c.FullTrigger(j, "grow_up")
	a.InteractFilters = c.filter(j, "interact_filters")
	a.TransformToItem = c.String(j, "transform_to_item", "")
	a.PauseGrowthItems = c.Descriptors(j, "pause_growth_items")
	a.ResetGrowthItems = c.Descriptors(j, "reset_growth_items")
	return a
}

// BreedableConfig reads minecraft:breedable. breeds_with is either the
// 1.26.60 object of mate type to breed event, or the older list (or one
// object) of mate_type, baby_type and breed_event.
func (c *Context) BreedableConfig(j Object) state.BreedableConfig {
	b := state.DefaultBreedable()
	b.RequireTame = c.Bool(j, alias(j, "require_tame", "requireTame"), b.RequireTame)
	b.AllowSitting = c.Bool(j, alias(j, "allow_sitting", "allowSitting"), b.AllowSitting)
	b.CausesPregnancy = c.Bool(j, "causes_pregnancy", b.CausesPregnancy)
	b.RequireFullHealth = c.Bool(j, "require_full_health", b.RequireFullHealth)
	b.ExtraBabyChance = c.Float(j, alias(j, "extra_baby_chance", "extraBabyChance"), b.ExtraBabyChance)
	b.Cooldown = c.Float(j, alias(j, "breed_cooldown", "breedCooldown"), b.Cooldown)
	// Older files keep the offspring members here.
	b.InheritTamed = c.Bool(j, "inherit_tamed", b.InheritTamed)
	b.BlendAttributes = c.Bool(j, "blend_attributes", b.BlendAttributes)
	b.BreedItems = c.Items(j, "breed_items")
	b.LoveFilters = c.filter(j, "love_filters")
	b.TransformToItem = c.String(j, "transform_to_item", "")
	for _, o := range objects(j["environment_requirements"]) {
		b.Environment = append(b.Environment, state.EnvironmentRequirement{
			Blocks: c.Descriptors(o, "blocks"), Count: c.Int(o, "count", 0), Radius: c.Float(o, "radius", 0),
		})
	}
	if !j.Has("breeds_with") {
		return b
	}
	var list []json.RawMessage
	if json.Unmarshal(j["breeds_with"], &list) == nil {
		for _, raw := range list {
			if o, err := ParseObject(raw); err == nil {
				b.BreedsWith = append(b.BreedsWith, c.legacyBreedType(o))
			}
		}
		return b
	}
	o, err := ParseObject(j["breeds_with"])
	if err != nil {
		c.Warn(fmt.Errorf("breeds_with: %w", err))
		return b
	}
	if o.Has("mate_type") {
		b.BreedsWith = append(b.BreedsWith, c.legacyBreedType(o))
		return b
	}
	for _, mate := range sortedKeys(o) {
		t := state.BreedType{Mate: mate}
		if string(o[mate]) != "{}" {
			t.Event = c.parser().ParseTrigger(o[mate], c.Warn)
		}
		b.BreedsWith = append(b.BreedsWith, t)
	}
	return b
}

func (c *Context) legacyBreedType(o Object) state.BreedType {
	return state.BreedType{
		Mate: c.String(o, "mate_type", ""), Baby: c.String(o, "baby_type", ""),
		Event: c.FullTrigger(o, "breed_event"),
	}
}

// Offspring reads minecraft:offspring into b: the baby of each mate
// (offspring_pairs), inherit_tamed and blend_attributes. The mutation,
// colour, property and parent-centric blending members are the engine's
// and are not read here.
func (c *Context) Offspring(j Object, b *state.BreedableConfig) {
	b.InheritTamed = c.Bool(j, "inherit_tamed", b.InheritTamed)
	b.BlendAttributes = c.Bool(j, "blend_attributes", b.BlendAttributes)
	if !j.Has("offspring_pairs") {
		return
	}
	pairs, err := ParseObject(j["offspring_pairs"])
	if err != nil {
		c.Warn(fmt.Errorf("offspring_pairs: %w", err))
		return
	}
	for _, mate := range sortedKeys(pairs) {
		baby := c.String(pairs, mate, "")
		found := false
		for i := range b.BreedsWith {
			if b.BreedsWith[i].Mate == mate {
				b.BreedsWith[i].Baby = baby
				found = true
			}
		}
		if !found {
			c.Warn(fmt.Errorf("offspring_pairs: %q is not in breeds_with", mate))
		}
	}
}

func sortedKeys(o Object) []string {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TameableConfig reads minecraft:tameable.
func (c *Context) TameableConfig(j Object) state.TameableConfig {
	t := state.DefaultTameable()
	t.Probability = c.Float(j, "probability", t.Probability)
	t.TameItems = c.Items(j, alias(j, "tame_items", "tameItems"))
	t.TameEvent = c.FullTrigger(j, "tame_event")
	return t
}

// TamemountConfig reads minecraft:tamemount.
func (c *Context) TamemountConfig(j Object) state.TamemountConfig {
	t := state.DefaultTamemount()
	t.MinTemper = c.Int(j, alias(j, "min_temper", "minTemper"), t.MinTemper)
	t.MaxTemper = c.Int(j, alias(j, "max_temper", "maxTemper"), t.MaxTemper)
	t.AttemptTemperMod = c.Int(j, alias(j, "attempt_temper_mod", "attemptTemperMod"), t.AttemptTemperMod)
	for _, raw := range rawList(j["feed_items"]) {
		f := state.TemperItem{Item: c.item(raw)}
		if o, err := ParseObject(raw); err == nil {
			f.TemperMod = c.Int(o, "temper_mod", 0)
		}
		t.FeedItems = append(t.FeedItems, f)
	}
	for _, raw := range rawList(j[alias(j, "auto_reject_items", "autoRejectItems")]) {
		t.AutoRejectItems = append(t.AutoRejectItems, c.item(raw))
	}
	t.FeedText = c.String(j, "feed_text", "")
	t.RideText = c.String(j, "ride_text", "")
	t.TameEvent = c.FullTrigger(j, "tame_event")
	return t
}

// AngryConfig reads minecraft:angry.
func (c *Context) AngryConfig(j Object) state.AngryConfig {
	a := state.DefaultAngry()
	a.Duration = c.Int(j, "duration", a.Duration)
	a.DurationDelta = c.Int(j, alias(j, "duration_delta", "durationDelta"), a.DurationDelta)
	a.BroadcastAnger = c.Bool(j, alias(j, "broadcast_anger", "broadcastAnger"), a.BroadcastAnger)
	a.OnAttack = c.Bool(j, "broadcast_anger_on_attack", a.OnAttack)
	a.OnBeingAttacked = c.Bool(j, "broadcast_anger_on_being_attacked", a.OnBeingAttacked)
	a.WhenDying = c.Bool(j, alias(j, "broadcast_anger_when_dying", "broadcastAngerWhenDying"), a.WhenDying)
	a.BroadcastRange = c.Int(j, alias(j, "broadcast_range", "broadcastRange"), a.BroadcastRange)
	a.BroadcastTargets = c.Descriptors(j, alias(j, "broadcast_targets", "broadcastTargets"))
	a.BroadcastFilters = c.filter(j, alias(j, "broadcast_filters", "broadcastFilters"))
	a.Filters = c.filter(j, "filters")
	a.CalmEvent = c.FullTrigger(j, "calm_event")
	a.Sound = c.Sound(j, "angry_sound", a.Sound)
	a.SoundInterval = c.Range(j, alias(j, "sound_interval", "soundInterval"), a.SoundInterval)
	return a
}

// HealableConfig reads minecraft:healable.
func (c *Context) HealableConfig(j Object) state.HealableConfig {
	h := state.DefaultHealable()
	h.ForceUse = c.Bool(j, "force_use", h.ForceUse)
	h.Filters = c.filter(j, "filters")
	for _, raw := range rawList(j["items"]) {
		o, err := ParseObject(raw)
		if err != nil {
			c.Warn(fmt.Errorf("items: %w", err))
			continue
		}
		it := state.HealItem{Item: c.item(raw), HealAmount: c.Int(o, "heal_amount", 0)}
		for _, e := range objects(o["effects"]) {
			eff := state.DefaultHealEffect()
			eff.Name = c.String(e, "name", "")
			eff.Duration = c.Int(e, "duration", eff.Duration)
			eff.Amplifier = c.Int(e, "amplifier", eff.Amplifier)
			eff.Chance = c.Float(e, "chance", eff.Chance)
			it.Effects = append(it.Effects, eff)
		}
		h.Items = append(h.Items, it)
	}
	return h
}

// DespawnConfig reads minecraft:despawn.
func (c *Context) DespawnConfig(j Object) state.DespawnConfig {
	d := state.DefaultDespawn()
	d.Filters = c.filter(j, "filters")
	d.RemoveChildEntities = c.Bool(j, "remove_child_entities", d.RemoveChildEntities)
	if j.Has("despawn_from_distance") {
		d.FromDistance = true
		if o, err := ParseObject(j["despawn_from_distance"]); err == nil {
			d.MinDistance = c.Float(o, "min_distance", d.MinDistance)
			d.MaxDistance = c.Float(o, "max_distance", d.MaxDistance)
		}
	}
	d.FromInactivity = c.Bool(j, "despawn_from_inactivity", d.FromInactivity)
	d.InactivityTimer = c.Int(j, "min_range_inactivity_timer", d.InactivityTimer)
	d.FromChance = c.Bool(j, "despawn_from_chance", d.FromChance)
	d.RandomChance = c.Int(j, "min_range_random_chance", d.RandomChance)
	d.FromSimulationEdge = c.Bool(j, "despawn_from_simulation_edge", d.FromSimulationEdge)
	return d
}
