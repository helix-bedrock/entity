package component

import (
	"encoding/json"
	"fmt"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/state"
)

// The names of the remaining state components.
const (
	TypeFamilyName                = "minecraft:type_family"
	SittableName                  = "minecraft:sittable"
	DamageOverTimeName            = "minecraft:damage_over_time"
	DryingOutTimerName            = "minecraft:drying_out_timer"
	InstantDespawnName            = "minecraft:instant_despawn"
	PeekName                      = "minecraft:peek"
	AmbientSoundIntervalName      = "minecraft:ambient_sound_interval"
	HeartbeatName                 = "minecraft:heartbeat"
	BribeableName                 = "minecraft:bribeable"
	CelebrateHuntName             = "minecraft:celebrate_hunt"
	ExplodeName                   = "minecraft:explode"
	TeleportName                  = "minecraft:teleport"
	TrailName                     = "minecraft:trail"
	GroupSizeName                 = "minecraft:group_size"
	HomeName                      = "minecraft:home"
	InsideBlockNotifierName       = "minecraft:inside_block_notifier"
	AreaAttackName                = "minecraft:area_attack"
	MobEffectName                 = "minecraft:mob_effect"
	SpawnEntityName               = "minecraft:spawn_entity"
	BlockSensorName               = "minecraft:block_sensor"
	CombatRegenerationName        = "minecraft:combat_regeneration"
	TrustingName                  = "minecraft:trusting"
	GeneticsName                  = "minecraft:genetics"
	AngerLevelName                = "minecraft:anger_level"
	RavagerBlockedName            = "minecraft:ravager_blocked"
	SpellEffectsName              = "minecraft:spell_effects"
	PreferredPathName             = "minecraft:preferred_path"
	BreakDoorName                 = "minecraft:annotation.break_door"
	ShareablesName                = "minecraft:shareables"
	LootName                      = "minecraft:loot"
	InventoryName                 = "minecraft:inventory"
	TradeTableName                = "minecraft:trade_table"
	EconomyTradeTableName         = "minecraft:economy_trade_table"
	BossName                      = "minecraft:boss"
	ShooterName                   = "minecraft:shooter"
	BalloonableName               = "minecraft:balloonable"
	BoostableName                 = "minecraft:boostable"
	ItemControllableName          = "minecraft:item_controllable"
	DyeableName                   = "minecraft:is_dyeable"
	BreakBlocksName               = "minecraft:break_blocks"
	DashActionName                = "minecraft:dash_action"
	VerticalMovementActionName    = "minecraft:vertical_movement_action"
	FreeCameraControlledName      = "minecraft:free_camera_controlled"
	ReflectProjectilesName        = "minecraft:reflect_projectiles"
	KnockbackRulesName            = "minecraft:apply_knockback_rules"
	RaidTriggerName               = "minecraft:raid_trigger"
	MobEffectImmunityName         = "minecraft:mob_effect_immunity"
	InsomniaName                  = "minecraft:insomnia"
	GrowsCropName                 = "minecraft:grows_crop"
	DwellerName                   = "minecraft:dweller"
	FlockingName                  = "minecraft:flocking"
	GameEventMovementTrackingName = "minecraft:game_event_movement_tracking"
	LookedAtName                  = "minecraft:looked_at"
)

// readOthers reads the components of state's markers.go, timers.go,
// world.go, social.go and engine.go into s. The running ones keep their
// state from prev while their JSON is unchanged.
func (c *Context) readOthers(env *state.Env, s, prev *state.Set, comps definition.Group, read func(string) (Object, bool), same func(...string) bool) {
	var r behavior.Random
	if env.World != nil {
		r = env.World.Random()
	}
	for name, key := range state.IntDataComponents() {
		if j, ok := read(name); ok {
			s.IntData = append(s.IntData, state.IntDataConfig{Key: key, Value: c.Int(j, "value", 0)})
		}
	}
	if j, ok := read(TypeFamilyName); ok {
		s.TypeFamily = &state.TypeFamilyConfig{Families: c.Descriptors(j, "family")}
	}
	for _, name := range state.MarkerComponents() {
		if _, ok := comps[name]; ok {
			if s.Markers == nil {
				s.Markers = map[string]bool{}
			}
			s.Markers[name] = true
		}
	}
	if j, ok := read(SittableName); ok {
		s.Sittable = &state.SittableConfig{SitEvent: c.Trigger(j, "sit_event"), StandEvent: c.Trigger(j, "stand_event")}
	}
	c.readTimers(env, s, prev, read, same, r)
	c.readWorld(s, prev, read, same)
	c.readSocial(s, prev, read, same, r)
	c.readEngine(s, read)
	for _, name := range state.Passthrough() {
		if raw, ok := comps[name]; ok {
			if s.Raw == nil {
				s.Raw = state.RawComponents{}
			}
			s.Raw[name] = raw
		}
	}
}

