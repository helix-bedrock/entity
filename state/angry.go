package state

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// AngryConfig is minecraft:angry.
type AngryConfig struct {
	// Duration is the time angry in seconds, -1 for no limit;
	// DurationDelta varies it by up to that many seconds either way.
	Duration, DurationDelta int32
	// BroadcastAnger alerts friends when the mob first gets angry;
	// OnAttack and OnBeingAttacked when it has just hurt or been hurt by
	// a mob. WhenDying is read but not used: it is new since 1.21.3 and
	// its code was not read.
	BroadcastAnger, OnAttack, OnBeingAttacked, WhenDying bool
	BroadcastRange                                       int32
	// BroadcastTargets are the families alerted; empty alerts mobs of the
	// mob's own type.
	BroadcastTargets []string
	// BroadcastFilters test each friend, with the friend as self and the
	// angry mob as other.
	BroadcastFilters *definition.Filter
	// Filters test the target, with the mob as self and the target as
	// other.
	Filters   *definition.Filter
	CalmEvent *definition.Trigger
	// Sound plays every SoundInterval seconds while angry,
	// behavior.NoSound for none.
	Sound         int
	SoundInterval behavior.Range[float32]
	// LastHurtWindow is how recent, in ticks, a hit must be to broadcast
	// on attack or on being attacked.
	LastHurtWindow int32
}

// DefaultAngry is BDS's angry (1.26.60 schema 0x735EE80: duration 25,
// duration_delta 0, broadcast_anger, _on_attack and _on_being_attacked
// false, broadcast_anger_when_dying true, broadcast_range 20; the 2-tick
// window from 1.21.3 AngrySystem::_tickComponent 0x1406C3BB0).
func DefaultAngry() AngryConfig {
	return AngryConfig{Duration: 25, WhenDying: true, BroadcastRange: 20, Sound: behavior.NoSound, LastHurtWindow: 2}
}

// Alerted is a friend the angry mob alerts. The engine's mob implements it
// to fire its minecraft:on_friendly_anger component.
type Alerted interface {
	FriendlyAnger(angry behavior.Actor)
}

// Angry is minecraft:angry: the mob stays angry at its target until the
// timer runs out, alerting friends (1.21.3 AngrySystem::_tickComponent
// 0x1406C3BB0, AngryComponent::canGetAngry 0x140564940,
// AngryDefinition::initialize 0x1405699E0, alertFriendsToAnger
// 0x1406C6010, calmDown 0x1406C7EC0, setNextSoundEventTick 0x14056EE70).
type Angry struct {
	cfg       AngryConfig
	timer     int32
	ticked    bool
	nextSound uint64
	buf       []behavior.Actor
}

// NewAngry makes the component and starts its timer.
func NewAngry(c AngryConfig, r behavior.Random) *Angry {
	a := &Angry{cfg: c}
	a.Restart(r)
	return a
}

// Config returns the settings.
func (a *Angry) Config() AngryConfig { return a.cfg }

// SetConfig changes the settings; the timer is kept.
func (a *Angry) SetConfig(c AngryConfig) { a.cfg = c }

// Timer is the ticks left angry; below 0 never runs out.
func (a *Angry) Timer() int32 { return a.timer }

// Restart draws the timer again: duration plus a delta in
// [-duration_delta, duration_delta], in ticks.
func (a *Angry) Restart(r behavior.Random) {
	d := a.cfg.DurationDelta
	v := -d
	if r != nil && v < d+1 {
		if n := 2*d + 1; n != 0 {
			v += r.NextInt(n)
		}
	}
	a.timer = ticksPerSecond * (v + a.cfg.Duration)
}

// Tick keeps the mob angry while it can be angry at its target, alerting
// friends, counting the timer down and playing the angry sound; otherwise
// it calms down.
func (a *Angry) Tick(env *Env) {
	m := env.Self
	w := env.World
	target := w.Actor(m.Target())
	if !a.CanGetAngry(env, m, m, target, false) {
		a.calmDown(env)
		return
	}
	m.SetFlag(behavior.FlagAngry, true)
	now := int64(w.CurrentTick())
	_, hurtMob := m.LastHurtMob()
	_, hurtBy := m.LastHurtBy()
	window := int64(a.cfg.LastHurtWindow)
	if !a.ticked && a.cfg.BroadcastAnger ||
		a.cfg.OnAttack && int64(hurtMob) > now-window ||
		a.cfg.OnBeingAttacked && int64(hurtBy) > now-window {
		a.alert(env)
	}
	if a.timer < 0 {
		return
	}
	a.timer--
	if a.timer <= 0 {
		a.calmDown(env)
	}
	if a.nextSound < uint64(now) {
		if a.cfg.Sound != behavior.NoSound {
			m.PlaySoundAt(a.cfg.Sound, m.AttachPos(AttachHead))
		}
		a.nextSound = uint64(now) + uint64(max(int32(a.soundDelay(w.Random())*ticksPerSecond), 0))
	}
}

