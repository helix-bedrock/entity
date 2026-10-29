package state

import (
	"math"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// PushMode is how a pushable_by_entity preset moves the mob (1.26.60
// PushMode enum, 0xDDAB610).
type PushMode uint8

// The push modes, by BDS number.
const (
	PushNone PushMode = iota
	PushDefault
	PushLegacyBoat
	PushLegacyMinecart
	// PushBall is the sulfur cube's kick.
	PushBall
)

// PushModeNames returns the JSON names of the push modes, by number.
func PushModeNames() []string {
	return []string{"none", "default", "legacy_boat", "legacy_minecart", "ball"}
}

// PushPreset is one preset of minecraft:pushable_by_entity (1.26.60
// PushableByEntityPreset, reflection 0xE1D9D10).
type PushPreset struct {
	// Filter picks the preset, with self the pushed mob and other the
	// pusher; nil passes.
	Filter *definition.Filter
	Mode   PushMode
	// Strength is strength_multiplier, the push speed per tick.
	Strength float32
	// MinDistance and MaxDistance bound the distance between the two
	// (on the larger axis for the default mode, else the length); the
	// pusher's half width is taken off before the MaxDistance test.
	MinDistance, MaxDistance float32
	// RequireCollisionOverlap pushes only when the two boxes overlap.
	RequireCollisionOverlap bool
	// ScaleSelf scales the push on the pushed mob and ScaleOther the push
	// back on the pusher.
	ScaleSelf, ScaleOther float32
	// The ball mode's kick: the pusher's speed times KickSpeedScale,
	// clamped to MinKickSpeed..MaxKickSpeed, and the upward part of the
	// kick on a mob on the ground.
	KickSpeedScale, MinKickSpeed, MaxKickSpeed, VerticalKickMultiplier float32
	// PlaySound plays the push sound when the push on the mob is at least
	// PlaySoundImpulseThreshold, at most once per PlaySoundCooldown ticks.
	PlaySound                 bool
	PlaySoundCooldown         int64
	PlaySoundImpulseThreshold float32
}

// DefaultPushPreset is the 1.26.60 default preset (static 0xFD47DE8,
// initialized in 0x5B012D0).
func DefaultPushPreset() PushPreset {
	return PushPreset{
		Mode: PushDefault, Strength: 0.05, MinDistance: 0.1, MaxDistance: math.MaxFloat32,
		ScaleSelf: 1, ScaleOther: 1,
		KickSpeedScale: 2, MaxKickSpeed: 0.5, VerticalKickMultiplier: 0.3,
		PlaySoundCooldown: 4, PlaySoundImpulseThreshold: 0.2,
	}
}

// PushableByEntityConfig is minecraft:pushable_by_entity.
type PushableByEntityConfig struct {
	Presets []PushPreset
	// Reach is how far past its box, on x and z, a mob pushes others
	// (Mob::pushActors, 1.26.60 0xBFEE7F0, grows the box by 0.2).
	Reach float32
	// MinecartAlign is how closely two minecarts must line up, as the
	// cosine between the push and the pushed cart's heading, to share
	// their speed; MinecartKeep is the part of their own speed each keeps.
	MinecartAlign, MinecartKeep float32
	// MinBallLength is the shortest distance the ball mode takes a
	// direction from.
	MinBallLength float32
	// SoundEvent is the level sound event of the push sound.
	SoundEvent int
}

// DefaultPushableByEntity is the BDS default: no presets.
func DefaultPushableByEntity() PushableByEntityConfig {
	return PushableByEntityConfig{
		Reach: 0.2, MinecartAlign: 0.8, MinecartKeep: 0.2, MinBallLength: 1e-4, SoundEvent: 599,
	}
}

// PushableByEntity is a mob's minecraft:pushable_by_entity. Having one
// makes the mob pushable by others; its preset decides how.
type PushableByEntity struct {
	cfg PushableByEntityConfig
	// lastSound is the tick the push sound last played, if played.
	lastSound uint64
	played    bool
}

// NewPushableByEntity makes the component.
func NewPushableByEntity(c PushableByEntityConfig) *PushableByEntity {
	return &PushableByEntity{cfg: c}
}

// Config returns the settings.
func (p *PushableByEntity) Config() PushableByEntityConfig { return p.cfg }

// SetConfig replaces the settings.
func (p *PushableByEntity) SetConfig(c PushableByEntityConfig) { p.cfg = c }

// Preset returns the first preset whose filter passes with self the
// pushed mob and other the pusher, else the default (1.26.60 0x75178D0).
// BDS falls back to a built-in boat or minecart preset for those entity
// types behind a version toggle that was not identified; that fallback
// is not ported, and vanilla boats and minecarts list their presets.
func (p *PushableByEntity) Preset(env *Env, self, pusher behavior.Actor) PushPreset {
	for i := range p.cfg.Presets {
		if p.cfg.Presets[i].Filter.Eval(env.tester(self, pusher)) {
			return p.cfg.Presets[i]
		}
	}
	return DefaultPushPreset()
}

// PushWorld is the engine's side of pushing: it finds each actor's
// component and moves actors other than the ticking mob. PushActors
// changes other actors' motion and their components' sound timers, so the
// engine runs it where no other mob's tick runs at the same time.
type PushWorld interface {
	// PushableByEntity returns the actor's component, nil when it cannot
	// be pushed or is in actor category 1024, which BDS never pushes.
	PushableByEntity(a behavior.Actor) *PushableByEntity
	// PushThrough is the actor's push-through value (0 for none), which
	// weakens default, boat and minecart pushes on it.
	PushThrough(a behavior.Actor) float32
	// SameVehicle reports that b is a's vehicle root or rides it.
	SameVehicle(a, b behavior.Actor) bool
	// PrevVehicle is the vehicle the actor last left while it is not
	// removed, NoID for none; ClearPrevVehicle forgets it.
	PrevVehicle(a behavior.Actor) behavior.ID
	ClearPrevVehicle(a behavior.Actor)
	// Controlled reports that a player steers the vehicle a
	// (VehicleInputIntentComponent).
	Controlled(a behavior.Actor) bool
	// PullIn lets vehicle a take mob b as a passenger, or b take a, and
	// stops the navigation of a mob pulled in (1.26.60 0x7517B50).
	PullIn(a, b behavior.Actor)
	// IsMinecart reports that the actor is a minecart type.
	IsMinecart(a behavior.Actor) bool
	// OnGround reports that the actor stands on the ground.
	OnGround(a behavior.Actor) bool
	// Velocity and SetVelocity are an actor's motion. The ball mode reads
	// the pusher's motion from a component that was not identified; the
	// velocity stands in.
	Velocity(a behavior.Actor) behavior.Vec3
	SetVelocity(a behavior.Actor, v behavior.Vec3)
	// Push adds v to the actor's motion, pushed by by. The engine also
	// sends a pushed player its motion and breaks a painting or leash knot
	// pushed at all, as BDS's push apply (0x7518430) does.
	Push(a, by behavior.Actor, v behavior.Vec3)
	// PlaySound plays a level sound event at p.
	PlaySound(a behavior.Actor, event int, p behavior.Vec3)
}

// PushActors is Mob::pushActors (1.26.60 0xBFEE7F0) for the ticking mob,
// whose own component is self (nil when it cannot be pushed). Players are
// skipped; they push in their own movement. dst is scratch space for the
// actor query and is returned for reuse.
func PushActors(env *Env, self *PushableByEntity, w PushWorld, dst []behavior.Actor) []behavior.Actor {
	if env.Self == nil || env.World == nil || w == nil {
		return dst
	}
	c := DefaultPushableByEntity()
	if self != nil {
		c = self.cfg
	}
	box := env.Self.Box()
	box.Min.X, box.Min.Z = box.Min.X-c.Reach, box.Min.Z-c.Reach
	box.Max.X, box.Max.Z = box.Max.X+c.Reach, box.Max.Z+c.Reach
	dst = env.World.Entities(dst[:0], env.Self.ID(), box)
	selfOnly := self == nil || w.Controlled(env.Self)
	prev := w.PrevVehicle(env.Self)
	sawPrev := false
	for _, o := range dst {
		if o == nil || o.IsPlayer() {
			continue
		}
		if o.ID() == prev {
			sawPrev = true
			continue
		}
		if w.SameVehicle(env.Self, o) {
			continue
		}
		p := w.PushableByEntity(o)
		if p == nil {
			continue
		}
		if w.Controlled(o) {
			if !skipPush(w, o, env.Self) {
				w.PullIn(o, env.Self)
			}
			continue
		}
		p.push(env, w, o, env.Self, selfOnly)
	}
	// A mob forgets the vehicle it left once it no longer touches it.
	if prev != behavior.NoID && !sawPrev {
		w.ClearPrevVehicle(env.Self)
	}
	return dst
}

// skipPush is 1.26.60 0x7517560 without its first check (a level
// virtual, believed to be the client side test): either rides, or one
// last left the other.
func skipPush(w PushWorld, a, b behavior.Actor) bool {
	return a.Vehicle() != behavior.NoID || b.Vehicle() != behavior.NoID ||
		w.PrevVehicle(a) == b.ID() || w.PrevVehicle(b) == a.ID()
}

// push is the 1.26.60 push (0x7516F70): it pushes the mob a away from the
// pusher b and, unless selfOnly, b away from a when b's own preset pushes.
func (p *PushableByEntity) push(env *Env, w PushWorld, a, b behavior.Actor, selfOnly bool) {
	if skipPush(w, a, b) {
		return
	}
	pre := p.Preset(env, a, b)
	if pre.Mode == PushNone {
		return
	}
	w.PullIn(a, b)
	pa, pb := a.Pos(), b.Pos()
	dx, dz := pb.X-pa.X, pb.Z-pa.Z
	var dist float32
	if pre.Mode == PushDefault {
		dist = max(abs(dx), abs(dz))
	} else {
		dist = float32(math.Sqrt(float64(dx*dx + dz*dz)))
	}
	bb := b.Box()
	if dist < pre.MinDistance || dist-(bb.Max.X-bb.Min.X)*0.5 > pre.MaxDistance {
		return
	}
	if pre.RequireCollisionOverlap && !intersects(bb, a.Box()) {
		return
	}
	va, vb := p.vectors(w, pre, a, b, dx, dz, dist)
	w.Push(a, b, va)
	if !selfOnly {
		if q := w.PushableByEntity(b); q != nil && q.Preset(env, b, a).Mode != PushNone {
			w.Push(b, a, vb)
		}
	}
	if pre.PlaySound {
		p.sound(env, w, pre, a, va)
	}
}

// sound plays the push sound at the middle of a's box when the push is
// strong enough and the cooldown is over.
func (p *PushableByEntity) sound(env *Env, w PushWorld, pre PushPreset, a behavior.Actor, va behavior.Vec3) {
	now := env.World.CurrentTick()
	if p.played && int64(now-p.lastSound) < pre.PlaySoundCooldown {
		return
	}
	t := pre.PlaySoundImpulseThreshold
	if t*t > va.X*va.X+va.Y*va.Y+va.Z*va.Z {
		return
	}
	box, pos := a.Box(), a.Pos()
	w.PlaySound(a, p.cfg.SoundEvent, behavior.Vec3{X: pos.X, Y: box.Min.Y + (box.Max.Y-box.Min.Y)*0.5, Z: pos.Z})
	p.lastSound, p.played = now, true
}

// vectors returns the pushes on a and b for the preset's mode: default
// (0x7518320), legacy_boat (0x7517D90), legacy_minecart (0x7517E90) and
// ball (0x75180E0). dist is the mode's distance, never below MinDistance.
func (p *PushableByEntity) vectors(w PushWorld, pre PushPreset, a, b behavior.Actor, dx, dz, dist float32) (va, vb behavior.Vec3) {
	if dist <= 0 && pre.Mode != PushBall {
		return
	}
	keep := 1 - w.PushThrough(a)
	switch pre.Mode {
	case PushDefault:
		// BDS normalizes by the square root of the larger axis, not the
		// length, and leaves ScaleSelf out of the push back.
		f := float32(1 / math.Sqrt(float64(dist)))
		g := min(f, 1) * pre.Strength
		x, z := dx*f*g*keep, dz*f*g*keep
		return behavior.Vec3{X: -x * pre.ScaleSelf, Z: -z * pre.ScaleSelf},
			behavior.Vec3{X: x * pre.ScaleOther, Z: z * pre.ScaleOther}
	case PushLegacyBoat, PushLegacyMinecart:
		if pre.Mode == PushLegacyBoat && b.Flag(FlagSneaking) {
			return
		}
		f := 1 / dist
		g := min(f, 1) * pre.Strength * keep * pre.ScaleSelf
		x, z := dx*f*g, dz*f*g
		if pre.Mode == PushLegacyMinecart && w.IsMinecart(b) {
			return p.minecarts(w, a, b, dx*f, dz*f, x, z)
		}
		return behavior.Vec3{X: -x, Z: -z}, behavior.Vec3{X: x * pre.ScaleOther, Z: z * pre.ScaleOther}
	case PushBall:
		return p.ball(w, pre, a, b, dx, dz, dist)
	}
	return
}

// minecarts is the minecart against minecart push: lined-up carts share
// their speed, others do not push. nx, nz is the unit push direction.
func (p *PushableByEntity) minecarts(w PushWorld, a, b behavior.Actor, nx, nz, x, z float32) (va, vb behavior.Vec3) {
	// BDS reads the heading from its sine table.
	i := float32(float64(a.BodyYaw())*math.Pi/180) * 10430.378
	cos, sin := tableSin(int32(i+16384)), tableSin(int32(i))
	if l := float32(math.Sqrt(float64(cos*cos + sin*sin))); l >= 1e-4 {
		cos, sin = cos/l, sin/l
	} else {
		cos, sin = 0, 0
	}
	if abs(nx*cos+nz*sin) < p.cfg.MinecartAlign {
		return
	}
	k := p.cfg.MinecartKeep
	sa, sb := w.Velocity(a), w.Velocity(b)
	ax, az := (sa.X+sb.X)*0.5, (sa.Z+sb.Z)*0.5
	w.SetVelocity(a, behavior.Vec3{X: sa.X * k, Y: sa.Y, Z: sa.Z * k})
	w.SetVelocity(b, behavior.Vec3{X: sb.X * k, Y: sb.Y, Z: sb.Z * k})
	return behavior.Vec3{X: ax - x, Z: az - z}, behavior.Vec3{X: ax + x, Z: az + z}
}

// ball is the sulfur cube's kick: the pusher's speed sets the kick, which
// sends the mob straight away from the pusher, up too when it is on the
// ground, and the pusher back. l is the distance between the two.
func (p *PushableByEntity) ball(w PushWorld, pre PushPreset, a, b behavior.Actor, dx, dz, l float32) (va, vb behavior.Vec3) {
	s := w.Velocity(b)
	speed := float32(math.Sqrt(float64(s.X*s.X+s.Y*s.Y+s.Z*s.Z))) * pre.KickSpeedScale
	kick := min(max(speed, pre.MinKickSpeed), pre.MaxKickSpeed)
	var nx, nz float32
	if l >= p.cfg.MinBallLength {
		nx, nz = dx/l, dz/l
	}
	var up float32
	if w.OnGround(a) {
		up = pre.VerticalKickMultiplier
	}
	ks, ko := kick*pre.ScaleSelf, kick*pre.ScaleOther
	return behavior.Vec3{X: -nx * ks, Y: up * ks, Z: -nz * ks}, behavior.Vec3{X: nx * ko, Z: nz * ko}
}

// tableSin is mce::Math's sine table lookup: 65536 steps per turn.
func tableSin(i int32) float32 {
	return float32(math.Sin(float64(uint16(i)) * 2 * math.Pi / 65536))
}

// intersects is AABB::intersects: the boxes overlap with some volume.
func intersects(a, b behavior.Box) bool {
	return a.Max.X > b.Min.X && a.Min.X < b.Max.X &&
		a.Max.Y > b.Min.Y && a.Min.Y < b.Max.Y &&
		a.Max.Z > b.Min.Z && a.Min.Z < b.Max.Z
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