func (c *Context) readTimers(env *state.Env, s, prev *state.Set, read func(string) (Object, bool), same func(...string) bool, r behavior.Random) {
	if j, ok := read(DamageOverTimeName); ok {
		if s.DamageOverTime = prev.DamageOverTime; s.DamageOverTime == nil || !same(DamageOverTimeName) {
			s.DamageOverTime = state.NewDamageOverTime(c.DamageOverTimeConfig(j))
		}
	}
	if j, ok := read(DryingOutTimerName); ok {
		if s.DryingOutTimer = prev.DryingOutTimer; s.DryingOutTimer == nil || !same(DryingOutTimerName) {
			s.DryingOutTimer = state.NewDryingOutTimer(state.DryingOutTimerConfig{
				TotalTime:             c.Ticks(j, "total_time", 0),
				WaterBottleRefillTime: c.Ticks(j, "water_bottle_refill_time", 0),
				DriedOut:              c.FullTrigger(j, "dried_out_event"),
				StoppedDryingOut:      c.FullTrigger(j, "stopped_drying_out_event"),
				RecoverAfterDriedOut:  c.FullTrigger(j, "recover_after_dried_out_event"),
			})
		}
	}
	if j, ok := read(InstantDespawnName); ok {
		s.InstantDespawn = &state.InstantDespawnConfig{RemoveChildEntities: c.Bool(j, "remove_child_entities", false)}
	}
	if j, ok := read(PeekName); ok {
		if s.Peek = prev.Peek; s.Peek == nil || !same(PeekName) {
			p := state.DefaultPeek()
			p.OnOpen, p.OnClose, p.OnTargetOpen = c.FullTrigger(j, "on_open"), c.FullTrigger(j, "on_close"), c.FullTrigger(j, "on_target_open")
			s.Peek = state.NewPeek(p)
		}
	}
	if j, ok := read(AmbientSoundIntervalName); ok {
		if s.AmbientSound = prev.AmbientSound; s.AmbientSound == nil || !same(AmbientSoundIntervalName) {
			s.AmbientSound = state.NewAmbientSoundInterval(c.AmbientSoundIntervalConfig(j), env.Self)
		}
	}
	if j, ok := read(HeartbeatName); ok {
		def := 0
		if c.Resolver != nil {
			def = c.Resolver.Sound("heartbeat")
		}
		h := state.HeartbeatConfig{Interval: c.molang(j, "interval"), Sound: c.Sound(j, "sound_event", def)}
		s.Heartbeat = &h
	}
	if j, ok := read(BribeableName); ok {
		if s.Bribeable = prev.Bribeable; s.Bribeable == nil || !same(BribeableName) {
			b := state.DefaultBribeable()
			b.Cooldown = c.Ticks(j, "bribe_cooldown", b.Cooldown)
			b.Items = c.Descriptors(j, "bribe_items")
			s.Bribeable = state.NewBribeable(b)
		}
	}
	if j, ok := read(CelebrateHuntName); ok {
		if s.CelebrateHunt = prev.CelebrateHunt; s.CelebrateHunt == nil || !same(CelebrateHuntName) {
			s.CelebrateHunt = state.NewCelebrateHunt(state.CelebrateHuntConfig{
				Broadcast:     c.Bool(j, "broadcast", false),
				Radius:        c.Float(j, "radius", 0),
				Duration:      c.Ticks(j, "duration", 0),
				Targets:       c.filter(j, "celebration_targets"),
				Sound:         c.Sound(j, "celebrate_sound", 0),
				SoundInterval: c.Range(j, "sound_interval", behavior.Range[float32]{}),
			})
		}
	}
	if j, ok := read(ExplodeName); ok {
		if s.Explode = prev.Explode; s.Explode == nil || !same(ExplodeName) {
			s.Explode = state.NewExplode(c.ExplodeConfig(j), r)
		}
	}
}