// soundDelay draws from sound_interval; RandomValueBounds::getFloat was
// not read and is taken as a uniform draw.
func (a *Angry) soundDelay(r behavior.Random) float32 {
	iv := a.cfg.SoundInterval
	return iv.Min + r.NextFloat()*(iv.Max-iv.Min)
}

// calmDown clears the angry flag and the target and fires calm_event.
func (a *Angry) calmDown(env *Env) {
	m := env.Self
	m.SetFlag(behavior.FlagAngry, false)
	m.SetTarget(behavior.NoID)
	env.fire(a.cfg.CalmEvent, env.tester(m, nil))
}

// CanGetAngry is canGetAngry: whether checker may be angry at target on
// behalf of self (the mob with this component). With no target the mob
// stays angry only when filters is empty. forced is set for alerted
// friends, who skip the player checks.
func (a *Angry) CanGetAngry(env *Env, self Mob, checker behavior.Mob, target behavior.Actor, forced bool) bool {
	if target == nil {
		return emptyFilter(a.cfg.Filters)
	}
	if target.Removed() || checker == nil || checker.Removed() || target.ID() == checker.ID() {
		return false
	}
	if !a.cfg.Filters.Eval(env.tester(self, target)) || !target.Alive() {
		return false
	}
	if c, ok := checker.(Mob); ok && !c.CanAttack(target, forced) {
		return false
	}
	if checker.Owner() == behavior.NoID {
		if target.IsPlayer() && !forced && (target.Invulnerable() || env.World.Peaceful()) {
			return false
		}
		return true
	}
	return target.Owner() != checker.Owner() && target.Owner() != self.Owner()
}

// alert sets the mob's friends on its target: mobs within broadcast_range
// of the broadcast families (or of the mob's own type) that have no
// target, could be angry at the attacker, are not tame and pass
// broadcast_filters.
func (a *Angry) alert(env *Env) {
	m := env.Self
	w := env.World
	a.ticked = true
	target := w.Actor(m.Target())
	if target == nil {
		hurtBy, _ := m.LastHurtBy()
		m.SetTarget(hurtBy)
		if target = w.Actor(m.Target()); target == nil {
			return
		}
	}
	r := float32(a.cfg.BroadcastRange)
	b := m.Box()
	b.Min = behavior.Vec3{X: b.Min.X - r, Y: b.Min.Y - r, Z: b.Min.Z - r}
	b.Max = behavior.Vec3{X: b.Max.X + r, Y: b.Max.Y + r, Z: b.Max.Z + r}
	a.buf = w.Entities(a.buf[:0], m.ID(), b)
	defer clear(a.buf)
	attackerID, _ := m.LastHurtBy()
	attacker := w.Actor(attackerID)
	if target.IsMob() {
		attacker = target
	}
	for _, e := range a.buf {
		f, ok := e.(behavior.Mob)
		if !ok || !e.IsMob() {
			continue
		}
		if len(a.cfg.BroadcastTargets) == 0 {
			if e.Type() != m.Type() {
				continue
			}
		} else if !a.hasFamily(env, e) {
			continue
		}
		if f.Target() != behavior.NoID || !a.CanGetAngry(env, m, f, attacker, true) || f.Flag(behavior.FlagTamed) {
			continue
		}
		if !a.cfg.BroadcastFilters.Eval(env.tester(e, m)) {
			continue
		}
		f.SetTarget(target.ID())
		if al, ok := e.(Alerted); ok {
			al.FriendlyAnger(m)
		}
	}
}

func (a *Angry) hasFamily(env *Env, e behavior.Actor) bool {
	if env.HasFamily == nil {
		return false
	}
	for _, fam := range a.cfg.BroadcastTargets {
		if env.HasFamily(e, fam) {
			return true
		}
	}
	return false
}
