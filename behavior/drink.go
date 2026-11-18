package behavior

// MilkDrinker is the part of the mob drink_milk needs on top of ItemUser.
type MilkDrinker interface {
	ItemUser
	// RemoveAllEffects clears every mob effect.
	RemoveAllEffects()
}

// DrinkMilkConfig is the JSON of minecraft:behavior.drink_milk. Cooldown is
// cooldown_seconds in ticks (BDS truncates the seconds times 20).
type DrinkMilkConfig struct {
	Cooldown int32
	// Filters is tested with the mob as self and no other entity.
	Filters Filter
}

// DrinkMilk is DrinkMilkGoal: the wandering trader drinks milk to clear its
// effects.
type DrinkMilk struct {
	base
	m    MilkDrinker
	cfg  DrinkMilkConfig
	next uint64
	end  uint64
}

// NewDrinkMilk makes the goal. Its BDS default cooldown is 0.
func NewDrinkMilk(w World, m MilkDrinker, c DrinkMilkConfig) *DrinkMilk {
	return &DrinkMilk{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// CanUse implements goal.Goal: once per cooldown, the filters.
func (g *DrinkMilk) CanUse() bool {
	now := g.world.CurrentTick()
	if now < g.next {
		return false
	}
	g.next = now + uint64(int64(g.cfg.Cooldown))
	return g.cfg.Filters == nil || g.cfg.Filters(g.m, nil)
}

// CanContinueToUse implements goal.Goal.
func (g *DrinkMilk) CanContinueToUse() bool { return g.m.Flag(FlagUsingItem) }

// Start implements goal.Goal.
func (g *DrinkMilk) Start() {
	startUsing(g.world, g.m, Item{Name: itemMilkBucket, Count: 1}, &g.end)
}

// Stop implements goal.Goal.
func (g *DrinkMilk) Stop() { stopUsing(g.m, &g.end) }

// Tick implements goal.Goal: once drunk, the effects go.
func (g *DrinkMilk) Tick() {
	if g.world.CurrentTick() < g.end {
		return
	}
	g.m.SetFlag(FlagUsingItem, false)
	if g.m.Slot(SlotMainhand).Name == itemMilkBucket {
		g.m.RemoveAllEffects()
		g.m.ActorEvent(actorEventDrinkMilk, 0)
	}
}

// PotionDrinker is the part of the mob drink_potion needs on top of
// ItemUser.
type PotionDrinker interface {
	ItemUser
	// DrinkPotion gives the mob the effect of the potion with the aux value
	// (Potion by ID, then).
	DrinkPotion(aux int16)
	// SpeedModifier adds the goal's modifier with the amount to the
	// movement attribute, or removes it.
	SpeedModifier(on bool, amount float32)
}

// Potion is one entry of drink_potion's potions list.
type Potion struct {
	// ID is the potion type, the aux value of the potion item.
	ID     int16
	Chance float32
	// Filters is tested with the mob as self and its target as the other.
	Filters Filter
}

// DrinkPotionConfig is the JSON of minecraft:behavior.drink_potion.
type DrinkPotionConfig struct {
	SpeedModifier float32 `json:"speed_modifier"`
	Potions       []Potion
}

// drinkPotionEvery is how many canUse calls drink_potion lets pass between
// two tries: it goes on when the call counter was at 5.
const drinkPotionEvery = 5

// DrinkPotion is DrinkPotionGoal: the witch drinks a potion that fits the
// moment.
type DrinkPotion struct {
	base
	m     PotionDrinker
	cfg   DrinkPotionConfig
	calls int32
	end   uint64
}

// NewDrinkPotion makes the goal. Its BDS defaults are all zero.
func NewDrinkPotion(w World, m PotionDrinker, c DrinkPotionConfig) *DrinkPotion {
	return &DrinkPotion{base: base{mob: m, world: w, flags: flagsBit3}, m: m, cfg: c}
}

// CanUse implements goal.Goal: every sixth call, the first
// potion whose chance and filters pass is drunk.
func (g *DrinkPotion) CanUse() bool {
	c := g.calls
	g.calls++
	if c < drinkPotionEvery {
		return false
	}
	g.calls = 0
	r := g.world.Random()
	target := g.actor(g.m.Target())
	for _, p := range g.cfg.Potions {
		if r.NextFloat() >= p.Chance || p.Filters != nil && !p.Filters(g.m, target) {
			continue
		}
		if !startUsing(g.world, g.m, Item{Name: itemPotion, Aux: p.ID, Count: 1}, &g.end) {
			return false
		}
		g.m.SpeedModifier(false, g.cfg.SpeedModifier)
		g.m.SpeedModifier(true, g.cfg.SpeedModifier)
		return true
	}
	return false
}

// CanContinueToUse implements goal.Goal.
func (g *DrinkPotion) CanContinueToUse() bool { return g.m.Flag(FlagUsingItem) }

// Start implements goal.Goal.
func (g *DrinkPotion) Start() {}

// Stop implements goal.Goal.
func (g *DrinkPotion) Stop() {
	stopUsing(g.m, &g.end)
	g.m.SpeedModifier(false, g.cfg.SpeedModifier)
}

// Tick implements goal.Goal: once drunk, the potion works.
func (g *DrinkPotion) Tick() {
	if g.world.CurrentTick() < g.end {
		return
	}
	g.m.SetFlag(FlagUsingItem, false)
	if h := g.m.Slot(SlotMainhand); h.Name == itemPotion {
		g.m.DrinkPotion(h.Aux)
		g.m.ActorEvent(actorEventDrinkPotion, 0)
	}
}