// DamageOverTimeConfig reads minecraft:damage_over_time.
func (c *Context) DamageOverTimeConfig(j Object) state.DamageOverTimeConfig {
	d := state.DefaultDamageOverTime()
	d.DamagePerHurt = c.Int(j, alias(j, "damage_per_hurt", "damagePerHurt"), d.DamagePerHurt)
	d.TimeBetweenHurt = c.Ticks(j, alias(j, "time_between_hurt", "timeBetweenHurt"), d.TimeBetweenHurt)
	return d
}

// AmbientSoundIntervalConfig reads minecraft:ambient_sound_interval.
func (c *Context) AmbientSoundIntervalConfig(j Object) state.AmbientSoundIntervalConfig {
	a := state.DefaultAmbientSoundInterval()
	a.Value = c.Float(j, "value", a.Value)
	a.Range = c.Float(j, "range", a.Range)
	a.Event = c.String(j, "event_name", a.Event)
	for _, o := range objects(j["event_names"]) {
		a.Events = append(a.Events, state.AmbientSound{Condition: c.molang(o, "condition"), Event: c.String(o, "event_name", "")})
	}
	return a
}

// ExplodeConfig reads minecraft:explode; several members have an old
// camelCase name too.
func (c *Context) ExplodeConfig(j Object) state.ExplodeConfig {
	e := state.DefaultExplode()
	e.FuseLength = c.RangeTicks(j, alias(j, "fuse_length", "fuseLength"), e.FuseLength)
	e.FuseLit = c.Bool(j, alias(j, "fuse_lit", "fuseLit"), e.FuseLit)
	e.Power = c.Float(j, "power", e.Power)
	e.MaxResistance = c.Float(j, alias(j, "max_resistance", "maxResistance"), e.MaxResistance)
	e.CausesFire = c.Bool(j, alias(j, "causes_fire", "causesFire"), e.CausesFire)
	e.BreaksBlocks = c.Bool(j, "breaks_blocks", e.BreaksBlocks)
	e.FireAffectedByGriefing = c.Bool(j, alias(j, "fire_affected_by_griefing", "fireAffectedByGriefing"), e.FireAffectedByGriefing)
	e.DestroyByGriefing = c.Bool(j, alias(j, "destroy_affected_by_griefing", "destroyAffectedByGriefing"), e.DestroyByGriefing)
	e.AllowUnderwater = c.Bool(j, "allow_underwater", e.AllowUnderwater)
	e.DamageScaling = c.Float(j, "damage_scaling", e.DamageScaling)
	e.KnockbackScaling = c.Float(j, "knockback_scaling", e.KnockbackScaling)
	e.NegatesFallDamage = c.Bool(j, "negates_fall_damage", e.NegatesFallDamage)
	e.TogglesBlocks = c.Bool(j, "toggles_blocks", e.TogglesBlocks)
	e.ParticleEffect = c.String(j, "particle_effect", e.ParticleEffect)
	e.SoundEffect = c.String(j, "sound_effect", e.SoundEffect)
	return e
}

