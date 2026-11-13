package state

import (
	"encoding/json"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// The components below do their work on the engine's side: items and
// containers, trading, loot, riding controls a player steers, projectiles
// and explosions hitting things, villages and raids, the client. entity
// reads them so the engine gets their settings from the same definition
// and the same group changes as the rest; their logic is not ported here.
// Defaults are zero unless a comment names the BDS function they come
// from.

// LootConfig is minecraft:loot.
type LootConfig struct {
	Table string `json:"table"`
}

// InventoryConfig is minecraft:inventory: the mob's container, which the
// engine owns.
type InventoryConfig struct {
	ContainerType              string `json:"container_type"`
	Size                       int32  `json:"inventory_size"`
	CanBeSiphonedFrom          bool   `json:"can_be_siphoned_from"`
	Private                    bool   `json:"private"`
	RestrictToOwner            bool   `json:"restrict_to_owner"`
	AdditionalSlotsPerStrength int32  `json:"additional_slots_per_strength"`
}

// TradeTableConfig is minecraft:trade_table.
type TradeTableConfig struct {
	Table                string `json:"table"`
	DisplayName          string `json:"display_name"`
	NewScreen            bool   `json:"new_screen"`
	PersistTrades        bool   `json:"persist_trades"`
	ConvertTradesEconomy bool   `json:"convert_trades_economy"`
}

// EconomyTradeTableConfig is minecraft:economy_trade_table. The discounts
// are ranges as written (one number or two).
type EconomyTradeTableConfig struct {
	Table                  string `json:"table"`
	DisplayName            string `json:"display_name"`
	NewScreen              bool   `json:"new_screen"`
	PersistTrades          bool   `json:"persist_trades"`
	ShowTradeScreen        bool   `json:"show_trade_screen"`
	UseLegacyPriceFormula  bool   `json:"use_legacy_price_formula"`
	HeroDemandDiscount     int32  `json:"hero_demand_discount"`
	CuredDiscount          behavior.Range[int32]
	MaxCuredDiscount       behavior.Range[int32]
	NearbyCuredDiscount    int32 `json:"nearby_cured_discount"`
	MaxNearbyCuredDiscount int32 `json:"max_nearby_cured_discount"`
}

// BossConfig is minecraft:boss (1.21.3 schema 0x140563180: name,
// should_darken_sky, hud_range).
type BossConfig struct {
	Name            string `json:"name"`
	HudRange        int32  `json:"hud_range"`
	ShouldDarkenSky bool   `json:"should_darken_sky"`
}

// ShooterProjectile is one of minecraft:shooter's projectiles: used when
// its filters pass.
type ShooterProjectile struct {
	Def        string
	AuxVal     int32
	Filters    *definition.Filter
	LoseTarget bool
}

// ShooterConfig is minecraft:shooter (1.21.3 schema 0x14062D6F0).
type ShooterConfig struct {
	Def         string
	AuxVal      int32
	Power       float32
	Magic       bool
	Sound       int
	Projectiles []ShooterProjectile
}

// Pick is the projectile to shoot at target: the first entry whose
// filters pass, else the default def. ShooterComponent::onShoot (1.21.3
// 0x14063ADE0), which makes this choice, was not read.
func (c *ShooterConfig) Pick(env *Env, target behavior.Actor) (def string, aux int32, loseTarget bool) {
	ctx := env.tester(env.Self, target)
	for _, p := range c.Projectiles {
		if p.Filters == nil || p.Filters.Eval(ctx) {
			return p.Def, p.AuxVal, p.LoseTarget
		}
	}
	return c.Def, c.AuxVal, false
}

// BalloonableConfig is minecraft:balloonable (1.21.3 schema 0x140562530:
// soft_distance 2, max_distance 10, mass 1).
type BalloonableConfig struct {
	SoftDistance, MaxDistance, Mass float32
	OnBalloon, OnUnballoon          *definition.Trigger
}

// DefaultBalloonable is the BDS default.
func DefaultBalloonable() BalloonableConfig {
	return BalloonableConfig{SoftDistance: 2, MaxDistance: 10, Mass: 1}
}

// BoostItem is one of minecraft:boostable's boost_items.
type BoostItem struct {
	Item        string `json:"item"`
	Damage      int32  `json:"damage"`
	ReplaceItem string `json:"replace_item"`
}

// BoostableConfig is minecraft:boostable. Duration is in ticks.
type BoostableConfig struct {
	Duration        int32
	SpeedMultiplier float32
	Items           []BoostItem
}

// ItemControllableConfig is minecraft:item_controllable.
type ItemControllableConfig struct {
	ControlItems []string
}

// DyeableConfig is minecraft:is_dyeable.
type DyeableConfig struct {
	InteractText string `json:"interact_text"`
}

// BreakBlocksConfig is minecraft:break_blocks: the block descriptors the
// mob breaks by walking into them (BreakBlocksComponent, the engine's).
type BreakBlocksConfig struct {
	Blocks []string
}

// DashActionConfig is minecraft:dash_action, a dash the rider triggers.
// Cooldown is in ticks.
type DashActionConfig struct {
	Cooldown                             int32
	HorizontalMomentum, VerticalMomentum float32
	CanDashUnderwater                    bool
	Direction                            string
}

// VerticalMovementActionConfig is minecraft:vertical_movement_action.
type VerticalMovementActionConfig struct {
	VerticalVelocity float32 `json:"vertical_velocity"`
}

// FreeCameraControlledConfig is minecraft:free_camera_controlled.
type FreeCameraControlledConfig struct {
	BackwardsMovementModifier float32 `json:"backwards_movement_modifier"`
	StrafeSpeedModifier       float32 `json:"strafe_speed_modifier"`
}

// ReflectProjectilesConfig is minecraft:reflect_projectiles. The angles
// and scale stay Molang, as written.
type ReflectProjectilesConfig struct {
	AzimuthAngle, ElevationAngle, ReflectionScale string
	Projectiles                                   []string
	Sound                                         int
}

// KnockbackPreset is one of minecraft:apply_knockback_rules' presets.
type KnockbackPreset struct {
	Filter                             *definition.Filter
	HorizontalPower, VerticalPower     float32
	VerticalVelocityCap, SlowdownScale float32
	ScaleWithDamage                    bool
	Mode, ExtraApproach                string
}

// KnockbackRulesConfig is minecraft:apply_knockback_rules.
type KnockbackRulesConfig struct {
	Presets []KnockbackPreset
}

// Preset is the first preset whose filter passes for the attacker.
func (c *KnockbackRulesConfig) Preset(env *Env, attacker behavior.Actor) (KnockbackPreset, bool) {
	ctx := env.tester(env.Self, attacker)
	for _, p := range c.Presets {
		if p.Filter == nil || p.Filter.Eval(ctx) {
			return p, true
		}
	}
	return KnockbackPreset{}, false
}

// RaidTriggerConfig is minecraft:raid_trigger.
type RaidTriggerConfig struct {
	Triggered *definition.Trigger
}

// MobEffectImmunityConfig is minecraft:mob_effect_immunity.
type MobEffectImmunityConfig struct {
	Effects []string
}

// Immune reports an effect the mob cannot get.
func (c *MobEffectImmunityConfig) Immune(effect string) bool {
	for _, e := range c.Effects {
		if e == effect {
			return true
		}
	}
	return false
}

// InsomniaConfig is minecraft:insomnia (1.21.3 schema 0x1405D9B20:
// days_until_insomnia 3).
type InsomniaConfig struct {
	Days float32 `json:"days_until_insomnia"`
}

// DefaultInsomnia is the BDS default.
func DefaultInsomnia() InsomniaConfig { return InsomniaConfig{Days: 3} }

// GrowsCropConfig is minecraft:grows_crop (bees). GrowCropSystem (1.21.3
// tick 0x14077FDF0) was not read.
type GrowsCropConfig struct {
	Charges int32   `json:"charges"`
	Chance  float32 `json:"chance"`
}

// DwellerConfig is minecraft:dweller: the mob's village membership, which
// the engine runs.
type DwellerConfig struct {
	DwellingType            string `json:"dwelling_type"`
	DwellerRole             string `json:"dweller_role"`
	PreferredProfession     string `json:"preferred_profession"`
	CanFindPOI              bool   `json:"can_find_poi"`
	CanMigrate              bool   `json:"can_migrate"`
	UpdateIntervalBase      int32  `json:"update_interval_base"`
	UpdateIntervalVariant   int32  `json:"update_interval_variant"`
	FirstFoundingReward     int32  `json:"first_founding_reward"`
	DwellingBoundsTolerance int32  `json:"dwelling_bounds_tolerance"`
}

// FlockingConfig is minecraft:flocking. FlockingComponent's neighbourhood
// and steering (1.21.3 manageNeighborhood 0x1405B5770,
// calculateFlockVector 0x1405AD850) are not ported.
type FlockingConfig struct {
	InWater                bool    `json:"in_water"`
	MatchVariants          bool    `json:"match_variants"`
	UseCenterOfMass        bool    `json:"use_center_of_mass"`
	LowFlockLimit          int32   `json:"low_flock_limit"`
	HighFlockLimit         int32   `json:"high_flock_limit"`
	GoalWeight             float32 `json:"goal_weight"`
	LonerChance            float32 `json:"loner_chance"`
	InfluenceRadius        float32 `json:"influence_radius"`
	BreachInfluence        float32 `json:"breach_influence"`
	SeparationWeight       float32 `json:"separation_weight"`
	SeparationThreshold    float32 `json:"separation_threshold"`
	CohesionWeight         float32 `json:"cohesion_weight"`
	CohesionThreshold      float32 `json:"cohesion_threshold"`
	InnerCohesionThreshold float32 `json:"innner_cohesion_threshold"`
	MinHeight              float32 `json:"min_height"`
	MaxHeight              float32 `json:"max_height"`
	BlockDistance          float32 `json:"block_distance"`
	BlockWeight            float32 `json:"block_weight"`
}

// GameEventMovementTrackingConfig is minecraft:game_event_movement_tracking:
// which movement game events the engine posts for the mob. That the
// members default to true was not read; vanilla only writes false.
type GameEventMovementTrackingConfig struct {
	EmitMove bool `json:"emit_move"`
	EmitSwim bool `json:"emit_swim"`
	EmitFlap bool `json:"emit_flap"`
}

// DefaultGameEventMovementTracking is the assumed default.
func DefaultGameEventMovementTracking() GameEventMovementTrackingConfig {
	return GameEventMovementTrackingConfig{EmitMove: true, EmitSwim: true, EmitFlap: true}
}

// LookedAtConfig is minecraft:looked_at (the creaking). Its 1.26.60
// parser (sub_6FDD2B0) was not decompiled and its system not found: the
// members are read as written and the engine runs it.
type LookedAtConfig struct {
	SearchRadius           float32
	SetTarget              string
	FindPlayersOnly        bool
	Cooldown               behavior.Range[float32]
	FieldOfView            float32
	ScaleFOVByDistance     bool
	LineOfSightObstruction string
	LookAtLocations        []string
	MinLookedAtDuration    float32
	Filters                *definition.Filter
	LookedAt, NotLookedAt  *definition.Trigger
}

// Passthrough are the components entity hands to the engine as written:
// projectiles, the NPC dialogue, network bandwidth, the player's own
// values and exhaustion. None of them belongs to a mob's AI.
func Passthrough() []string {
	return []string{
		"minecraft:projectile", "minecraft:npc", "minecraft:conditional_bandwidth_optimization",
		"minecraft:player.exhaustion", "minecraft:player.experience", "minecraft:player.level",
		"minecraft:player.saturation", "minecraft:exhaustion_values",
	}
}

// RawComponents holds passthrough components by name.
type RawComponents map[string]json.RawMessage
