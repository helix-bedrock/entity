package state

import (
	"math"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// The set_target modes of minecraft:looked_at, by BDS number (the parser).
const (
	LookedAtNever               = 0
	LookedAtOnceAndStop         = 1
	LookedAtOnceAndKeepScanning = 2
)

// The line_of_sight_obstruction_type values, by BDS number.
const (
	LineOfSightOutline            = 0
	LineOfSightCollision          = 1
	LineOfSightCollisionForCamera = 2
)

// LookAtLocation is one look_at_locations entry: an actor location of the
// owner (BDS ActorLocation number) raised by VerticalOffset.
type LookAtLocation struct {
	Location       int
	VerticalOffset float32
}

// LookedAtConfig is minecraft:looked_at as BDS 1.26.60 reads it (parser,
// defaults in the definition built by).
type LookedAtConfig struct {
	SearchRadius    float32
	SetTarget       int
	FindPlayersOnly bool
	// Cooldown is looked_at_cooldown in seconds, drawn after each look.
	Cooldown behavior.Range[float32]
	// FieldOfView is the full view cone of the looker in degrees.
	FieldOfView        float32
	ScaleFOVByDistance bool
	LineOfSight        int
	// LookAtLocations are the points of the owner a looker must see; BDS
	// takes the first whose line of sight is clear.
	LookAtLocations     []LookAtLocation
	MinLookedAtDuration float32
	// Filters run with the owner as self and the looker as other.
	Filters               *definition.Filter
	LookedAt, NotLookedAt *definition.Trigger
	// StareSound is the level sound event played at the owner's head when
	// it was looked at long enough (52, stare).
	StareSound int
}

// DefaultLookedAt is BDS's default: a 10 block search, a 26 degree cone
// that narrows with distance, collision line of sight, set_target
// once_and_stop_scanning, and the head as the point to look at.
//
// BDS's own default for an absent look_at_locations was not traced: with
// none, its system finds no looker at all, yet the enderman, which writes
// none, is angered by a look at its head. The head stands in.
func DefaultLookedAt() LookedAtConfig {
	return LookedAtConfig{
		SearchRadius: 10, SetTarget: LookedAtOnceAndStop, FieldOfView: 26, ScaleFOVByDistance: true,
		LineOfSight: LineOfSightCollision, LookAtLocations: []LookAtLocation{{Location: AttachHead}},
		StareSound: 52,
	}
}

// LookedAt is the running component (LookedAtSystem, per mob):
// a cooldown, then the nearest player (or first entity) in the search
// radius looking at the owner; after min_looked_at_duration it may become
// the target, looked_at_event fires, the stare sound plays and a new
// cooldown starts. With nobody looking, not_looked_at_event fires.
type LookedAt struct {
	cfg      LookedAtConfig
	cooldown int32
	looked   int32
	buf      []behavior.Actor
}

// NewLookedAt makes the component.
func NewLookedAt(c LookedAtConfig) *LookedAt { return &LookedAt{cfg: c} }

// Config returns the settings.
func (l *LookedAt) Config() LookedAtConfig { return l.cfg }

// SetConfig replaces the settings.
func (l *LookedAt) SetConfig(c LookedAtConfig) { l.cfg = c }

// Tick runs one tick of the system for the owner.
func (l *LookedAt) Tick(env *Env) {
	if l.cooldown > 0 {
		l.cooldown--
		return
	}
	m, w, c := env.Self, env.World, &l.cfg
	target := w.Actor(m.Target())
	if target != nil && c.SetTarget == LookedAtOnceAndStop {
		return
	}
	looker := l.find(env)
	if looker == nil {
		env.fire(c.NotLookedAt, env.tester(m, nil))
		l.looked = 0
		return
	}
	need := int32(c.MinLookedAtDuration * ticksPerSecond)
	if need > 0 && l.looked < need {
		l.looked++
	}
	if l.looked < need {
		return
	}
	if target == nil && c.SetTarget != LookedAtNever && m.CanAttack(looker, false) {
		m.SetTarget(looker.ID())
	}
	env.fire(c.LookedAt, env.tester(m, nil))
	if c.StareSound > 0 {
		m.PlaySoundAt(c.StareSound, m.AttachPos(AttachHead))
	}
	lo := c.Cooldown.Min
	l.cooldown = int32(((c.Cooldown.Max-lo)*w.Random().NextFloat() + lo) * ticksPerSecond)
	l.looked = 0
}

// find is the looker: the nearest player within the search radius (BDS
// compares actor positions, a player's at its eyes), or with
// find_players_only off the first entity in the owner's box grown by the
// radius.
func (l *LookedAt) find(env *Env) behavior.Actor {
	m, w, c := env.Self, env.World, &l.cfg
	if !c.FindPlayersOnly {
		b := m.Box()
		r := c.SearchRadius
		box := behavior.Box{
			Min: behavior.Vec3{X: b.Min.X - r, Y: b.Min.Y - r, Z: b.Min.Z - r},
			Max: behavior.Vec3{X: b.Max.X + r, Y: b.Max.Y + r, Z: b.Max.Z + r},
		}
		l.buf = w.Entities(l.buf[:0], m.ID(), box)
		for _, a := range l.buf {
			if l.looking(env, a) {
				return a
			}
		}
		return nil
	}
	best := float32(math.MaxFloat32)
	limit := c.SearchRadius * c.SearchRadius
	if c.SearchRadius < 0 {
		limit = math.MaxFloat32
	}
	var found behavior.Actor
	l.buf = w.Players(l.buf[:0])
	from := actorPos(m)
	for _, p := range l.buf {
		if !l.looking(env, p) {
			continue
		}
		if d := distSqr(actorPos(p), from); d < limit && d < best {
			best, found = d, p
		}
	}
	return found
}

// looking is the system's test of one looker: not in
// creative or spectator, one of the owner's points inside its view cone
// and in clear sight from its head, and the filters passed.
func (l *LookedAt) looking(env *Env, a behavior.Actor) bool {
	if a.Removed() || ignoredMode(env.World, a.GameMode()) {
		return false
	}
	m, c := env.Self, &l.cfg
	eye := a.AttachPos(AttachHead)
	pitch, yaw := a.HeadRotation()
	view := normalized(behavior.ViewVector(pitch, yaw))
	cosHalf := behavior.TableCos(c.FieldOfView * 0.5 * degToRad)
	for _, loc := range c.LookAtLocations {
		p := m.AttachPos(loc.Location)
		p.Y += loc.VerticalOffset
		d := behavior.Vec3{X: p.X - eye.X, Y: p.Y - eye.Y, Z: p.Z - eye.Z}
		dist := float32(math.Sqrt(float64(d.X*d.X + d.Y*d.Y + d.Z*d.Z)))
		if dist >= minDirLength {
			d = behavior.Vec3{X: d.X / dist, Y: d.Y / dist, Z: d.Z / dist}
		} else {
			d = behavior.Vec3{}
		}
		scale := float32(1)
		if c.ScaleFOVByDistance {
			scale = 1 / dist
		}
		if view.X*d.X+view.Y*d.Y+view.Z*d.Z <= 1-(1-cosHalf)*scale {
			continue
		}
		if ignoredMode(env.World, a.GameMode()) || !env.World.ClearLine(eye, p) {
			continue
		}
		return c.Filters.Eval(env.tester(m, a))
	}
	return false
}

const (
	degToRad     = math.Pi / 180
	minDirLength = 0.0001
)

// ignoredMode is a player in creative or spectator, the default game mode
// taken as the world's.
func ignoredMode(w behavior.World, mode int) bool {
	if mode < 0 {
		return false
	}
	if mode == behavior.GameModeDefault {
		mode = w.DefaultGameMode()
	}
	return mode == behavior.GameModeCreative || mode == behavior.GameModeSpectator
}

// actorPos is BDS's actor position: a player's is at its eyes.
func actorPos(a behavior.Actor) behavior.Vec3 {
	if a.IsPlayer() {
		return a.Eyes()
	}
	return a.Pos()
}

func normalized(v behavior.Vec3) behavior.Vec3 {
	l := float32(math.Sqrt(float64(v.X*v.X + v.Y*v.Y + v.Z*v.Z)))
	if l < minDirLength {
		return behavior.Vec3{}
	}
	return behavior.Vec3{X: v.X / l, Y: v.Y / l, Z: v.Z / l}
}