func (c *Context) readWorld(s, prev *state.Set, read func(string) (Object, bool), same func(...string) bool) {
	if j, ok := read(TeleportName); ok {
		if s.Teleport = prev.Teleport; s.Teleport == nil || !same(TeleportName) {
			s.Teleport = state.NewTeleport(c.TeleportConfig(j))
		}
	}
	if j, ok := read(TrailName); ok {
		t := state.DefaultTrail()
		t.Block = c.String(j, "block_type", t.Block)
		t.Filter = c.filter(j, "spawn_filter")
		t.Offset = state.ClampOffset(c.Vec3(j, "spawn_offset", t.Offset))
		s.Trail = &t
	}
	if j, ok := read(GroupSizeName); ok {
		if s.GroupSize = prev.GroupSize; s.GroupSize == nil || !same(GroupSizeName) {
			g := state.DefaultGroupSize()
			g.Radius = c.Float(j, "radius", g.Radius)
			g.Filter = c.filter(j, "filters")
			s.GroupSize = state.NewGroupSize(g)
		}
	}
	if j, ok := read(HomeName); ok {
		h := state.DefaultHome()
		h.RestrictionRadius = c.Int(j, "restriction_radius", h.RestrictionRadius)
		h.RestrictionType = c.String(j, "restriction_type", h.RestrictionType)
		h.Blocks = c.Descriptors(j, "home_block_list")
		if s.Home = prev.Home; s.Home == nil {
			s.Home = state.NewHome(h)
		} else {
			s.Home.SetConfig(h)
		}
	}
	if j, ok := read(InsideBlockNotifierName); ok {
		if s.InsideBlockNotifier = prev.InsideBlockNotifier; s.InsideBlockNotifier == nil || !same(InsideBlockNotifierName) {
			var n state.InsideBlockNotifierConfig
			for _, o := range objects(j["block_list"]) {
				n.Blocks = append(n.Blocks, state.InsideBlock{
					Block:   c.Descriptors(o, "block"),
					Entered: c.FullTrigger(o, "entered_block_event"),
					Exited:  c.FullTrigger(o, "exited_block_event"),
				})
			}
			s.InsideBlockNotifier = state.NewInsideBlockNotifier(n)
		}
	}
	if j, ok := read(AreaAttackName); ok {
		if s.AreaAttack = prev.AreaAttack; s.AreaAttack == nil || !same(AreaAttackName) {
			s.AreaAttack = state.NewAreaAttack(c.AreaAttackConfig(j))
		}
	}
	if j, ok := read(MobEffectName); ok {
		if s.MobEffect = prev.MobEffect; s.MobEffect == nil || !same(MobEffectName) {
			s.MobEffect = state.NewMobEffect(state.MobEffectConfig{
				Range:    c.Float(j, "effect_range", 0),
				Effect:   c.String(j, "mob_effect", ""),
				Duration: c.Ticks(j, "effect_time", 0),
				Cooldown: c.Ticks(j, "cooldown_time", 0),
				Ambient:  c.Bool(j, "ambient", false),
				Filter:   c.filter(j, "entity_filter"),
			})
		}
	}
	if j, ok := read(SpawnEntityName); ok {
		if s.SpawnEntity = prev.SpawnEntity; s.SpawnEntity == nil || !same(SpawnEntityName) {
			s.SpawnEntity = state.NewSpawnEntity(c.SpawnEntityConfig(j))
		}
	}
	if j, ok := read(BlockSensorName); ok {
		b := state.BlockSensorConfig{Radius: c.Float(j, "sensor_radius", 0), Sources: c.filter(j, "sources")}
		for _, o := range objects(j["on_break"]) {
			b.OnBreak = append(b.OnBreak, state.BlockBreakEntry{Blocks: c.Descriptors(o, "block_list"), Event: c.Trigger(o, "on_block_broken")})
		}
		s.BlockSensor = &b
	}
	if j, ok := read(CombatRegenerationName); ok {
		if s.CombatRegeneration = prev.CombatRegeneration; s.CombatRegeneration == nil || !same(CombatRegenerationName) {
			r := state.DefaultCombatRegeneration()
			r.Duration = c.Ticks(j, "regeneration_duration", r.Duration)
			r.ApplyToSelf = c.Bool(j, "apply_to_self", r.ApplyToSelf)
			r.ApplyToFamily = c.Bool(j, "apply_to_family", r.ApplyToFamily)
			s.CombatRegeneration = state.NewCombatRegeneration(r)
		}
	}
	if j, ok := read(PreferredPathName); ok {
		p := state.DefaultPreferredPath()
		p.DefaultBlockCost = c.Float(j, "default_block_cost", p.DefaultBlockCost)
		p.JumpCost = c.Float(j, "jump_cost", p.JumpCost)
		p.MaxFallBlocks = c.Int(j, "max_fall_blocks", p.MaxFallBlocks)
		for _, o := range objects(j["preferred_path_blocks"]) {
			p.Costs = append(p.Costs, state.PathCost{Cost: c.Float(o, "cost", 0), Blocks: c.Descriptors(o, "blocks")})
		}
		s.PreferredPath = &p
	}
	if j, ok := read(BreakDoorName); ok {
		s.BreakDoor = &state.BreakDoorConfig{BreakTime: c.Ticks(j, "break_time", 0), MinDifficulty: c.difficulty(j, "min_difficulty")}
	}
}

// difficulties are the difficulty names by number.
func difficulties() []string { return []string{"peaceful", "easy", "normal", "hard"} }

func (c *Context) difficulty(j Object, k string) int32 {
	if !j.Has(k) {
		return 0
	}
	return int32(c.enum(j, k, difficulties(), 0))
}

