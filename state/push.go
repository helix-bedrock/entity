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
	// PushBall is the sulfur cube's kick. Its logic was not found and it
	// does not push yet.
	PushBall
)

// PushModeNames returns the JSON names of the push modes, by number.
func PushModeNames() []string {
	return []string{"none", "default", "legacy_boat", "legacy_minecart", "ball"}
}

// PushPreset is one preset of minecraft:pushable_by_entity (1.26.60
// PushableByEntityPreset, 0xE1D9D10). The kick and sound members belong
// to the ball mode and are held only.
type PushPreset struct {
	// Filter picks the preset, with self the pushed mob and other the
	// pusher; nil passes.
	Filter *definition.Filter
	Mode   PushMode
	// Strength is strength_multiplier: the push speed per tick at a
	// distance of one block or less.
	Strength float32
	// MinDistance is the distance below which nothing is pushed;
	// MaxDistance is held for the ball mode.
	MinDistance, MaxDistance float32
	// RequireCollisionOverlap is held; what it changes was not found.
	RequireCollisionOverlap bool
	// ScaleSelf scales the push on the pushed mob; ScaleOther scales that
	// push again for the push back on the pusher.
	ScaleSelf, ScaleOther float32

	KickSpeedScale, MinKickSpeed, MaxKickSpeed, VerticalKickMultiplier float32
	PlaySound                                                          bool
	PlaySoundCooldown, PlaySoundImpulseThreshold                       float32
}

// DefaultPushPreset is the preset a mob without presets uses. The 1.26.60
// defaults were not found; these reproduce 1.21.3's
// PushableComponent::push (0x14063C770) for an ordinary mob.
func DefaultPushPreset() PushPreset {
	return PushPreset{
		Mode: PushDefault, Strength: 0.05, MinDistance: 0.01,
		RequireCollisionOverlap: true, ScaleSelf: 1, ScaleOther: 1,
	}
}

// PushableByEntityConfig is minecraft:pushable_by_entity.
type PushableByEntityConfig struct {
	Presets []PushPreset
	// Reach is how far past its box, on x and z, a mob pushes others
	// (Mob::pushActors 0x141919060 grows the box by 0.2).
	Reach float32
	// MinecartAlign is how closely two minecarts must line up, as the
	// cosine between the push and the pushed cart's heading, to share
	// their speed; MinecartKeep is the part of their own speed each keeps.
	MinecartAlign, MinecartKeep float32
}

// DefaultPushableByEntity is the BDS default: no presets.
func DefaultPushableByEntity() PushableByEntityConfig {
	return PushableByEntityConfig{Reach: 0.2, MinecartAlign: 0.8, MinecartKeep: 0.2}
}

// PushableByEntity is a mob's minecraft:pushable_by_entity. Having one
// makes the mob pushable by others; its preset decides how.
type PushableByEntity struct {
	cfg PushableByEntityConfig
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
// pushed mob and other the pusher, else the default. How 1.26.60 picks
// was not found.
func (p *PushableByEntity) Preset(env *Env, self, pusher behavior.Actor) PushPreset {
	for i := range p.cfg.Presets {
		if p.cfg.Presets[i].Filter.Eval(env.tester(self, pusher)) {
			return p.cfg.Presets[i]
		}
	}
	return DefaultPushPreset()
}

// PushWorld is the engine's side of pushing: it finds each actor's
// component and moves actors other than the ticking mob.
type PushWorld interface {
	// PushableByEntity returns the actor's component, nil when it cannot
	// be pushed.
	PushableByEntity(a behavior.Actor) *PushableByEntity
	// PushThrough is the actor's push-through value (0 for none), which
	// weakens pushes on it.
	PushThrough(a behavior.Actor) float32
	// SameVehicle reports that a and b ride the same vehicle root.
	SameVehicle(a, b behavior.Actor) bool
	// PrevVehicle is the vehicle the actor last left, NoID for none.
	PrevVehicle(a behavior.Actor) behavior.ID
	// Controlled reports that a player steers the vehicle a.
	Controlled(a behavior.Actor) bool
	// PullToRide lets the steered vehicle take the mob as a passenger.
	PullToRide(vehicle, mob behavior.Actor)
	// Velocity and SetVelocity are an actor's motion.
	Velocity(a behavior.Actor) behavior.Vec3
	SetVelocity(a behavior.Actor, v behavior.Vec3)
	// Push adds v to the actor's motion, pushed by by. The engine also
	// sends a pushed player its motion and breaks hanging entities.
	Push(a, by behavior.Actor, v behavior.Vec3)
}

// PushActors is Mob::pushActors (1.21.3 0x141919060) for the ticking mob,
// whose own component is self (nil when it cannot be pushed). Players are
// skipped; they push in their own movement. Not ported: the actor
// categories 1024 and 16 BDS also checks, pullInEntity and the clearing
// of the previous vehicle.
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
	for _, o := range dst {
		if o == nil || o.IsPlayer() || o.ID() == prev || w.SameVehicle(env.Self, o) {
			continue
		}
		p := w.PushableByEntity(o)
		if p == nil {
			continue
		}
		if w.Controlled(o) {
			if !skipPush(w, o, env.Self) {
				w.PullToRide(o, env.Self)
			}
			continue
		}
		p.push(env, w, o, env.Self, selfOnly)
	}
	return dst
}

