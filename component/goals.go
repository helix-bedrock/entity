package component

import (
	"strconv"
	"strings"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/goal"
	"github.com/helix-bedrock/entity/path"
)

// Vanilla returns a Registry with every goal component of BDS 1.26.60.
// Each factory starts from the goal's Default config and reads the JSON
// over it; members whose units differ from the config (seconds read as
// ticks, sound and damage names, filters, triggers, descriptors) are read
// one by one.
func Vanilla() *Registry {
	r := NewRegistry()
	for name, f := range vanillaGoals() {
		r.Set(BehaviorPrefix+name, f)
	}
	return r
}

// simple wraps a goal that only needs the mob.
func simple[M any, G goal.Goal](mk func(behavior.World, M) G) GoalFactory {
	return func(c *Context, _ Object) (goal.Goal, error) {
		m, err := need[M](c)
		if err != nil {
			return nil, err
		}
		return mk(c.World, m), nil
	}
}

// configured wraps a goal whose config the JSON fills through its json
// tags, then read finishes the members that need more.
func configured[M any, C any, G goal.Goal](def func() C, read func(c *Context, j Object, cfg *C), mk func(behavior.World, M, C) G) GoalFactory {
	return func(c *Context, j Object) (goal.Goal, error) {
		cfg := def()
		c.Into(j, &cfg)
		if read != nil {
			read(c, j, &cfg)
		}
		m, err := need[M](c)
		if err != nil {
			return nil, err
		}
		return mk(c.World, m, cfg), nil
	}
}

// speedOnly wraps a goal made with just speed_multiplier.
func speedOnly[M any, G goal.Goal](def float32, mk func(behavior.World, M, float32) G) GoalFactory {
	return func(c *Context, j Object) (goal.Goal, error) {
		m, err := need[M](c)
		if err != nil {
			return nil, err
		}
		return mk(c.World, m, c.Float(j, "speed_multiplier", def)), nil
	}
}

// soundInterval reads sound_interval in seconds.
func soundInterval(c *Context, j Object, r *behavior.Range[float32]) {
	*r = c.Range(j, "sound_interval", *r)
}

// particleColor reads a "#RRGGBB" or "#AARRGGBB" colour as ARGB; six digits
// are opaque.
func particleColor(c *Context, j Object, k string, def int32) int32 {
	s := strings.TrimPrefix(c.String(j, k, ""), "#")
	if s == "" {
		return def
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		c.Warn(err)
		return def
	}
	if len(s) <= 6 {
		v |= 0xFF000000
	}
	return int32(uint32(v))
}

func readMelee(c *Context, j Object, cfg *behavior.MeleeConfig) {
	cfg.Cooldown = c.Ticks(j, "cooldown_time", cfg.Cooldown)
	cfg.AttackTypes = c.Descriptors(j, "attack_types")
	if cfg.AttackTypes == nil {
		cfg.AttackTypes = c.Descriptors(j, "attack_type")
	}
	cfg.OnAttack = c.Trigger(j, "on_attack")
	cfg.OnKill = c.Trigger(j, "on_kill")
	cfg.PathTime = c.RangeInt(j, "min_path_time", cfg.PathTime)
	cfg.InnerIncrease = c.Int(j, "inner_boundary_time_increase", cfg.InnerIncrease)
	cfg.OuterIncrease = c.Int(j, "outer_boundary_time_increase", cfg.OuterIncrease)
	cfg.FailIncrease = c.Int(j, "path_fail_time_increase", cfg.FailIncrease)
}

func readNearest(c *Context, j Object, cfg *behavior.NearestAttackableConfig) {
	cfg.EntityTypes = c.EntityTypes(j, "entity_types")
	cfg.ScanInterval = c.Int(j, "scan_interval", cfg.ScanInterval)
	if lo, hi, ok := rangeOf(j["attack_interval"]); ok && j.Has("attack_interval") {
		// A number, or the {"min", "max"} form of files written back out of
		// BDS; the goal keeps one interval, so the larger end is used.
		cfg.RandomInterval = int32(max(lo, hi))
	}
	cfg.Probability = c.Float(j, "persist_time", cfg.Probability)
	cfg.MustSeeForget = c.Ticks(j, "must_see_forget_duration", cfg.MustSeeForget)
	cfg.SneakMultiplier = c.Float(j, "target_sneak_visibility_multiplier", cfg.SneakMultiplier)
	cfg.InvisibleMultiplier = c.Float(j, "target_invisible_multiplier", cfg.InvisibleMultiplier)
}

func readLookAt(c *Context, j Object, cfg *behavior.LookAtConfig) {
	cfg.AngleVertical = c.Int(j, "angle_of_view_vertical", cfg.AngleVertical)
	cfg.AngleHorizontal = c.Int(j, "angle_of_view_horizontal", cfg.AngleHorizontal)
	cfg.LookTime = c.RangeTicks(j, "look_time", cfg.LookTime)
	if f := c.Filter(j, "filters"); f != nil {
		cfg.Filters = f
	}
}

func readRandomLook(c *Context, j Object, cfg *behavior.RandomLookConfig) {
	cfg.LookTime = behavior.Range[int32]{
		Min: c.Int(j, "min_look_time", cfg.LookTime.Min),
		Max: c.Int(j, "max_look_time", cfg.LookTime.Max),
	}
	cfg.LookTime = c.RangeTicks(j, "look_time", cfg.LookTime)
	cfg.Angle = behavior.Range[float32]{
		Min: c.Float(j, "min_angle_of_view_horizontal", cfg.Angle.Min),
		Max: c.Float(j, "max_angle_of_view_horizontal", cfg.Angle.Max),
	}
}

func readMoveToBlock(c *Context, j Object, cfg *behavior.MoveToBlockConfig) {
	if j.Has("target_selection_method") {
		m, ok := behavior.ParseSelection(c.String(j, "target_selection_method", ""))
		if !ok {
			c.Warn(errUnknown("target_selection_method"))
		}
		cfg.Method = m
	}
	cfg.TargetOffset = c.Vec3(j, "target_offset", cfg.TargetOffset)
	cfg.OnReach = c.Triggers(j, "on_reach")
	cfg.OnStayCompleted = c.Triggers(j, "on_stay_completed")
	cfg.TargetBlocks = c.Descriptors(j, "target_blocks")
	cfg.BlockFilters = c.BlockFilter(j, "target_block_filters")
}

func readTempt(c *Context, j Object, cfg *behavior.TemptConfig) {
	items := c.Descriptors(j, "items")
	r := c.Resolver
	cfg.Tempts = func(p behavior.Actor) bool { return r.HoldsItem(p, items) }
	cfg.Sound = c.Sound(j, "tempt_sound", cfg.Sound)
	soundInterval(c, j, &cfg.SoundInterval)
	cfg.OnTemptStart = c.Trigger(j, "on_tempt_start")
	cfg.OnTemptEnd = c.Trigger(j, "on_tempt_end")
}

func readMoveToLiquid(c *Context, j Object, cfg *behavior.MoveToLiquidConfig) {
	cfg.Material = c.Material(j, "material_type", cfg.Material)
}