// TeleportConfig reads minecraft:teleport.
func (c *Context) TeleportConfig(j Object) state.TeleportConfig {
	t := state.DefaultTeleport()
	t.RandomTeleports = c.Bool(j, alias(j, "random_teleports", "randomTeleports"), t.RandomTeleports)
	t.MinRandomTime = c.Ticks(j, alias(j, "min_random_teleport_time", "minRandomTeleportTime"), t.MinRandomTime)
	t.MaxRandomTime = c.Ticks(j, alias(j, "max_random_teleport_time", "maxRandomTeleportTime"), t.MaxRandomTime)
	t.Cube = c.Vec3(j, alias(j, "random_teleport_cube", "randomTeleportCube"), t.Cube)
	t.TargetDistance = c.Float(j, alias(j, "target_distance", "targetDistance"), t.TargetDistance)
	t.TargetChance = c.Float(j, alias(j, "target_teleport_chance", "target_tp_chance"), t.TargetChance)
	t.LightChance = c.Float(j, alias(j, "light_teleport_chance", "lightTeleportChance"), t.LightChance)
	t.DarkChance = c.Float(j, alias(j, "dark_teleport_chance", "darkTeleportChance"), t.DarkChance)
	t.TeleportsOnProjectileHit = c.Bool(j, "teleports_on_projectile_hit", t.TeleportsOnProjectileHit)
	return t
}

// AreaAttackConfig reads minecraft:area_attack.
func (c *Context) AreaAttackConfig(j Object) state.AreaAttackConfig {
	a := state.DefaultAreaAttack()
	a.DamageRange = c.Float(j, "damage_range", a.DamageRange)
	a.DamagePerTick = c.Int(j, "damage_per_tick", a.DamagePerTick)
	if name := c.String(j, "cause", ""); name != "" && c.Resolver != nil {
		if cause, ok := c.Resolver.DamageCause(name); ok {
			a.Cause = cause
		} else {
			c.Warn(fmt.Errorf("cause: unknown damage cause %q", name))
		}
	}
	a.Cooldown = c.Ticks(j, "damage_cooldown", a.Cooldown)
	a.Filter = c.filter(j, "entity_filter")
	a.PlaySound = c.Bool(j, "play_attack_sound", a.PlaySound)
	a.SelfAsSource = c.Bool(j, "use_self_as_damage_source", a.SelfAsSource)
	a.DeathMessageOverride = c.String(j, "death_message_override", a.DeathMessageOverride)
	return a
}

// SpawnEntityConfig reads minecraft:spawn_entity: one entry or a list,
// under entities or (the old form) the component itself.
func (c *Context) SpawnEntityConfig(j Object) state.SpawnEntityConfig {
	list := []Object{j}
	if j.Has("entities") {
		list = objects(j["entities"])
	}
	var s state.SpawnEntityConfig
	for _, o := range list {
		e := state.DefaultSpawnEntry()
		e.MinWait = c.Ticks(o, "min_wait_time", e.MinWait)
		e.MaxWait = c.Ticks(o, "max_wait_time", e.MaxWait)
		e.Sound = c.String(o, "spawn_sound", e.Sound)
		if o.Has("spawn_item") {
			e.Item = first(c.Descriptors(o, "spawn_item"))
		}
		e.ItemEvent = c.FullTrigger(o, "spawn_item_event")
		e.Entity = c.String(o, "spawn_entity", e.Entity)
		e.Event = c.String(o, "spawn_event", e.Event)
		e.Method = c.String(o, "spawn_method", e.Method)
		e.SingleUse = c.Bool(o, "single_use", e.SingleUse)
		e.ShouldLeash = c.Bool(o, "should_leash", e.ShouldLeash)
		e.Count = c.Int(o, "num_to_spawn", e.Count)
		e.Filters = c.filter(o, "filters")
		s.Entries = append(s.Entries, e)
	}
	return s
}

