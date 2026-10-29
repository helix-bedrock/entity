package state

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
}

// TickInput is the engine's answers a state tick needs beyond Env.
type TickInput struct {
	// Chunks may be nil, which turns off the despawn rules that need it.
	Chunks Chunks
	// Invulnerable is a player in creative or with the invulnerable
	// ability, who always breathes.
	Invulnerable bool
}

// Tick runs the ticking components once: ageable, breedable, tamemount,
// angry, interact, transformation, hurt_on_condition, breathable,
// burns_in_daylight and despawn. BDS runs each from its own system; this
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
	if s.Despawn != nil {
		s.Despawn.Tick(env, in.Chunks)
	}
	return transform
}