// skipPush is PushableComponent::skipPush (1.21.3 0x140641AC0) without
// its first check (an unread actor virtual).
func skipPush(w PushWorld, a, b behavior.Actor) bool {
	return a.Vehicle() != behavior.NoID || b.Vehicle() != behavior.NoID ||
		w.PrevVehicle(a) == b.ID() || w.PrevVehicle(b) == a.ID()
}

// push is PushableComponent::push (1.21.3 0x14063C770): it pushes the
// mob a away from the pusher b and, unless selfOnly, b away from a.
func (p *PushableByEntity) push(env *Env, w PushWorld, a, b behavior.Actor, selfOnly bool) {
	if skipPush(w, a, b) {
		return
	}
	pre := p.Preset(env, a, b)
	va, vb := p.vectors(env, w, pre, a, b)
	w.Push(a, b, va)
	if !selfOnly {
		w.Push(b, a, vb)
	}
}

// vectors returns the pushes on a and b for the preset's mode: default as
// push's own maths, legacy_boat as _calculatePushVectorBoat (0x1406242C0)
// and legacy_minecart as _calculatePushVectorMinecart (0x140624440).
func (p *PushableByEntity) vectors(env *Env, w PushWorld, pre PushPreset, a, b behavior.Actor) (va, vb behavior.Vec3) {
	pa, pb := a.Pos(), b.Pos()
	dx, dz := pb.X-pa.X, pb.Z-pa.Z
	k := (1 - w.PushThrough(a)) * pre.Strength * pre.ScaleSelf
	switch pre.Mode {
	case PushDefault:
		// BDS normalizes by the larger axis, not the length.
		m := max(abs(dx), abs(dz))
		if m < pre.MinDistance {
			return
		}
		f := float32(1 / math.Sqrt(float64(m)))
		g := min(f, 1)
		x, z := dx*f*g*k, dz*f*g*k
		return behavior.Vec3{X: -x, Z: -z}, behavior.Vec3{X: x * pre.ScaleOther, Z: z * pre.ScaleOther}
	case PushLegacyBoat, PushLegacyMinecart:
		d2 := dx*dx + dz*dz
		if d2 < pre.MinDistance*pre.MinDistance || pre.Mode == PushLegacyBoat && b.Flag(FlagSneaking) {
			return
		}
		f := float32(1 / math.Sqrt(float64(d2)))
		g := min(f, 1)
		x, z := dx*f*g*k, dz*f*g*k
		if pre.Mode == PushLegacyMinecart && p.bothMinecarts(env, w, a, b) {
			return p.minecarts(w, a, b, dx, dz, x, z)
		}
		return behavior.Vec3{X: -x, Z: -z}, behavior.Vec3{X: x * pre.ScaleOther, Z: z * pre.ScaleOther}
	}
	return
}

// bothMinecarts reports that the pusher is pushed as a minecart too. BDS
// tests the pusher's entity type; the pusher's own preset stands in.
func (p *PushableByEntity) bothMinecarts(env *Env, w PushWorld, a, b behavior.Actor) bool {
	q := w.PushableByEntity(b)
	return q != nil && q.Preset(env, b, a).Mode == PushLegacyMinecart
}

// minecarts is the minecart against minecart push: lined-up carts share
// their speed, others do not push.
func (p *PushableByEntity) minecarts(w PushWorld, a, b behavior.Actor, dx, dz, x, z float32) (va, vb behavior.Vec3) {
	l := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	if l == 0 {
		return
	}
	yaw := float64(a.BodyYaw()) * math.Pi / 180
	dot := (dx*float32(math.Cos(yaw)) + dz*float32(math.Sin(yaw))) / l
	if abs(dot) < p.cfg.MinecartAlign {
		return
	}
	ka := p.cfg.MinecartKeep
	sa, sb := w.Velocity(a), w.Velocity(b)
	ax, az := (sa.X+sb.X)*0.5, (sa.Z+sb.Z)*0.5
	w.SetVelocity(a, behavior.Vec3{X: sa.X * ka, Y: sa.Y, Z: sa.Z * ka})
	w.SetVelocity(b, behavior.Vec3{X: sb.X * ka, Y: sb.Y, Z: sb.Z * ka})
	return behavior.Vec3{X: ax - x, Z: az - z}, behavior.Vec3{X: ax + x, Z: az + z}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