func (c *Context) readSocial(s, prev *state.Set, read func(string) (Object, bool), same func(...string) bool, r behavior.Random) {
	if j, ok := read(TrustingName); ok {
		t := state.DefaultTrusting()
		t.Probability = c.Float(j, "probability", t.Probability)
		t.Items = c.Descriptors(j, "trust_items")
		t.OnTrust = c.FullTrigger(j, "trust_event")
		s.Trusting = &t
	}
	if j, ok := read(GeneticsName); ok {
		if s.Genetics = prev.Genetics; s.Genetics == nil || !same(GeneticsName) {
			s.Genetics = state.NewGenetics(c.GeneticsConfig(j), r)
		}
	}
	if j, ok := read(AngerLevelName); ok {
		if s.AngerLevel = prev.AngerLevel; s.AngerLevel == nil || !same(AngerLevelName) {
			s.AngerLevel = state.NewAngerLevel(c.AngerLevelConfig(j))
		}
	}
	if j, ok := read(RavagerBlockedName); ok {
		b := state.RavagerBlockedConfig{KnockbackStrength: c.Float(j, "knockback_strength", 0)}
		for _, o := range objects(j["reaction_choices"]) {
			b.Reactions = append(b.Reactions, state.WeightedTrigger{Weight: c.Int(o, "weight", 0), Trigger: c.FullTrigger(o, "value")})
		}
		s.RavagerBlocked = &b
	}
	if j, ok := read(SpellEffectsName); ok {
		if s.SpellEffects = prev.SpellEffects; s.SpellEffects == nil || !same(SpellEffectsName) {
			var e state.SpellEffectsConfig
			for _, o := range objects(j["add_effects"]) {
				e.Add = append(e.Add, state.SpellEffect{
					Effect: c.String(o, "effect", ""), Duration: c.Ticks(o, "duration", 0), Amplifier: c.Int(o, "amplifier", 0),
					Ambient: c.Bool(o, "ambient", false), Visible: c.Bool(o, "visible", true),
					ScreenAnimation: c.Bool(o, "display_on_screen_animation", false),
				})
			}
			e.Remove = c.Descriptors(j, "remove_effects")
			s.SpellEffects = state.NewSpellEffects(e)
		}
	}
	if j, ok := read(ShareablesName); ok {
		sh := behavior.DefaultShareables()
		c.Into(j, &sh)
		sh.Items = nil
		for _, o := range objects(j["items"]) {
			it := behavior.DefaultShareableItem()
			c.Into(o, &it)
			sh.Items = append(sh.Items, it)
		}
		s.Shareables = &sh
	}
}

// GeneticsConfig reads minecraft:genetics. An allele is a number or a
// range_min/range_max object.
func (c *Context) GeneticsConfig(j Object) state.GeneticsConfig {
	g := state.GeneticsConfig{MutationRate: c.Float(j, "mutation_rate", 0)}
	unset := state.AlleleRange{Min: -1, Max: -1}
	for _, o := range objects(j["genes"]) {
		gene := state.Gene{
			Name:         c.String(o, "name", ""),
			Alleles:      c.allele(o, "allele_range", state.AlleleRange{}),
			Simplified:   c.Bool(o, "use_simplified_breeding", false),
			MutationRate: c.Float(o, "mutation_rate", -1),
		}
		for _, v := range objects(o["genetic_variants"]) {
			gene.Variants = append(gene.Variants, state.GeneticVariant{
				Main: c.allele(v, "main_allele", unset), Hidden: c.allele(v, "hidden_allele", unset),
				Either: c.allele(v, "either_allele", unset), Both: c.allele(v, "both_allele", unset),
				Birth: c.FullTrigger(v, "birth_event"),
			})
		}
		g.Genes = append(g.Genes, gene)
	}
	return g
}

func (c *Context) allele(j Object, k string, def state.AlleleRange) state.AlleleRange {
	if !j.Has(k) {
		return def
	}
	r := c.RangeInt(j, k, behavior.Range[int32]{Min: def.Min, Max: def.Max})
	return state.AlleleRange{Min: r.Min, Max: r.Max}
}

// AngerLevelConfig reads minecraft:anger_level.
func (c *Context) AngerLevelConfig(j Object) state.AngerLevelConfig {
	a := state.AngerLevelConfig{
		AngryThreshold:              c.Int(j, "angry_threshold", 0),
		AngryBoost:                  c.Int(j, "angry_boost", 0),
		MaxAnger:                    c.Int(j, "max_anger", 0),
		DecrementInterval:           c.Ticks(j, "anger_decrement_interval", 0),
		DefaultAnnoyingness:         c.Int(j, "default_annoyingness", 0),
		ProjectileAnnoyance:         c.Int(j, "default_projectile_annoyingness", 0),
		RemoveTargetsBelowThreshold: c.Bool(j, "remove_targets_below_angry_threshold", false),
		Nuisance:                    c.filter(j, "nuisance_filter"),
	}
	for _, o := range objects(j["on_increase_sounds"]) {
		a.Sounds = append(a.Sounds, state.AngerSound{Condition: c.molang(o, "condition"), Sound: c.Sound(o, "sound", 0)})
	}
	return a
}

// molang compiles a Molang member, nil when missing or not compilable.
func (c *Context) molang(j Object, k string) func(behavior.Mob) float32 {
	if !j.Has(k) || c.Resolver == nil {
		return nil
	}
	var src string
	if json.Unmarshal(j[k], &src) != nil {
		src = string(j[k])
	}
	eval, ok := c.Resolver.Molang(src)
	if !ok {
		c.Warn(fmt.Errorf("%s: not Molang: %q", k, src))
		return nil
	}
	return eval
}

