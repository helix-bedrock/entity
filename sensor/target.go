package sensor

import (
	"math"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// TargetNearbyConfig is minecraft:target_nearby_sensor.
type TargetNearbyConfig struct {
	// InsideRange and OutsideRange are distances to the target.
	InsideRange, OutsideRange float32
	// MustSee needs the target in sight for on_inside_range and enables
	// on_vision_lost_inside_range.
	MustSee bool
	// OnInside, OnOutside and OnVisionLost are on_inside_range,
	// on_outside_range and on_vision_lost_inside_range.
	OnInside, OnOutside, OnVisionLost *definition.Trigger
}

// DefaultTargetNearby is BDS's sensor: inside 1, outside 5, must_see off
// (1.26.60 schema 0x70046C0).
func DefaultTargetNearby() TargetNearbyConfig {
	return TargetNearbyConfig{InsideRange: 1, OutsideRange: 5}
}

// TargetNearby fires events as the mob's target comes into or leaves
// range (1.21.3 _tickTargetNearbyComponent 0x140B1B2A0).
type TargetNearby struct {
	cfg                   TargetNearbyConfig
	seen, inside, outside bool
	dist                  float32
}

// NewTargetNearby makes a target sensor.
func NewTargetNearby(c TargetNearbyConfig) *TargetNearby {
	return &TargetNearby{cfg: c, dist: -1}
}

// Config returns the sensor's settings.
func (s *TargetNearby) Config() TargetNearbyConfig { return s.cfg }

// SetConfig changes the settings.
func (s *TargetNearby) SetConfig(c TargetNearbyConfig) { s.cfg = c }

// Distance is the distance measured last tick, -1 without a target.
func (s *TargetNearby) Distance() float32 { return s.dist }

// Tick measures the distance to the target and fires the events whose
// state changed.
func (s *TargetNearby) Tick(env *Env) {
	var target behavior.Actor
	if id := env.Self.Target(); id != behavior.NoID {
		target = env.World.Actor(id)
	}
	if target == nil || target.Removed() {
		s.seen, s.inside, s.outside, s.dist = false, false, false, -1
		return
	}
	d := Distance(env.Self.Box(), target.Box())
	inside := d < s.cfg.InsideRange
	outside := d > s.cfg.OutsideRange
	fireInside := inside && !s.inside
	fireOutside := outside && !s.outside
	fireLost, seen := false, false
	if s.cfg.MustSee {
		if c, ok := env.Self.(behavior.CanSeer); ok && env.Self.IsMob() {
			seen = c.CanSee(target)
		}
		fireInside = inside && seen && (!s.inside || !s.seen)
		fireLost = !outside && !seen && s.seen
	}
	ctx := env.tester(nil)
	if fireInside {
		env.fire(s.cfg.OnInside, ctx)
	}
	if fireOutside {
		env.fire(s.cfg.OnOutside, ctx)
	}
	if fireLost {
		env.fire(s.cfg.OnVisionLost, ctx)
	}
	s.seen, s.inside, s.outside, s.dist = seen, inside, outside, d
}

// Distance is BDS's distance from self's box to the target: AABB::distanceTo
// is called with the target's box where it takes a point, so it measures to
// the target box's minimum corner (1.21.3 0x1420D1560).
func Distance(self, target behavior.Box) float32 {
	p := target.Min
	c := behavior.Vec3{X: clamp(p.X, self.Min.X, self.Max.X), Y: clamp(p.Y, self.Min.Y, self.Max.Y), Z: clamp(p.Z, self.Min.Z, self.Max.Z)}
	dx, dy, dz := p.X-c.X, p.Y-c.Y, p.Z-c.Z
	return sqrt(dx*dx + dy*dy + dz*dz)
}

func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

func sqrt(v float32) float32 { return float32(math.Sqrt(float64(v))) }