func readGiveItems(c *Context, j Object, cfg *behavior.GiveItemsConfig) {
	cfg.RunSpeed = c.Float(j, "run_speed", cfg.RunSpeed)
	cfg.ThrowForce = c.Float(j, "throw_force", cfg.ThrowForce)
	cfg.VerticalThrowMul = c.Float(j, "vertical_throw_mul", cfg.VerticalThrowMul)
	cfg.ThrowSound = c.Sound(j, "throw_sound", cfg.ThrowSound)
	if t := c.Triggers(j, "on_item_throw"); t != nil {
		cfg.OnItemThrow = t
	}
}

// timer reads a timer_flag goal.
func timer(flag int) GoalFactory {
	return func(c *Context, j Object) (goal.Goal, error) {
		cfg := behavior.TimerFlagConfig{}
		cfg.Cooldown = c.RangeTicks(j, "cooldown_range", cfg.Cooldown)
		cfg.Duration = c.RangeTicks(j, "duration_range", cfg.Duration)
		cfg.OnStart = c.Trigger(j, "on_start")
		cfg.OnEnd = c.Trigger(j, "on_end")
		return behavior.NewTimerFlag(c.World, c.Mob, flag, cfg), nil
	}
}

type errUnknown string

func (e errUnknown) Error() string { return string(e) + ": unknown value" }

// eventChoices reads send_event's event_choices; times are in seconds.
func eventChoices(c *Context, j Object) []behavior.SendEventChoice {
	list := objects(j["event_choices"])
	out := make([]behavior.SendEventChoice, 0, len(list))
	for _, e := range list {
		ch := behavior.DefaultSendEventChoice()
		ch.MinActivationRange = c.Float(e, "min_activation_range", ch.MinActivationRange)
		ch.MaxActivationRange = c.Float(e, "max_activation_range", ch.MaxActivationRange)
		ch.CooldownTime = c.Ticks(e, "cooldown_time", ch.CooldownTime)
		ch.Weight = c.Float(e, "weight", ch.Weight)
		ch.LookAtTarget = c.Bool(e, "look_at_target", ch.LookAtTarget)
		ch.DoCasting = c.Bool(e, "do_casting", ch.DoCasting)
		ch.ParticleColor = particleColor(c, e, "particle_color", ch.ParticleColor)
		ch.StartSoundEvent = c.Sound(e, "start_sound_event", ch.StartSoundEvent)
		ch.Filters = c.Filter(e, "filters")
		for _, s := range objects(e["sequence"]) {
			ch.Sequence = append(ch.Sequence, behavior.SendEventStage{
				BaseDelay:  c.Ticks(s, "base_delay", 0),
				Event:      c.String(s, "event", ""),
				SoundEvent: c.Sound(s, "sound_event", behavior.NoSound),
			})
		}
		ch.CastDuration = c.Ticks(e, "cast_duration", 0)
		if ch.CastDuration <= 0 {
			ch.CastDuration = ch.TotalDelay()
		}
		out = append(out, ch)
	}
	return out
}

// summonChoices reads summon_entity's summon_choices; times are in seconds.
func summonChoices(c *Context, j Object) []behavior.SummonChoice {
	list := objects(j["summon_choices"])
	out := make([]behavior.SummonChoice, 0, len(list))
	for _, e := range list {
		ch := behavior.DefaultSummonChoice()
		ch.MinActivationRange = c.Float(e, "min_activation_range", ch.MinActivationRange)
		ch.MaxActivationRange = c.Float(e, "max_activation_range", ch.MaxActivationRange)
		ch.CooldownTime = c.Ticks(e, "cooldown_time", ch.CooldownTime)
		ch.Weight = c.Float(e, "weight", ch.Weight)
		ch.DoCasting = c.Bool(e, "do_casting", ch.DoCasting)
		ch.ParticleColor = particleColor(c, e, "particle_color", ch.ParticleColor)
		ch.StartSoundEvent = c.Sound(e, "start_sound_event", ch.StartSoundEvent)
		ch.Filters = c.Filter(e, "filters")
		for _, s := range objects(e["sequence"]) {
			q := behavior.DefaultSummonSequence()
			if c.String(s, "shape", "") == "circle" {
				q.Shape = behavior.SummonCircle
			}
			if c.String(s, "target", "") == "target" {
				q.Target = behavior.SummonAtTarget
			}
			q.Size = c.Float(s, "size", q.Size)
			q.BaseDelay = c.Ticks(s, "base_delay", q.BaseDelay)
			q.DelayPerSummon = c.Ticks(s, "delay_per_summon", q.DelayPerSummon)
			q.SummonCap = c.Int(s, "summon_cap", q.SummonCap)
			q.SummonCapRadius = c.Float(s, "summon_cap_radius", q.SummonCapRadius)
			q.EntityType = c.String(s, "entity_type", q.EntityType)
			if s.Has("entity_lifespan") {
				if l := c.Float(s, "entity_lifespan", -1); l >= 0 {
					q.EntityLifespan = int32(l * ticksPerSecond)
				}
			}
			q.NumEntitiesSpawned = c.Int(s, "num_entities_spawned", q.NumEntitiesSpawned)
			q.SoundEvent = c.Sound(s, "sound_event", q.SoundEvent)
			q.SummonEvent = c.String(s, "summon_event", q.SummonEvent)
			ch.Sequence = append(ch.Sequence, q)
		}
		ch.CastDuration = c.Ticks(e, "cast_duration", -1)
		if ch.CastDuration <= 0 {
			ch.CastDuration = ch.TotalDelay()
		}
		out = append(out, ch)
	}
	return out
}

// materials reads lay_egg's target_materials_above_block. Only the
// material names the vanilla files use are known; others are reported.
func materials(c *Context, j Object, k string, def []path.Material) []path.Material {
	if !j.Has(k) {
		return def
	}
	known := map[string]path.Material{
		"air": path.MaterialAir, "water": path.MaterialWater, "lava": path.MaterialLava, "any": behavior.MaterialAny,
	}
	var out []path.Material
	for _, n := range c.Descriptors(j, k) {
		m, ok := known[strings.ToLower(n)]
		if !ok {
			c.Warn(errUnknown(k + " " + n))
			continue
		}
		out = append(out, m)
	}
	return out
}