func (c *Context) readEngine(s *state.Set, read func(string) (Object, bool)) {
	if j, ok := read(LootName); ok {
		var l state.LootConfig
		c.Into(j, &l)
		s.Loot = &l
	}
	if j, ok := read(InventoryName); ok {
		var i state.InventoryConfig
		c.Into(j, &i)
		s.Inventory = &i
	}
	if j, ok := read(TradeTableName); ok {
		var t state.TradeTableConfig
		c.Into(j, &t)
		s.TradeTable = &t
	}
	if j, ok := read(EconomyTradeTableName); ok {
		var t state.EconomyTradeTableConfig
		c.Into(j, &t)
		t.CuredDiscount = c.RangeInt(j, "cured_discount", t.CuredDiscount)
		t.MaxCuredDiscount = c.RangeInt(j, "max_cured_discount", t.MaxCuredDiscount)
		s.EconomyTradeTable = &t
	}
	if j, ok := read(BossName); ok {
		var b state.BossConfig
		c.Into(j, &b)
		s.Boss = &b
	}
	if j, ok := read(ShooterName); ok {
		sh := state.ShooterConfig{
			Def: c.String(j, "def", ""), AuxVal: c.Int(j, alias(j, "aux_val", "auxVal"), -1),
			Power: c.Float(j, "power", 0), Magic: c.Bool(j, "magic", false), Sound: c.Sound(j, "sound", 0),
		}
		for _, o := range objects(j["projectiles"]) {
			sh.Projectiles = append(sh.Projectiles, state.ShooterProjectile{
				Def: c.String(o, "def", ""), AuxVal: c.Int(o, alias(o, "aux_val", "auxVal"), -1),
				Filters: c.filter(o, "filters"), LoseTarget: c.Bool(o, "lose_target", false),
			})
		}
		s.Shooter = &sh
	}
	if j, ok := read(BalloonableName); ok {
		b := state.DefaultBalloonable()
		b.SoftDistance = c.Float(j, "soft_distance", b.SoftDistance)
		b.MaxDistance = c.Float(j, "max_distance", b.MaxDistance)
		b.Mass = c.Float(j, "mass", b.Mass)
		b.OnBalloon, b.OnUnballoon = c.FullTrigger(j, "on_balloon"), c.FullTrigger(j, "on_unballoon")
		s.Balloonable = &b
	}
	if j, ok := read(BoostableName); ok {
		b := state.BoostableConfig{Duration: c.Ticks(j, "duration", 0), SpeedMultiplier: c.Float(j, "speed_multiplier", 0)}
		for _, o := range objects(j["boost_items"]) {
			var it state.BoostItem
			c.Into(o, &it)
			b.Items = append(b.Items, it)
		}
		s.Boostable = &b
	}
	if j, ok := read(ItemControllableName); ok {
		s.ItemControllable = &state.ItemControllableConfig{ControlItems: c.Descriptors(j, "control_items")}
	}
	if j, ok := read(DyeableName); ok {
		var d state.DyeableConfig
		c.Into(j, &d)
		s.Dyeable = &d
	}
	if j, ok := read(BreakBlocksName); ok {
		s.BreakBlocks = &state.BreakBlocksConfig{Blocks: c.Descriptors(j, "breakable_blocks")}
	}
	if j, ok := read(DashActionName); ok {
		s.DashAction = &state.DashActionConfig{
			Cooldown: c.Ticks(j, "cooldown_time", 0), HorizontalMomentum: c.Float(j, "horizontal_momentum", 0),
			VerticalMomentum: c.Float(j, "vertical_momentum", 0), CanDashUnderwater: c.Bool(j, "can_dash_underwater", false),
			Direction: c.String(j, "direction", ""),
		}
	}
	if j, ok := read(VerticalMovementActionName); ok {
		var v state.VerticalMovementActionConfig
		c.Into(j, &v)
		s.VerticalMovementAction = &v
	}
	if j, ok := read(FreeCameraControlledName); ok {
		var f state.FreeCameraControlledConfig
		c.Into(j, &f)
		s.FreeCameraControlled = &f
	}
	if j, ok := read(ReflectProjectilesName); ok {
		s.ReflectProjectiles = &state.ReflectProjectilesConfig{
			AzimuthAngle: c.text(j, "azimuth_angle"), ElevationAngle: c.text(j, "elevation_angle"),
			ReflectionScale: c.text(j, "reflection_scale"), Projectiles: c.Descriptors(j, "reflected_projectiles"),
			Sound: c.Sound(j, "reflection_sound", 0),
		}
	}
	if j, ok := read(KnockbackRulesName); ok {
		var k state.KnockbackRulesConfig
		for _, o := range objects(j["presets"]) {
			k.Presets = append(k.Presets, state.KnockbackPreset{
				Filter: c.filter(o, "filter"), HorizontalPower: c.Float(o, "horizontal_power", 0),
				VerticalPower: c.Float(o, "vertical_power", 0), VerticalVelocityCap: c.Float(o, "vertical_velocity_cap", 0),
				SlowdownScale: c.Float(o, "slowdown_scale", 0), ScaleWithDamage: c.Bool(o, "scale_with_damage", false),
				Mode: c.String(o, "knockback_mode", ""), ExtraApproach: c.String(o, "extra_knockback_approach", ""),
			})
		}
		s.KnockbackRules = &k
	}
	if j, ok := read(RaidTriggerName); ok {
		s.RaidTrigger = &state.RaidTriggerConfig{Triggered: c.FullTrigger(j, "triggered_event")}
	}
	if j, ok := read(MobEffectImmunityName); ok {
		s.MobEffectImmunity = &state.MobEffectImmunityConfig{Effects: c.Descriptors(j, "mob_effects")}
	}
	if j, ok := read(InsomniaName); ok {
		in := state.DefaultInsomnia()
		c.Into(j, &in)
		s.Insomnia = &in
	}
	if j, ok := read(GrowsCropName); ok {
		var g state.GrowsCropConfig
		c.Into(j, &g)
		s.GrowsCrop = &g
	}
	if j, ok := read(DwellerName); ok {
		var d state.DwellerConfig
		c.Into(j, &d)
		s.Dweller = &d
	}
	if j, ok := read(FlockingName); ok {
		var f state.FlockingConfig
		c.Into(j, &f)
		s.Flocking = &f
	}
	if j, ok := read(GameEventMovementTrackingName); ok {
		g := state.DefaultGameEventMovementTracking()
		c.Into(j, &g)
		s.GameEventMovementTracking = &g
	}
	if j, ok := read(LookedAtName); ok {
		l := state.LookedAtConfig{
			SearchRadius: c.Float(j, "search_radius", 0), SetTarget: c.String(j, "set_target", ""),
			FindPlayersOnly: c.Bool(j, "find_players_only", false), Cooldown: c.Range(j, "looked_at_cooldown", behavior.Range[float32]{}),
			FieldOfView: c.Float(j, "field_of_view", 0), ScaleFOVByDistance: c.Bool(j, "scale_fov_by_distance", false),
			LineOfSightObstruction: c.String(j, "line_of_sight_obstruction_type", ""),
			MinLookedAtDuration:    c.Float(j, "min_looked_at_duration", 0), Filters: c.filter(j, "filters"),
			LookedAt: c.FullTrigger(j, "looked_at_event"), NotLookedAt: c.FullTrigger(j, "not_looked_at_event"),
		}
		for _, o := range objects(j["look_at_locations"]) {
			l.LookAtLocations = append(l.LookAtLocations, string(o.Raw()))
		}
		s.LookedAt = &l
	}
}

// text reads a member that is a string or a number as its text.
func (c *Context) text(j Object, k string) string {
	if !j.Has(k) {
		return ""
	}
	var s string
	if json.Unmarshal(j[k], &s) == nil {
		return s
	}
	return string(j[k])
}

// othersChanged writes the data and flags of the remaining components as
// they come and go, as their initialize and uninitialize do.
func othersChanged(m state.Mob, s, prev *state.Set, prevComps, comps definition.Group) {
	for name, key := range state.IntDataComponents() {
		old, had := prevComps[name]
		now, has := comps[name]
		if has && (!had || string(old) != string(now)) {
			for _, d := range s.IntData {
				if d.Key == key {
					d.Apply(m)
				}
			}
		}
	}
	state.SetFlags(m, state.MarkerFlagComponents(), hasIn(prevComps), hasIn(comps))
	if prev.Sittable != nil && s.Sittable == nil {
		state.SittableRemoved(m)
	}
	if s.Heartbeat != nil && prev.Heartbeat == nil {
		s.Heartbeat.Apply(m)
	}
}
