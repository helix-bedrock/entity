package behavior

import (
	"math"
	"slices"

	"github.com/helix-bedrock/entity/path"
)

// Gifter is the part of the mob drop_item_for needs on top of Mob. Loot
// and items are the engine's.
type Gifter interface {
	Mob
	// TimeOfDay is the dimension's time of day for the level time, from 0
	// to 1 (Dimension::getTimeOfDay).
	TimeOfDay() float32
	// DropGift rolls the loot table with the level's Random and the mob as
	// the looting entity, and spawns each item at the mob's position,
	// pickable after pickupDelay ticks. BDS then also sends its player event
	// when a cat drops for a player.
	DropGift(table string, pickupDelay int32, to ID)
}

// DropItemForConfig is the JSON of minecraft:behavior.drop_item_for. Times
// are in seconds.
type DropItemForConfig struct {
	MoveToConfig
	SecondsBeforePickup float32 `json:"seconds_before_pickup"`
	Cooldown            float32 `json:"cooldown"`
	DropItemChance      float32 `json:"drop_item_chance"`
	// OfferingDistance is how far from the target the mob wants to stand.
	OfferingDistance float32 `json:"offering_distance"`
	// MinimumTeleportDistance is compared with the squared distance to the
	// target spot, as BDS does.
	MinimumTeleportDistance float32 `json:"minimum_teleport_distance"`
	// MaxHeadLookAtHeight is the look control's yaw limit.
	MaxHeadLookAtHeight float32 `json:"max_head_look_at_height"`
	// TargetRange widens the mob's box on each axis for the target search.
	TargetRange    Vec3
	TeleportOffset Vec3
	TimeOfDayRange Range[float32]
	LootTable      string `json:"loot_table"`
	// OnDropAttempt is fired on the mob whenever the goal stops.
	OnDropAttempt string
	// EntityTypes pick the target; an entity passing any of them is taken.
	// With none, the goal never finds one.
	EntityTypes []Filter
}

// DefaultDropItemFor returns the goal's BDS defaults (the definition's
// default 0xE8F2D50, copied into the goal by 0x5884460).
func DefaultDropItemFor() DropItemForConfig {
	return DropItemForConfig{
		MoveToConfig:     MoveToConfig{SpeedMultiplier: 1, GoalRadius: 0.5, SearchHeight: 1},
		Cooldown:         0.25,
		DropItemChance:   1,
		OfferingDistance: 1, MinimumTeleportDistance: 2, MaxHeadLookAtHeight: 10,
		TargetRange:    Vec3{X: 1, Y: 1, Z: 1},
		TeleportOffset: Vec3{Y: 1},
		TimeOfDayRange: Range[float32]{0, 1},
	}
}

// DropItemFor is DropItemForGoal (canUse 0x55240F0, canContinue 0x5524110,
// start 0x5524DC0, stop 0x55256A0, tick 0x5524130, isValidTarget
// 0x5525A00, findTargetBlock 0x5525700): in the time of day window the mob
// picks the nearest entity passing the filters, walks to a spot near it,
// and drops a loot table there once.
type DropItemFor struct {
	moveTo
	m      Gifter
	cfg    DropItemForConfig
	target ID
	// wait counts down to the next start; dropped is set once a drop was
	// tried.
	wait, waitTicks int32
	pickupDelay     int32
	offering2       float32
	dropped         bool
	buf             []Actor
}

// NewDropItemFor makes the goal (create 0x5523F70).
func NewDropItemFor(w World, m Gifter, c DropItemForConfig) *DropItemFor {
	g := &DropItemFor{m: m, target: NoID}
	g.init(w, m, c.MoveToConfig, flagsMove|flagsJump, g)
	g.useAim, g.aimOffset = true, true
	g.SetConfig(c)
	return g
}

// Config returns the goal's settings.
func (g *DropItemFor) Config() DropItemForConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *DropItemFor) SetConfig(c DropItemForConfig) {
	g.cfg, g.moveTo.cfg = c, c.MoveToConfig
	g.radius2 = c.GoalRadius * c.GoalRadius
	g.pickupDelay = int32(c.SecondsBeforePickup * ticksPerSecond)
	g.waitTicks = int32(c.Cooldown * ticksPerSecond)
	g.offering2 = c.OfferingDistance * c.OfferingDistance
}

// CanUse implements goal.Goal: only the cooldown; the search is Start's.
func (g *DropItemFor) CanUse() bool {
	w := g.wait
	if w > 0 {
		g.wait--
	}
	return w <= 0
}

// CanContinueToUse implements goal.Goal.
func (g *DropItemFor) CanContinueToUse() bool { return g.target != NoID && !g.dropped }

