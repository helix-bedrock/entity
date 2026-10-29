package state

import "github.com/helix-bedrock/entity/behavior"

// Set is a mob's state components; a nil member is a component the mob
// does not have.
type Set struct {
	Ageable         *Ageable
	Breedable       *Breedable
	Tameable        *Tameable
	Tamemount       *Tamemount
	Angry           *Angry
	Healable        *Healable
	Despawn         *Despawn
	Interact        *Interact
	Giveable        *Giveable
	Nameable        *Nameable
	Rideable        *Rideable
	AddRider        *AddRiderConfig
	Transformation  *Transformation
	HurtOnCondition *HurtOnCondition
	Breathable      *Breathable
	BurnsInDaylight *BurnsInDaylight
	Leashable       *Leashable
	LeashableTo     *LeashableToConfig
	// Attributes are the attribute components the mob lists, by name.
	Attributes       map[string]AttributeConfig
	Attack           *AttackConfig
	Equipment        *EquipmentConfig
	Equippable       *Equippable
	ExperienceReward *ExperienceRewardConfig
	Scale            *ScaleConfig
	AttackCooldown   *AttackCooldown

	// The physics-style components. The marker components that only set
	// a flag are in PhysicsFlagComponents, not here.
	Physics              *PhysicsConfig
	CollisionBox         *CollisionBoxConfig
	CustomHitTest        *CustomHitTestConfig
	PushableByEntity     *PushableByEntity
	PushableByBlock      bool
	Buoyant              *Buoyant
	RailMovement         *RailMovementConfig
	RailSensor           *RailSensor
	FrictionModifier     *ValueConfig
	AirDragModifier      *ValueConfig
	Bounciness           *ValueConfig
	MovementSoundOffset  *ValueConfig
	WaterMovement        *WaterMovementConfig
	VariableMaxAutoStep  *VariableMaxAutoStepConfig
	JumpStatic           *JumpStaticConfig
	JumpDynamic          *JumpDynamicConfig
	SlowdownImmunity     *SlowdownImmunityConfig
	BlockClimber         bool
	CanStandOnPowderSnow bool
	InputGroundControl   bool
}

// TickInput is the engine's answers a state tick needs beyond Env.
type TickInput struct {
	// Chunks may be nil, which turns off the despawn rules that need it.
	Chunks Chunks
	// Invulnerable is a player in creative or with the invulnerable
	// ability, who always breathes.
	Invulnerable bool
	// LeashChunks may be nil: a leashed mob whose holder is missing then
	// waits and never drops the leash.
	LeashChunks LeashChunks
	// Rails may be nil, which turns off rail_sensor.
	Rails Rails
}

// Tick runs the ticking components once: ageable, breedable, tamemount,
// angry, interact, transformation, hurt_on_condition, breathable,
// burns_in_daylight, leashable, attack_cooldown, rail_sensor and
// despawn. BDS runs each from its own system; this
// order is not BDS's system order, which was not read. It reports that
// the mob's transformation is due; the engine then transforms it.
func (s *Set) Tick(env *Env, in TickInput) (transform bool) {
	if s == nil || env.Self == nil || env.World == nil {
		return false
	}
	if s.Ageable != nil {
		s.Ageable.Tick(env)
	}
	if s.Breedable != nil {
		s.Breedable.Tick(env)
	}
	if s.Tamemount != nil {
		s.Tamemount.Tick(env)
	}
	if s.Angry != nil {
		s.Angry.Tick(env)
	}
	if s.Interact != nil {
		s.Interact.Tick()
	}
	if s.Transformation != nil {
		transform = s.Transformation.Tick(env)
	}
	if s.HurtOnCondition != nil {
		s.HurtOnCondition.Tick(env)
	}
	if s.Breathable != nil {
		s.Breathable.Tick(env, in.Invulnerable)
	}
	if s.BurnsInDaylight != nil {
		s.BurnsInDaylight.Tick(env)
	}
	if s.Leashable != nil {
		s.Leashable.Tick(env, in.LeashChunks)
	}
	if s.AttackCooldown != nil {
		s.AttackCooldown.Tick(env)
	}
	if s.RailSensor != nil {
		s.RailSensor.Tick(env, in.Rails)
	}
	if s.Despawn != nil {
		s.Despawn.Tick(env, in.Chunks)
	}
	return transform
}

// BeforeMove runs the components BDS runs as movement systems before the
// mob moves: buoyant. l may be nil, which keeps the mob from floating.
func (s *Set) BeforeMove(env *Env, l Liquids) {
	if s != nil && s.Buoyant != nil {
		s.Buoyant.Tick(env, l)
	}
}

// AfterMove pushes the actors around the mob, as Mob::pushActors does
// after the mob's travel. dst is scratch space for the actor query and is
// returned for reuse.
func (s *Set) AfterMove(env *Env, w PushWorld, dst []behavior.Actor) []behavior.Actor {
	if s == nil {
		return PushActors(env, nil, w, dst)
	}
	return PushActors(env, s.PushableByEntity, w, dst)
}
