package state

// Set is a mob's state components; a nil member is a component the mob
// does not have.
type Set struct {
	Ageable   *Ageable
	Breedable *Breedable
	Tameable  *Tameable
	Tamemount *Tamemount
	Angry     *Angry
	Healable  *Healable
	Despawn   *Despawn
}

// Tick runs the ticking components once: ageable, breedable, tamemount,
// angry and despawn. BDS runs each from its own system; this order is not
// BDS's system order, which was not read. chunks may be nil, which turns
// off the despawn rules that need it.
func (s *Set) Tick(env *Env, chunks Chunks) {
	if s == nil || env.Self == nil || env.World == nil {
		return
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
	if s.Despawn != nil {
		s.Despawn.Tick(env, chunks)
	}
}