// Start implements goal.Goal.
func (g *DropItemFor) Start() {
	g.target = NoID
	g.aim = g.mob.Pos()
	g.wait = g.waitTicks
	if g.mob.Nav() == nil {
		return
	}
	if t := g.m.TimeOfDay(); g.cfg.TimeOfDayRange.Min > t || t > g.cfg.TimeOfDayRange.Max {
		return
	}
	b, r := g.mob.Box(), g.cfg.TargetRange
	lo := Vec3{X: b.Min.X - r.X, Y: b.Min.Y - r.Y, Z: b.Min.Z - r.Z}
	hi := Vec3{X: b.Max.X + r.X, Y: b.Max.Y + r.Y, Z: b.Max.Z + r.Z}
	g.buf = g.world.Entities(g.buf[:0], g.mob.ID(), Box{
		Min: Vec3{X: min(lo.X, hi.X), Y: min(lo.Y, hi.Y), Z: min(lo.Z, hi.Z)},
		Max: Vec3{X: max(lo.X, hi.X), Y: max(lo.Y, hi.Y), Z: max(lo.Z, hi.Z)},
	})
	p := g.mob.Pos()
	slices.SortStableFunc(g.buf, func(a, b Actor) int {
		da, db := dist2(p, a.Pos()), dist2(p, b.Pos())
		switch {
		case da < db:
			return -1
		case da > db:
			return 1
		}
		return 0
	})
	for _, a := range g.buf {
		if g.wanted(a) {
			g.target = a.ID()
			break
		}
	}
	clear(g.buf)
	g.find()
	g.moveTo.Start()
}

// wanted is the target test: a filter passes and neither rides the other.
func (g *DropItemFor) wanted(a Actor) bool {
	if slices.Contains(g.mob.Riders(), a.ID()) || slices.Contains(a.Riders(), g.mob.ID()) {
		return false
	}
	for _, f := range g.cfg.EntityTypes {
		if f != nil && f(g.mob, a) {
			return true
		}
	}
	return false
}

// Stop implements goal.Goal.
func (g *DropItemFor) Stop() {
	if g.cfg.OnDropAttempt != "" {
		g.mob.Event(g.cfg.OnDropAttempt, g.mob.ID())
	}
	g.target = NoID
	g.dropped = false
}

// Tick implements goal.Goal.
func (g *DropItemFor) Tick() {
	g.moveTo.Tick()
	n := g.mob.Nav()
	pathDone := n == nil || n.Done()
	if dist2(g.aim, g.mob.Pos()) > g.cfg.MinimumTeleportDistance {
		o := g.cfg.TeleportOffset
		g.m.Teleport(add(g.aim, Vec3{X: float32(floorInt(o.X)), Y: float32(floorInt(o.Y)), Z: float32(floorInt(o.Z))}))
		if n != nil {
			n.Stop()
		}
	}
	if g.target == NoID {
		return
	}
	t := g.actor(g.target)
	if t == nil {
		g.target = NoID
		return
	}
	if !g.dropped && (g.reached || pathDone) {
		g.dropped = true
		if c := g.cfg.DropItemChance; c > 0 && c >= g.world.Random().NextFloat() && g.cfg.LootTable != "" {
			g.m.DropGift(g.cfg.LootTable, g.pickupDelay, t.ID())
		}
	}
	g.mob.LookAt(t.Eyes(), g.cfg.MaxHeadLookAtHeight, g.mob.MaxHeadXRot())
}

// valid is the goal's isValidTarget: a solid block (Block::isSolid) with
// air above.
func (g *DropItemFor) valid(p Pos) bool {
	return g.world.Block(Pos{X: p.X, Y: p.Y + 1, Z: p.Z}).Material == path.MaterialAir && g.world.Block(p).BlockSolid
}

// find is the goal's findTargetBlock: around the target's block, the spot
// whose squared distance to it is nearest the squared offering distance;
// the first of equals wins.
func (g *DropItemFor) find() bool {
	t := g.actor(g.target)
	if t == nil {
		return false
	}
	c := blockOf(t.Pos())
	r, h := g.cfg.SearchRange, g.cfg.SearchHeight
	if h < 0 || r <= 0 {
		return false
	}
	best, found := float32(moveToFar), false
	for k := int32(0); k <= h; k = ringNext(k) {
		for i := int32(0); i < r; i++ {
			for x := int32(0); x <= i; x = ringNext(x) {
				z := int32(0)
				if x > -i && x < i {
					z = i
				}
				for ; z <= i; z = ringNext(z) {
					p := Pos{X: c.X + x, Y: c.Y - 1 + k, Z: c.Z + z}
					if !g.valid(p) {
						continue
					}
					d := float32(math.Abs(float64(g.offering2 - float32(blockDist2(c, p)))))
					if best > d && g.reachable(p) {
						g.aim, best, found = vecOf(p), d, true
					}
				}
			}
		}
	}
	return found
}
