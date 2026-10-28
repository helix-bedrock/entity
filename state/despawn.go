package state

import (
	"math"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// DespawnConfig is minecraft:despawn.
type DespawnConfig struct {
	// Filters, when not empty, replace the standard rules: the mob
	// despawns when they pass.
	Filters *definition.Filter
	// RemoveChildEntities also removes the mobs leashed to it; the engine
	// does that when it despawns the mob.
	RemoveChildEntities bool
	// FromDistance turns on the distance rule (the despawn_from_distance
	// member is present); MinDistance and MaxDistance are its distances.
	FromDistance             bool
	MinDistance, MaxDistance float32
	// FromInactivity keeps a mob that acted within InactivityTimer seconds.
	FromInactivity  bool
	InactivityTimer int32
	// FromChance despawns one time in RandomChance.
	FromChance   bool
	RandomChance int32
	// FromSimulationEdge despawns a mob at the edge of the ticked area.
	FromSimulationEdge bool
	// EdgeTickRange is the chunk tick range at or below which a mob past
	// MaxDistance despawns.
	EdgeTickRange int32
}

// DefaultDespawn is BDS's despawn (1.26.60 schema 0x723A360:
// remove_child_entities false, min_range_inactivity_timer 30,
// despawn_from_inactivity true, min_range_random_chance 800,
// despawn_from_chance true, despawn_from_simulation_edge true,
// min_distance 32, max_distance 128; the tick range 4 from 1.21.3
// _tryStandardDespawnRules 0x1405A4630). The distance rule is off until
// despawn_from_distance is given.
func DefaultDespawn() DespawnConfig {
	return DespawnConfig{
		MinDistance: 32, MaxDistance: 128, FromInactivity: true, InactivityTimer: 30,
		FromChance: true, RandomChance: 800, FromSimulationEdge: true, EdgeTickRange: 4,
	}
}

// Chunks is the engine's answers about the chunks around a mob.
type Chunks interface {
	// TickRange is the level's chunk tick range.
	TickRange() int32
	// Loaded reports the chunks within r chunks of p fully loaded.
	Loaded(p behavior.Pos, r int32) bool
	// UntickedNeighbour reports a chunk within r of the chunk at p that is
	// not ticked.
	UntickedNeighbour(p behavior.Pos, r int32) bool
}

// Despawn is minecraft:despawn (1.21.3 DespawnComponent::tick 0x1405BC140,
// _tryStandardDespawnRules 0x1405A4630). BDS passes the distances to
// Dimension::fetchAnyInteractablePlayer in registers the decompile drops;
// which call takes min_distance and which max_distance is taken from what
// the rules do and was not read.
type Despawn struct {
	cfg DespawnConfig
	buf []behavior.Actor
}

// NewDespawn makes the component.
func NewDespawn(c DespawnConfig) *Despawn { return &Despawn{cfg: c} }

// Config returns the settings.
func (d *Despawn) Config() DespawnConfig { return d.cfg }

// SetConfig changes the settings.
func (d *Despawn) SetConfig(c DespawnConfig) { d.cfg = c }

// Tick despawns the mob when its rules say so. Nothing happens while no
// player can interact anywhere in the dimension.
func (d *Despawn) Tick(env *Env, chunks Chunks) {
	m := env.Self
	if !d.playerWithin(env, m.Pos(), math.MaxFloat32) {
		return
	}
	c := &d.cfg
	var gone bool
	if emptyFilter(c.Filters) {
		if !c.FromDistance && !c.FromInactivity && !c.FromChance && !c.FromSimulationEdge {
			gone = true
		} else {
			gone = d.standard(env, chunks)
		}
	} else {
		if chunks == nil || !chunks.Loaded(blockPos(m.Pos()), 1) {
			return
		}
		gone = c.Filters.Eval(env.tester(m, nil))
	}
	if gone {
		m.Despawn()
	}
}

// standard is BDS's standard rules: inactivity or chance, and no player
// within min_distance; else past max_distance at a short tick range or at
// the simulation edge.
func (d *Despawn) standard(env *Env, chunks Chunks) bool {
	m := env.Self
	c := &d.cfg
	if m.Persistent() {
		return false
	}
	ok := true
	if c.FromInactivity && m.IsMob() && m.NoActionTime() < ticksPerSecond*c.InactivityTimer {
		ok = false
	} else if c.FromChance {
		n := c.RandomChance
		if n < 0 {
			n = -n
		}
		ok = n == 0 || env.World.Random().NextInt(n) == 0
	}
	if c.FromDistance {
		near := d.playerWithin(env, m.Pos(), c.MinDistance)
		if near {
			m.ResetNoActionTime()
		}
		ok = ok && !near
	}
	if ok && (c.FromDistance || c.FromInactivity || c.FromChance) {
		return true
	}
	if (!ok && c.FromDistance || c.FromSimulationEdge) && chunks != nil && chunks.TickRange() <= c.EdgeTickRange {
		return !d.playerWithin(env, m.Pos(), c.MaxDistance)
	}
	if c.FromSimulationEdge && chunks != nil {
		p := m.Pos()
		origin := behavior.Pos{X: int32(math.Floor(float64(p.X))) >> 4 << 4, Z: int32(math.Floor(float64(p.Z))) >> 4 << 4}
		if chunks.Loaded(origin, 1) && chunks.UntickedNeighbour(origin, 1) {
			return true
		}
	}
	return c.FromDistance && !d.playerWithin(env, m.Pos(), c.MaxDistance)
}

// playerWithin is Dimension::fetchAnyInteractablePlayer: a player that is
// not a spectator within dist.
func (d *Despawn) playerWithin(env *Env, p behavior.Vec3, dist float32) bool {
	d.buf = env.World.Players(d.buf[:0])
	defer clear(d.buf)
	for _, pl := range d.buf {
		if behavior.Spectator(env.World, pl) || pl.Removed() {
			continue
		}
		q := pl.Pos()
		dx, dy, dz := float64(q.X-p.X), float64(q.Y-p.Y), float64(q.Z-p.Z)
		if dist == math.MaxFloat32 || dx*dx+dy*dy+dz*dz <= float64(dist)*float64(dist) {
			return true
		}
	}
	return false
}

func blockPos(v behavior.Vec3) behavior.Pos {
	return behavior.Pos{X: int32(math.Floor(float64(v.X))), Y: int32(math.Floor(float64(v.Y))), Z: int32(math.Floor(float64(v.Z)))}
}