func vanillaGoals() map[string]GoalFactory {
	return map[string]GoalFactory{
		// Wandering and looking.
		"random_stroll": configured(behavior.DefaultStroll, nil, behavior.NewStroll),
		"random_swim":   configured(behavior.DefaultRandomSwim, nil, behavior.NewRandomSwim),
		"random_fly":    configured(behavior.DefaultRandomFly, nil, behavior.NewRandomFly),
		"random_breach": configured(behavior.DefaultRandomBreach, func(c *Context, j Object, cfg *behavior.RandomBreachConfig) {
			cfg.Cooldown = c.Ticks(j, "cooldown_time", cfg.Cooldown)
		}, behavior.NewRandomBreach),
		"random_hover": configured(behavior.DefaultRandomHover, func(c *Context, j Object, cfg *behavior.RandomHoverConfig) {
			cfg.HoverHeight = c.RangeInt(j, "hover_height", cfg.HoverHeight)
		}, behavior.NewRandomHover),
		"hover": configured(behavior.DefaultHover, nil, behavior.NewHover),
		"float_wander": configured(behavior.DefaultFloatWander, func(c *Context, j Object, cfg *behavior.FloatWanderConfig) {
			cfg.FloatDuration = c.Range(j, "float_duration", cfg.FloatDuration)
		}, behavior.NewFloatWander),
		"swim_idle": configured(behavior.DefaultSwimIdle, func(c *Context, j Object, cfg *behavior.SwimIdleConfig) {
			cfg.IdleTime = c.Ticks(j, "idle_time", cfg.IdleTime)
		}, behavior.NewSwimIdle),
		"swim_wander": configured(behavior.DefaultSwimWander, func(c *Context, j Object, cfg *behavior.SwimWanderConfig) {
			cfg.WanderTime = c.Ticks(j, "wander_time", cfg.WanderTime)
		}, behavior.NewSwimWander),
		"look_at_player":         configured(behavior.DefaultLookAt, readLookAt, behavior.NewLookAt),
		"look_at_entity":         configured(behavior.DefaultLookAt, readLookAt, behavior.NewLookAt),
		"look_at_target":         configured(behavior.DefaultLookAtTarget, readLookAt, behavior.NewLookAtTarget),
		"look_at_trading_player": configured(behavior.DefaultLookAt, readLookAt, behavior.NewLookAtTrading),
		"random_look_around":     configured(behavior.DefaultRandomLook, readRandomLook, behavior.NewRandomLook),
		"random_look_around_and_sit": configured(behavior.DefaultRandomLookSit, func(c *Context, j Object, cfg *behavior.RandomLookSitConfig) {
			readRandomLook(c, j, &cfg.RandomLookConfig)
			cfg.Looks = behavior.Range[int32]{Min: c.Int(j, "min_look_count", cfg.Looks.Min), Max: c.Int(j, "max_look_count", cfg.Looks.Max)}
			cfg.Cooldown = c.Int(j, "random_look_around_cooldown", cfg.Cooldown)
		}, behavior.NewRandomLookSit),
		"float": configured(behavior.DefaultFloat, func(c *Context, j Object, cfg *behavior.FloatConfig) {
			if j.Has("time_under_water_to_dismount_passengers") {
				cfg.DismountTicks = int32(c.Float(j, "time_under_water_to_dismount_passengers", 0)*ticksPerSecond + 0.5)
			}
		}, behavior.NewFloat),
		"run_around_like_crazy":             speedOnly(1, behavior.NewRunAroundLikeCrazy),
		"move_towards_home_restriction":     speedOnly(1, behavior.NewTowardsHome),
		"move_towards_dwelling_restriction": speedOnly(1, behavior.NewTowardsDwelling),
		"mount_pathing":                     configured(behavior.DefaultMountPathing, nil, behavior.NewMountPathing),
		"move_towards_target":               configured(behavior.DefaultMoveTowardsTarget, nil, behavior.NewMoveTowardsTarget),
		"move_to_random_block":              configured(behavior.DefaultMoveToRandomBlock, nil, behavior.NewMoveToRandomBlock),
		"move_to_block":                     configured(behavior.DefaultMoveToBlock, readMoveToBlock, behavior.NewMoveToBlock),
		"move_to_water":                     configured(func() behavior.MoveToLiquidConfig { return behavior.DefaultMoveToWater }, readMoveToLiquid, behavior.NewMoveToLiquid),
		"move_to_lava":                      configured(func() behavior.MoveToLiquidConfig { return behavior.DefaultMoveToLava }, readMoveToLiquid, behavior.NewMoveToLiquid),
		"move_to_liquid":                    configured(func() behavior.MoveToLiquidConfig { return behavior.DefaultMoveToLiquid }, readMoveToLiquid, behavior.NewMoveToLiquid),
		"move_to_land":                      configured(behavior.DefaultMoveTo, nil, behavior.NewMoveToLand),
		"swim_up_for_breath": configured(behavior.DefaultSwimUpForBreath, func(c *Context, j Object, cfg *behavior.SwimUpForBreathConfig) {
			cfg.Material = c.Material(j, "material_type", cfg.Material)
		}, behavior.NewSwimUpForBreath),
		"rise_to_liquid_level": configured(behavior.DefaultRiseToLiquidLevel, nil, behavior.NewRiseToLiquidLevel),
		"flee_sun":             configured(behavior.DefaultFleeSun, nil, behavior.NewFleeSun),
		"restrict_sun":         simple(behavior.NewRestrictSun),
		"find_cover": configured(behavior.DefaultFindCover, func(c *Context, j Object, cfg *behavior.FindCoverConfig) {
			cfg.Cooldown = c.Ticks(j, "cooldown", cfg.Cooldown)
		}, behavior.NewFindCover),
		"move_around_target": configured(behavior.DefaultMoveAroundTarget, func(c *Context, j Object, cfg *behavior.MoveAroundTargetConfig) {
			cfg.Filters = c.Filter(j, "filters")
			cfg.Range = c.Range(j, "destination_position_range", cfg.Range)
		}, behavior.NewMoveAroundTarget),
		"avoid_block": configured(behavior.DefaultAvoidBlock, func(c *Context, j Object, cfg *behavior.AvoidBlockConfig) {
			if j.Has("target_selection_method") {
				cfg.Method, _ = behavior.ParseSelection(c.String(j, "target_selection_method", ""))
			}
			cfg.TargetBlocks = c.Descriptors(j, "target_blocks")
			cfg.OnEscape = c.Triggers(j, "on_escape")
			cfg.Sound = c.Sound(j, "avoid_block_sound", cfg.Sound)
			soundInterval(c, j, &cfg.SoundInterval)
		}, behavior.NewAvoidBlock),
		"avoid_mob_type": configured(behavior.DefaultAvoid, func(c *Context, j Object, cfg *behavior.AvoidConfig) {
			cfg.Types = nil
			if j.Has("entity_types") {
				for _, e := range objects(j["entity_types"]) {
					t := behavior.DefaultAvoidType()
					c.Into(e, &t)
					t.Filters = c.Filter(e, "filters")
					cfg.Types = append(cfg.Types, t)
				}
			}
			cfg.Sound = c.Sound(j, "avoid_mob_sound", cfg.Sound)
			soundInterval(c, j, &cfg.SoundInterval)
			cfg.OnEscape = c.Trigger(j, "on_escape_event")
		}, behavior.NewAvoid),
		"panic": configured(behavior.DefaultPanic, func(c *Context, j Object, cfg *behavior.PanicConfig) {
			if j.Has("damage_sources") {
				cfg.DamageSources = c.DamageSources(j, "damage_sources")
			}
			cfg.Sound = c.Sound(j, "panic_sound", cfg.Sound)
			soundInterval(c, j, &cfg.SoundInterval)
		}, behavior.NewPanic),
		"tempt":              configured(behavior.DefaultTempt, readTempt, behavior.NewTempt),
		"float_tempt":        configured(behavior.DefaultTempt, readTempt, behavior.NewFloatTempt),
		"stomp_turtle_egg":   configured(behavior.DefaultStompEgg, nil, behavior.NewStompEgg),
		"random_sitting":     configured(behavior.DefaultRandomSit, nil, behavior.NewRandomSit),
		"stay_while_sitting": simple(behavior.NewSit),
		"lay_down": configured(behavior.DefaultLayDown, func(c *Context, j Object, cfg *behavior.LayDownConfig) {
			cfg.Interval = c.Int(j, "interval", cfg.Interval)
			cfg.RandomStopInterval = c.Int(j, "random_stop_interval", cfg.RandomStopInterval)
		}, behavior.NewLayDown),
		"scared": configured(behavior.DefaultScared, func(c *Context, j Object, cfg *behavior.ScaredConfig) {
			cfg.SoundInterval = c.Int(j, "sound_interval", cfg.SoundInterval)
		}, behavior.NewScared),
		"croak": configured(behavior.DefaultCroak, func(c *Context, j Object, cfg *behavior.CroakConfig) {
			cfg.Interval = c.Range(j, "interval", cfg.Interval)
			cfg.Duration = c.Range(j, "duration", cfg.Duration)
			cfg.Filters = c.Filter(j, "filters")
		}, behavior.NewCroak),
		"nap": configured(behavior.DefaultNap, func(c *Context, j Object, cfg *behavior.NapConfig) {
			cfg.CooldownMin = c.Ticks(j, "cooldown_min", cfg.CooldownMin)
			cfg.CooldownMax = c.Ticks(j, "cooldown_max", cfg.CooldownMax)
			cfg.CanNapFilters = c.Filter(j, "can_nap_filters")
			cfg.WakeMobExceptions = c.Filter(j, "wake_mob_exceptions")
		}, behavior.NewNap),
		"timer_flag_1": timer(behavior.FlagTimer1),
		"timer_flag_2": timer(behavior.FlagTimer2),
		"timer_flag_3": timer(behavior.FlagTimer3),

		// Family, owners and followers.
		"breed":         speedOnly(1, behavior.NewBreed),
		"follow_parent": speedOnly(1, behavior.NewFollowParent),
		"make_love":     simple(behavior.NewMakeLove),
		"receive_love":  simple(behavior.NewReceiveLove),
		"follow_owner":  configured(behavior.DefaultFollowOwner, nil, behavior.NewFollowOwner),
		"teleport_to_owner": configured(behavior.DefaultTeleportToOwner, func(c *Context, j Object, cfg *behavior.TeleportToOwnerConfig) {
			cfg.Filters = c.Filter(j, "filters")
			cfg.Cooldown = c.Ticks(j, "cooldown", cfg.Cooldown)
		}, behavior.NewTeleportToOwner),
		"follow_mob": configured(behavior.DefaultFollowMob, func(c *Context, j Object, cfg *behavior.FollowMobConfig) {
			if f := c.Filter(j, "filters"); f != nil {
				cfg.Filters = f
			}
		}, behavior.NewFollowMob),
		"follow_caravan": configured(behavior.DefaultFollowCaravan, func(c *Context, j Object, cfg *behavior.FollowCaravanConfig) {
			cfg.SpeedMultiplier = c.Float(j, "speed_multiplier", cfg.SpeedMultiplier)
			cfg.EntityCount = c.Int(j, "entity_count", cfg.EntityCount)
			cfg.EntityTypes = c.FilterList(j, "entity_types")
		}, behavior.NewFollowCaravan),
		"follow_target_captain": configured(behavior.DefaultFollowTargetLeader, func(c *Context, j Object, cfg *behavior.FollowTargetLeaderConfig) {
			cfg.SpeedMultiplier = c.Float(j, "speed_multiplier", cfg.SpeedMultiplier)
			cfg.WithinRadius = c.Float(j, "within_radius", cfg.WithinRadius)
			cfg.FollowDistance = c.Float(j, "follow_distance", cfg.FollowDistance)
			cfg.AlwaysLookForLeader = c.Bool(j, "always_look_for_leader", cfg.AlwaysLookForLeader)
			if f := c.Filter(j, "leader_filters"); f != nil {
				cfg.LeaderFilters = f
			}
			cfg.SearchCooldown = c.Int(j, "search_cooldown", cfg.SearchCooldown)
		}, behavior.NewFollowTargetLeader),
		"player_ride_tamed":    simple(behavior.NewPlayerRideTamed),
		"controlled_by_player": configured(behavior.DefaultControlledByPlayer, nil, behavior.NewControlledByPlayer),
		"find_mount":           configured(behavior.DefaultFindMount, nil, behavior.NewFindMount),
		"pet_sleep_with_owner": configured(behavior.DefaultPetSleepWithOwner, nil, behavior.NewPetSleepWithOwner),
		"beg": configured(behavior.DefaultBeg, func(c *Context, j Object, cfg *behavior.BegConfig) {
			cfg.Items = c.Descriptors(j, "items")
			cfg.LookTime = c.RangeTicks(j, "look_time", cfg.LookTime)
		}, behavior.NewBeg),
		"play": configured(behavior.DefaultPlay, func(c *Context, j Object, cfg *behavior.PlayConfig) {
			cfg.SpeedMultiplier = c.Float(j, "speed_multiplier", cfg.SpeedMultiplier)
			cfg.MaxPlayDuration = c.Ticks(j, "max_play_duration_seconds", cfg.MaxPlayDuration)
			cfg.ChanceToStart = c.Float(j, "chance_to_start", cfg.ChanceToStart)
			cfg.FollowDistance = c.Int(j, "follow_distance", cfg.FollowDistance)
			if j.Has("friend_types") {
				cfg.FriendTypes = c.FilterList(j, "friend_types")
			}
			cfg.FriendSearchArea = c.Vec3(j, "friend_search_area", cfg.FriendSearchArea)
			cfg.RandomPosSearchRange = c.Int(j, "random_pos_search_range", cfg.RandomPosSearchRange)
			cfg.RandomPosSearchHeight = c.Int(j, "random_pos_search_height", cfg.RandomPosSearchHeight)
		}, behavior.NewPlay),
		"play_dead": configured(behavior.DefaultPlayDead, func(c *Context, j Object, cfg *behavior.PlayDeadConfig) {
			cfg.Duration = c.Ticks(j, "duration", cfg.Duration)
			cfg.RandomDamageRange = c.RangeInt(j, "random_damage_range", cfg.RandomDamageRange)
			if j.Has("damage_sources") {
				cfg.DamageSources = c.DamageSources(j, "damage_sources")
			}
			cfg.Filters = c.Filter(j, "filters")
		}, behavior.NewPlayDead),
		"swim_with_entity": configured(behavior.DefaultSwimWithEntity, func(c *Context, j Object, cfg *behavior.SwimWithEntityConfig) {
			cfg.EntityTypes = c.FilterList(j, "entity_types")
		}, behavior.NewSwimWithEntity),

		// Targets.
		"hurt_by_target": configured(func() behavior.HurtByConfig { return behavior.HurtByConfig{} }, func(c *Context, j Object, cfg *behavior.HurtByConfig) {
			cfg.EntityTypes = c.EntityTypes(j, "entity_types")
			cfg.MustReach = c.Bool(j, "must_reach", cfg.MustReach)
			cfg.MustSee = c.Bool(j, "must_see", cfg.MustSee)
		}, behavior.NewHurtBy),
		"owner_hurt_by_target":                  simple(behavior.NewOwnerHurtBy),
		"owner_hurt_target":                     simple(behavior.NewOwnerHurt),
		"nearest_attackable_target":             configured(behavior.DefaultNearestAttackable, readNearest, behavior.NewNearestAttackable),
		"nearest_prioritized_attackable_target": configured(behavior.DefaultNearestAttackable, readNearest, behavior.NewNearestPrioritized),
		"defend_trusted_target": configured(behavior.DefaultDefendTrusted, func(c *Context, j Object, cfg *behavior.DefendTrustedConfig) {
			readNearest(c, j, &cfg.NearestAttackableConfig)
			cfg.OnDefendStart = c.Trigger(j, "on_defend_start")
		}, behavior.NewDefendTrusted),
		"defend_village_target": configured(behavior.DefaultDefendVillage, func(c *Context, j Object, cfg *behavior.DefendVillageConfig) {
			readNearest(c, j, &cfg.NearestAttackableConfig)
		}, behavior.NewDefendVillage),
		"target_when_pushed": configured(func() behavior.TargetWhenPushedConfig { return behavior.TargetWhenPushedConfig{} }, func(c *Context, j Object, cfg *behavior.TargetWhenPushedConfig) {
			cfg.EntityTypes = c.EntityTypes(j, "entity_types")
		}, behavior.NewTargetWhenPushed),
		"hold_ground": configured(behavior.DefaultHoldGround, func(c *Context, j Object, cfg *behavior.HoldGroundConfig) {
			cfg.WithinRadiusEvent = c.Trigger(j, "within_radius_event")
		}, behavior.NewHoldGround),
		"wither_target_highest_damage": configured(behavior.DefaultWitherTargetHighestDamage, func(c *Context, j Object, cfg *behavior.WitherTargetHighestDamageConfig) {
			cfg.MustSeeForget = c.Ticks(j, "must_see_forget_duration", cfg.MustSeeForget)
		}, behavior.NewWitherTargetHighestDamage),
		"vex_copy_owner_target": simple(behavior.NewVexCopyOwnerTarget),

		// Attacks.
		"melee_attack":     configured(behavior.DefaultMelee, readMelee, behavior.NewMelee),
		"melee_box_attack": configured(behavior.DefaultMeleeBox, readMelee, behavior.NewMeleeBox),
		"delayed_attack": configured(behavior.DefaultDelayedAttack, func(c *Context, j Object, cfg *behavior.DelayedAttackConfig) {
			readMelee(c, j, &cfg.MeleeConfig)
			cfg.AttackDuration = c.Ticks(j, "attack_duration", cfg.AttackDuration)
			if j.Has("attack_duration") || j.Has("hit_delay_pct") {
				cfg.HitDelay = int32(float32(cfg.AttackDuration) * c.Float(j, "hit_delay_pct", 0.5))
			}
			cfg.Sound = c.Sound(j, "sound_event", cfg.Sound)
		}, behavior.NewDelayedAttack),
		"stomp_attack": configured(behavior.DefaultStompAttack, func(c *Context, j Object, cfg *behavior.StompAttackConfig) {
			readMelee(c, j, &cfg.MeleeConfig)
		}, behavior.NewStompAttack),
		"use_kinetic_weapon": configured(behavior.DefaultUseKineticWeapon, func(c *Context, j Object, cfg *behavior.UseKineticWeaponConfig) {
			readMelee(c, j, &cfg.MeleeConfig)
			cfg.RepositionDistance = c.Range(j, "reposition_distance", cfg.RepositionDistance)
			cfg.CooldownDistance = c.Range(j, "cooldown_distance", cfg.CooldownDistance)
		}, behavior.NewUseKineticWeapon),
		"ranged_attack":  configured(behavior.DefaultRangedAttack, nil, behavior.NewRangedAttack),
		"leap_at_target": configured(behavior.DefaultLeap, nil, behavior.NewLeap),
		"ocelotattack": configured(behavior.DefaultOcelotAttack, func(c *Context, j Object, cfg *behavior.OcelotAttackConfig) {
			cfg.Cooldown = c.Ticks(j, "cooldown_time", cfg.Cooldown)
		}, behavior.NewOcelotAttack),
		"swell": configured(behavior.DefaultSwell, nil, behavior.NewSwell),
		"charge_attack": configured(behavior.DefaultChargeAttack, func(c *Context, j Object, cfg *behavior.ChargeAttackConfig) {
			cfg.SoundEvent = c.Sound(j, "sound_event", cfg.SoundEvent)
		}, behavior.NewChargeAttack),
		"slime_attack":           configured(behavior.DefaultSlimeAttack, nil, behavior.NewSlimeAttack),
		"slime_float":            configured(behavior.DefaultSlimeFloat, nil, behavior.NewSlimeFloat),
		"slime_keep_on_jumping":  configured(behavior.DefaultSlimeKeepOnJumping, nil, behavior.NewSlimeKeepOnJumping),
		"slime_random_direction": configured(behavior.DefaultSlimeRandomDirection, nil, behavior.NewSlimeRandomDirection),
		"fire_at_target": configured(behavior.DefaultFireAtTarget, func(c *Context, j Object, cfg *behavior.FireAtTargetConfig) {
			cfg.Filters = c.Filter(j, "filters")
			cfg.AttackRange = c.Range(j, "attack_range", cfg.AttackRange)
			cfg.AttackCooldown = c.Ticks(j, "attack_cooldown", cfg.AttackCooldown)
			cfg.PreShootDelay = c.Ticks(j, "pre_shoot_delay", cfg.PreShootDelay)
			cfg.PostShootDelay = c.Ticks(j, "post_shoot_delay", cfg.PostShootDelay)
			cfg.OwnerOffset = c.Vec3(j, "owner_offset", cfg.OwnerOffset)
			cfg.TargetOffset = c.Vec3(j, "target_offset", cfg.TargetOffset)
		}, behavior.NewFireAtTarget),
		"jump_around_target": configured(behavior.DefaultJumpAroundTarget, func(c *Context, j Object, cfg *behavior.JumpAroundTargetConfig) {
			cfg.Filters = c.Filter(j, "filters")
			cfg.JumpCooldown = c.Ticks(j, "jump_cooldown_duration", cfg.JumpCooldown)
			cfg.JumpCooldownHurt = c.Ticks(j, "jump_cooldown_when_hurt_duration", cfg.JumpCooldownHurt)
			cfg.LastHurtDuration = c.Ticks(j, "last_hurt_duration", cfg.LastHurtDuration)
			if j.Has("prepare_jump_duration") {
				cfg.PrepareJump = uint64(c.Ticks(j, "prepare_jump_duration", 0))
			}
			cfg.ValidDistance = c.Range(j, "valid_distance_to_target", cfg.ValidDistance)
			if j.Has("valid_distance_to_target") {
				cfg.ValidDistance.Min *= cfg.ValidDistance.Min
				cfg.ValidDistance.Max *= cfg.ValidDistance.Max
			}
			cfg.LandingDistance = c.Range(j, "landing_distance_from_target", cfg.LandingDistance)
		}, behavior.NewJumpAroundTarget),
		"aquatic_charge_attack": configured(behavior.DefaultAquaticChargeAttack, func(c *Context, j Object, cfg *behavior.AquaticChargeAttackConfig) {
			cfg.Cooldown = c.Range(j, "charge_cooldown_time", cfg.Cooldown)
			cfg.HasSpeedMultiplier = j.Has("charge_speed_multiplier")
		}, behavior.NewAquaticChargeAttack),
		"ram_attack": configured(behavior.DefaultRamAttack, func(c *Context, j Object, cfg *behavior.RamAttackConfig) {
			cfg.PreRamSound = c.Sound(j, "pre_ram_sound", cfg.PreRamSound)
			cfg.RamImpactSound = c.Sound(j, "ram_impact_sound", cfg.RamImpactSound)
			cfg.Cooldown = c.Range(j, "cooldown_range", cfg.Cooldown)
			cfg.OnStart = c.Trigger(j, "on_start")
		}, behavior.NewRamAttack),
		"stalk_and_pounce_on_target": configured(behavior.DefaultStalkAndPounce, func(c *Context, j Object, cfg *behavior.StalkAndPounceConfig) {
			sq := func(k string, def float32) float32 {
				if !j.Has(k) {
					return def
				}
				v := c.Float(j, k, 0)
				return v * v
			}
			cfg.MaxStalkDist = sq("max_stalk_dist", cfg.MaxStalkDist)
			cfg.PounceMaxDist = sq("pounce_max_dist", cfg.PounceMaxDist)
			cfg.StrikeDist = sq("strike_dist", cfg.StrikeDist)
			cfg.InterestTime = c.Ticks(j, "interest_time", cfg.InterestTime)
			cfg.StuckTime = c.Ticks(j, "stuck_time", cfg.StuckTime)
			cfg.StuckBlocks = c.BlockFilter(j, "stuck_blocks")
		}, behavior.NewStalkAndPounce),
		"knockback_roar": configured(behavior.DefaultKnockbackRoar, func(c *Context, j Object, cfg *behavior.KnockbackRoarConfig) {
			cfg.Duration = c.Ticks(j, "duration", cfg.Duration)
			cfg.AttackTime = c.Ticks(j, "attack_time", cfg.AttackTime)
			cfg.Cooldown = c.Ticks(j, "cooldown_time", cfg.Cooldown)
			if f := c.RoarFilter(j, "knockback_filters"); f != nil {
				cfg.KnockbackFilters = f
			}
			if f := c.RoarFilter(j, "damage_filters"); f != nil {
				cfg.DamageFilters = f
			}
			cfg.OnRoarEnd = c.Trigger(j, "on_roar_end")
		}, behavior.NewKnockbackRoar),
		"swoop_attack": configured(behavior.DefaultSwoopAttack, func(c *Context, j Object, cfg *behavior.SwoopAttackConfig) {
			cfg.DelayRange = c.Range(j, "delay_range", cfg.DelayRange)
		}, behavior.NewSwoopAttack),
		"circle_around_anchor": configured(behavior.DefaultCircleAroundAnchor, func(c *Context, j Object, cfg *behavior.CircleAroundAnchorConfig) {
			if j.Has("goal_radius") {
				r := c.Float(j, "goal_radius", 0)
				cfg.GoalRadius = r * r
			}
			cfg.RadiusRange = c.Range(j, "radius_range", cfg.RadiusRange)
			cfg.HeightOffset = c.Range(j, "height_offset_range", cfg.HeightOffset)
			cfg.HeightAbove = c.Range(j, "height_above_target_range", cfg.HeightAbove)
		}, behavior.NewCircleAroundAnchor),
		"guardian_attack":               configured(behavior.DefaultGuardianAttack, nil, behavior.NewGuardianAttack),
		"wither_random_attack_pos_goal": configured(behavior.DefaultWitherRandomAttackPos, nil, behavior.NewWitherRandomAttackPos),
		"vex_random_move":               configured(behavior.DefaultVexRandomMove, nil, behavior.NewVexRandomMove),
		"send_event": configured(behavior.DefaultSendEvent, func(c *Context, j Object, cfg *behavior.SendEventConfig) {
			cfg.EventChoices = eventChoices(c, j)
		}, behavior.NewSendEvent),
		"summon_entity": configured(behavior.DefaultSummonEntity, func(c *Context, j Object, cfg *behavior.SummonEntityConfig) {
			cfg.SummonChoices = summonChoices(c, j)
		}, behavior.NewSummonEntity),

		// Warden.
		"emerge": configured(behavior.DefaultEmerge, func(c *Context, j Object, cfg *behavior.EmergeConfig) {
			cfg.Duration = c.Ticks(j, "duration", cfg.Duration)
			cfg.Cooldown = c.Ticks(j, "cooldown_time", cfg.Cooldown)
			cfg.OnDone = c.Trigger(j, "on_done")
		}, behavior.NewEmerge),
		"dig": configured(behavior.DefaultDig, func(c *Context, j Object, cfg *behavior.DigConfig) {
			cfg.Duration = c.Ticks(j, "duration", cfg.Duration)
			cfg.IdleTime = c.Ticks(j, "idle_time", cfg.IdleTime)
			cfg.OnStart = c.Trigger(j, "on_start")
		}, behavior.NewDig),
		"roar": configured(behavior.DefaultRoar, func(c *Context, j Object, cfg *behavior.RoarConfig) {
			cfg.Duration = c.Ticks(j, "duration", cfg.Duration)
		}, behavior.NewRoar),
		"sniff": configured(behavior.DefaultSniff, func(c *Context, j Object, cfg *behavior.SniffConfig) {
			cfg.Duration = c.Ticks(j, "duration", cfg.Duration)
			cfg.Cooldown = c.Range(j, "cooldown_range", cfg.Cooldown)
		}, behavior.NewSniff),
		"investigate_suspicious_location": configured(behavior.DefaultInvestigate, nil, behavior.NewInvestigate),
		"sonic_boom": configured(behavior.DefaultSonicBoom, func(c *Context, j Object, cfg *behavior.SonicBoomConfig) {
			cfg.Duration = c.Ticks(j, "duration", cfg.Duration)
			cfg.AttackCooldown = c.Ticks(j, "attack_cooldown", cfg.AttackCooldown)
			cfg.DurationUntilAttackSound = c.Ticks(j, "duration_until_attack_sound", cfg.DurationUntilAttackSound)
			cfg.AttackSound = c.Sound(j, "attack_sound", cfg.AttackSound)
			cfg.ChargeSound = c.Sound(j, "charge_sound", cfg.ChargeSound)
		}, behavior.NewSonicBoom),

		// Items.
		"admire_item": configured(behavior.DefaultAdmireItem, func(c *Context, j Object, cfg *behavior.AdmireItemConfig) {
			cfg.Sound = c.Sound(j, "admire_item_sound", cfg.Sound)
			soundInterval(c, j, &cfg.SoundInterval)
			cfg.OnStart = c.Trigger(j, "on_admire_item_start")
			cfg.OnStop = c.Trigger(j, "on_admire_item_stop")
		}, behavior.NewAdmireItem),
		"barter":     simple(behavior.NewBarter),
		"equip_item": simple(behavior.NewEquipItem),
		"pickup_items": configured(behavior.DefaultPickupItems, func(c *Context, j Object, cfg *behavior.PickupItemsConfig) {
			cfg.ExcludedItems = c.Descriptors(j, "excluded_items")
			cfg.OnStart = c.Trigger(j, "on_pickup_item_start")
			cfg.OnEnd = c.Trigger(j, "on_pickup_item_end")
		}, behavior.NewPickupItems),
		"share_items": configured(behavior.DefaultShareItems, func(c *Context, j Object, cfg *behavior.ShareItemsConfig) {
			cfg.EntityTypes = c.FilterList(j, "entity_types")
		}, behavior.NewShareItems),
		"charge_held_item": configured(func() behavior.ChargeHeldItemConfig { return behavior.ChargeHeldItemConfig{} }, func(c *Context, j Object, cfg *behavior.ChargeHeldItemConfig) {
			cfg.Items = c.Descriptors(j, "items")
		}, behavior.NewChargeHeldItem),
		"snacking": configured(behavior.DefaultSnacking, func(c *Context, j Object, cfg *behavior.SnackingConfig) {
			cfg.Items = c.Descriptors(j, "items")
		}, behavior.NewSnacking),
		"eat_carried_item": configured(func() behavior.EatCarriedItemConfig { return behavior.EatCarriedItemConfig{} }, func(c *Context, j Object, cfg *behavior.EatCarriedItemConfig) {
			cfg.DelayBeforeEating = uint64(c.Float(j, "delay_before_eating", 0))
		}, behavior.NewEatCarriedItem),
		"drink_milk": configured(func() behavior.DrinkMilkConfig { return behavior.DrinkMilkConfig{} }, func(c *Context, j Object, cfg *behavior.DrinkMilkConfig) {
			cfg.Cooldown = c.Ticks(j, "cooldown_seconds", cfg.Cooldown)
			cfg.Filters = c.Filter(j, "filters")
		}, behavior.NewDrinkMilk),
		"drink_potion": configured(func() behavior.DrinkPotionConfig { return behavior.DrinkPotionConfig{} }, func(c *Context, j Object, cfg *behavior.DrinkPotionConfig) {
			cfg.Potions = nil
			for _, p := range objects(j["potions"]) {
				cfg.Potions = append(cfg.Potions, behavior.Potion{
					ID:      int16(c.Int(p, "id", -1)),
					Chance:  c.Float(p, "chance", 1),
					Filters: c.Filter(p, "filters"),
				})
			}
		}, behavior.NewDrinkPotion),
		"drop_item_for": configured(behavior.DefaultDropItemFor, func(c *Context, j Object, cfg *behavior.DropItemForConfig) {
			cfg.TargetRange = c.Vec3(j, "target_range", cfg.TargetRange)
			cfg.TeleportOffset = c.Vec3(j, "teleport_offset", cfg.TeleportOffset)
			cfg.TimeOfDayRange = c.Range(j, "time_of_day_range", cfg.TimeOfDayRange)
			cfg.OnDropAttempt = c.Trigger(j, "on_drop_attempt")
			cfg.EntityTypes = c.FilterList(j, "entity_types")
		}, behavior.NewDropItemFor),
		"go_and_give_items_to_owner": configured(behavior.DefaultGiveToOwner, func(c *Context, j Object, cfg *behavior.GiveToOwnerConfig) {
			readGiveItems(c, j, &cfg.GiveItemsConfig)
			cfg.ReachDistance = c.Float(j, "reach_mob_distance", cfg.ReachDistance)
		}, behavior.NewGiveToOwner),
		"go_and_give_items_to_noteblock": configured(behavior.DefaultGiveToNoteblock, func(c *Context, j Object, cfg *behavior.GiveToNoteblockConfig) {
			readGiveItems(c, j, &cfg.GiveItemsConfig)
			cfg.ReachDistance = c.Float(j, "reach_block_distance", cfg.ReachDistance)
			cfg.ListenTime = c.Int(j, "listen_time", cfg.ListenTime)
		}, behavior.NewGiveToNoteblock),
		"stay_near_noteblock": configured(behavior.DefaultStayNearNoteblock, func(c *Context, j Object, cfg *behavior.StayNearNoteblockConfig) {
			cfg.Speed = c.Float(j, "speed", cfg.Speed)
			cfg.StartDistance = c.Float(j, "start_distance", cfg.StartDistance)
			cfg.StopDistance = c.Float(j, "stop_distance", cfg.StopDistance)
			cfg.ListenTime = c.Int(j, "listen_time", cfg.ListenTime)
		}, behavior.NewStayNearNoteblock),
		"transport_items": configured(behavior.DefaultTransportItems, func(c *Context, j Object, cfg *behavior.TransportItemsConfig) {
			if j.Has("source_container_types") {
				cfg.SourceContainerTypes = c.Descriptors(j, "source_container_types")
			}
			if j.Has("destination_container_types") {
				cfg.DestinationContainerTypes = c.Descriptors(j, "destination_container_types")
			}
			if j.Has("allowed_items") {
				cfg.AllowedItems = c.Descriptors(j, "allowed_items")
			}
			if j.Has("disallowed_items") {
				cfg.DisallowedItems = c.Descriptors(j, "disallowed_items")
			}
		}, behavior.NewTransportItems),

		// Blocks.
		"eat_block": configured(behavior.DefaultEatBlock, func(c *Context, j Object, cfg *behavior.EatBlockConfig) {
			if j.Has("success_chance") {
				if f, ok := number(j["success_chance"]); ok {
					cfg.SuccessChance, cfg.SuccessChanceValue = nil, float32(f)
				} else if eval, ok := c.Resolver.Molang(c.String(j, "success_chance", "")); ok {
					cfg.SuccessChance = eval
				} else {
					c.Warn(errUnknown("success_chance"))
				}
			}
			cfg.OnEat = c.Triggers(j, "on_eat")
		}, behavior.NewEatBlock),
		"eat_mob": configured(behavior.DefaultEatMob, func(c *Context, j Object, cfg *behavior.EatMobConfig) {
			if j.Has("eat_mob_sound") {
				cfg.EatMobSound = int32(c.Sound(j, "eat_mob_sound", 0))
			}
		}, behavior.NewEatMob),
		"lay_egg": configured(behavior.DefaultLayEgg, func(c *Context, j Object, cfg *behavior.LayEggConfig) {
			if j.Has("target_blocks") {
				cfg.TargetBlocks = c.Descriptors(j, "target_blocks")
			}
			cfg.MaterialsAbove = materials(c, j, "target_materials_above_block", cfg.MaterialsAbove)
			if j.Has("lay_egg_sound") {
				cfg.LaySound = int32(c.Sound(j, "lay_egg_sound", 0))
			}
			cfg.OnLay = c.Triggers(j, "on_lay")
		}, behavior.NewLayEgg),
		"take_block": configured(behavior.DefaultTakeBlock, func(c *Context, j Object, cfg *behavior.TakeBlockConfig) {
			cfg.Blocks = c.Descriptors(j, "blocks")
			cfg.CanTake = c.Filter(j, "can_take")
			cfg.OnTake = c.Triggers(j, "on_take")
		}, behavior.NewTakeBlock),
		"place_block": configured(behavior.DefaultPlaceBlock, func(c *Context, j Object, cfg *behavior.PlaceBlockConfig) {
			cfg.PlaceableCarriedBlocks = c.Descriptors(j, "placeable_carried_blocks")
			cfg.CanPlace = c.Filter(j, "can_place")
			cfg.OnPlace = c.Triggers(j, "on_place")
		}, behavior.NewPlaceBlock),
		"jump_to_block": configured(behavior.DefaultJumpToBlock, func(c *Context, j Object, cfg *behavior.JumpToBlockConfig) {
			cfg.Cooldown = c.Range(j, "cooldown_range", cfg.Cooldown)
			cfg.PreferredBlocks = c.Descriptors(j, "preferred_blocks")
			cfg.ForbiddenBlocks = c.Descriptors(j, "forbidden_blocks")
		}, behavior.NewJumpToBlock),
		"random_search_and_dig": configured(behavior.DefaultRandomSearchAndDig, func(c *Context, j Object, cfg *behavior.RandomSearchAndDigConfig) {
			if j.Has("target_blocks") {
				cfg.TargetBlocks = c.Descriptors(j, "target_blocks")
			}
			cfg.OnSearchingStart = c.Triggers(j, "on_searching_start")
			cfg.OnFailDuringSearching = c.Triggers(j, "on_fail_during_searching")
			cfg.OnDiggingStart = c.Triggers(j, "on_digging_start")
			cfg.OnItemFound = c.Triggers(j, "on_item_found")
			cfg.OnFailDuringDigging = c.Triggers(j, "on_fail_during_digging")
			cfg.OnSuccess = c.Triggers(j, "on_success")
		}, behavior.NewRandomSearchAndDig),
		"silverfish_merge_with_stone": simple(behavior.NewSilverfishMerge),
		"silverfish_wake_up_friends":  simple(behavior.NewSilverfishWake),
		"ocelot_sit_on_block":         configured(behavior.DefaultMoveTo, nil, behavior.NewOcelotSitOnBlock),
		"roll":                        configured(behavior.DefaultRoll, nil, behavior.NewRoll),
		"sneeze": configured(behavior.DefaultSneeze, func(c *Context, j Object, cfg *behavior.SneezeConfig) {
			cfg.Cooldown = c.Ticks(j, "cooldown_time", cfg.Cooldown)
			cfg.Sound = c.Sound(j, "sound", cfg.Sound)
			cfg.PrepareSound = c.Sound(j, "prepare_sound", cfg.PrepareSound)
			cfg.EntityTypes = c.FilterList(j, "entity_types")
		}, behavior.NewSneeze),
		"find_underwater_treasure": configured(behavior.DefaultFindUnderwaterTreasure, nil, behavior.NewFindUnderwaterTreasure),

		// Doors.
		"door_interact":      simple(behavior.NewDoorInteract),
		"break_door":         simple(behavior.NewBreakDoor),
		"open_door":          configured(behavior.DefaultOpenDoor, nil, behavior.NewOpenDoor),
		"restrict_open_door": simple(behavior.NewRestrictOpenDoor),

		// Squids and the ender dragon.
		"squid_idle":                  simple(behavior.NewSquidIdle),
		"squid_dive":                  simple(behavior.NewSquidDive),
		"squid_flee":                  simple(behavior.NewSquidFlee),
		"squid_out_of_water":          simple(behavior.NewSquidOutOfWater),
		"squid_move_away_from_ground": simple(behavior.NewSquidMoveAwayFromGround),
		"dragonholdingpattern":        configured(behavior.DefaultDragonHoldingPattern, nil, behavior.NewDragonHoldingPattern),
		"dragontakeoff":               configured(behavior.DefaultDragonTakeoff, nil, behavior.NewDragonTakeoff),
		"dragonlanding":               configured(behavior.DefaultDragonLanding, nil, behavior.NewDragonLanding),
		"dragonscanning":              configured(behavior.DefaultDragonScanning, nil, behavior.NewDragonScanning),
		"dragonflaming":               configured(behavior.DefaultDragonFlaming, nil, behavior.NewDragonFlaming),
		"dragonchargeplayer":          configured(behavior.DefaultDragonChargePlayer, nil, behavior.NewDragonChargePlayer),
		"dragonstrafeplayer":          configured(behavior.DefaultDragonStrafePlayer, nil, behavior.NewDragonStrafePlayer),
		"dragondeath":                 configured(behavior.DefaultDragonDeath, nil, behavior.NewDragonDeath),

		// Villages and raids.
		"move_to_poi": configured(behavior.DefaultMoveToPOI, nil, behavior.NewMoveToPOI),
		"sleep":       configured(behavior.DefaultSleep, nil, behavior.NewSleep),
		"hide":        configured(behavior.DefaultHide, nil, behavior.NewHide),
		"mingle":      configured(behavior.DefaultMingle, nil, behavior.NewMingle),
		"go_home": configured(behavior.DefaultGoHome, func(c *Context, j Object, cfg *behavior.GoHomeConfig) {
			cfg.OnHome = c.Triggers(j, "on_home")
			cfg.OnFailed = c.Triggers(j, "on_failed")
		}, behavior.NewGoHome),
		"move_to_village":        configured(behavior.DefaultMoveToVillage, nil, behavior.NewMoveToVillage),
		"stroll_towards_village": configured(behavior.DefaultStrollTowardsVillage, nil, behavior.NewStrollTowardsVillage),
		"move_through_village":   configured(behavior.DefaultMoveThroughVillage, nil, behavior.NewMoveThroughVillage),
		"explore_outskirts":      configured(behavior.DefaultExploreOutskirts, nil, behavior.NewExploreOutskirts),
		"move_indoors":           configured(behavior.DefaultMoveIndoors, nil, behavior.NewMoveIndoors),
		"move_outdoors":          configured(behavior.DefaultMoveOutdoors, nil, behavior.NewMoveOutdoors),
		"harvest_farm_block":     configured(behavior.DefaultHarvestFarmBlock, nil, behavior.NewHarvestFarmBlock),
		"fertilize_farm_block":   configured(behavior.DefaultFertilizeFarmBlock, nil, behavior.NewFertilizeFarmBlock),
		"raid_garden": configured(behavior.DefaultRaidGarden, func(c *Context, j Object, cfg *behavior.RaidGardenConfig) {
			if j.Has("blocks") {
				cfg.Blocks = c.Descriptors(j, "blocks")
			}
		}, behavior.NewRaidGarden),
		"inspect_bookshelf": configured(behavior.DefaultInspectBookshelf, nil, behavior.NewInspectBookshelf),
		"work": configured(behavior.DefaultWork, func(c *Context, j Object, cfg *behavior.WorkConfig) {
			cfg.OnArrival = c.Triggers(j, "on_arrival")
		}, behavior.NewWork),
		"work_composter": configured(behavior.DefaultWorkComposter, func(c *Context, j Object, cfg *behavior.WorkComposterConfig) {
			cfg.OnArrival = c.Triggers(j, "on_arrival")
		}, behavior.NewWorkComposter),
		"trade_interest": configured(behavior.DefaultTradeInterest, nil, behavior.NewTradeInterest),
		"trade_with_player": configured(behavior.DefaultTradeWithPlayer, func(c *Context, j Object, cfg *behavior.TradeWithPlayerConfig) {
			if f := c.Filter(j, "filters"); f != nil {
				cfg.Filters = f
			}
		}, behavior.NewTradeWithPlayer),
		"celebrate": configured(behavior.DefaultCelebrate, func(c *Context, j Object, cfg *behavior.CelebrateConfig) {
			cfg.CelebrationSound = c.Sound(j, "celebration_sound", cfg.CelebrationSound)
			cfg.OnCelebrationEndEvent = c.Triggers(j, "on_celebration_end_event")
		}, behavior.NewCelebrate),
		"celebrate_survive": configured(behavior.DefaultCelebrateSurvive, func(c *Context, j Object, cfg *behavior.CelebrateSurviveConfig) {
			cfg.OnCelebrationEndEvent = c.Triggers(j, "on_celebration_end_event")
		}, behavior.NewCelebrateSurvive),
		"offer_flower": configured(behavior.DefaultOfferFlower, nil, behavior.NewOfferFlower),
		"take_flower": configured(behavior.DefaultTakeFlower, func(c *Context, j Object, cfg *behavior.TakeFlowerConfig) {
			if f := c.Filter(j, "filters"); f != nil {
				cfg.Filters = f
			}
			cfg.OnTakeFlower = c.Triggers(j, "on_take_flower")
		}, behavior.NewTakeFlower),
		"skeleton_horse_trap": configured(behavior.DefaultSkeletonHorseTrap, nil, behavior.NewSkeletonHorseTrap),
	}
}
