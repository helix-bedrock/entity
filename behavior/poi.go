package behavior

// POIKind is a village point of interest type (MoveToPOIGoalDefinition's
// POIType, in schema order).
type POIKind int32

// The POI kinds.
const (
	POIBed POIKind = iota
	POIMeetingArea
	POIJobSite
)

// POI is a village point of interest as the engine sees it.
type POI struct {
	// Pos is the POI block (+128).
	Pos Pos
	// Radius is how near counts as there (+168).
	Radius float32
	// UseBox is +218: the goal aims at the middle of Box and counts as
	// there once the mob's box touches it.
	UseBox bool
	// Box is the POI block's box, joined with the other half of a two block
	// POI such as a bed (0xBD0D110).
	Box Box
	// Sound is the job site's work sound event (+184).
	Sound string
}

// Dweller is the part of the mob the village goals need on top of Mob:
// the DwellerComponent and its village.
type Dweller interface {
	Mob
	// IsDweller reports a DwellerComponent.
	IsDweller() bool
	// POI is the dweller's own POI of the kind in its village (0x7072420,
	// then 0xBD13BA0).
	POI(kind POIKind) (POI, bool)
	// FreePOI is the village's nearest unclaimed POI of the kind to p
	// (0xBD1C9A0).
	FreePOI(kind POIKind, p Pos) (POI, bool)
	// POIResult records how a trip to the POI ended (0x5279660): ok clears
	// its fail count; otherwise the count goes up and, while it is 2 or
	// less, the engine shows level event 2009 three times on the POI's top
	// (0xBD0D270).
	POIResult(kind POIKind, ok bool)
	// Village is the bounds of the dweller's village, or of the nearest
	// village within 64 blocks of the mob when it has none, searching
	// search blocks (-1 for no limit) (0x527AB80).
	Village(search int32) (bounds Box, ok bool)
}

// MoveToPOIConfig is the JSON of minecraft:behavior.move_to_poi.
type MoveToPOIConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// TimeoutCooldown is the rest, in seconds, after a path that did not
	// move.
	TimeoutCooldown float32 `json:"timeout_cooldown"`
	GoalRadius      float32 `json:"goal_radius"`
	POIType         POIKind `json:"poi_type"`
}

// DefaultMoveToPOI returns the goal's BDS defaults (TypeSchema 0xDF7AD80).
func DefaultMoveToPOI() MoveToPOIConfig {
	return MoveToPOIConfig{SpeedMultiplier: 1, TimeoutCooldown: 8}
}

// poiLookup is the POI the goal goes to (slot 20); hide replaces it.
type poiLookup interface {
	lookup(kind POIKind) (POI, bool)
}

// moveToPOI is MoveToPOIGoal: walk to one of the dweller's POIs.
type moveToPOI struct {
	moveTo
	d    Dweller
	kind POIKind
	// box is +168: the POI is a box, and goalBox is it grown by the radius.
	box     bool
	goalBox Box
	// sameY is +169: a path whose end is not at the POI's height fails.
	sameY bool
	find  poiLookup
}

// initPOI sets up the base for a goal whose POI lookup is f.
func (g *moveToPOI) initPOI(w World, d Dweller, c MoveToPOIConfig, h moveToHooks, f poiLookup) {
	g.init(w, d, MoveToConfig{SpeedMultiplier: c.SpeedMultiplier, GoalRadius: c.GoalRadius}, flagsMove, h)
	g.restTime = int32(abs32(c.TimeoutCooldown)) * ticksPerSecond
	g.useAim = true
	g.d, g.kind, g.find = d, c.POIType, f
}

// lookup is the base POI lookup (0x5279A90): the dweller's own POI.
func (g *moveToPOI) lookup(kind POIKind) (POI, bool) { return g.d.POI(kind) }

// valid is isValidTarget (0x52797E0): the POI still exists.
func (g *moveToPOI) valid(Pos) bool {
	_, ok := g.find.lookup(g.kind)
	return ok
}

// reachable is the base path check.
func (g *moveToPOI) reachable(p Pos) bool { return g.moveTo.reachable(p) }

// moveToBlock is moveMobToBlock (0x52798C0): BDS walks the path canUse
// made, which goes to the stored point.
func (g *moveToPOI) moveToBlock() {
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.aim, g.cfg.SpeedMultiplier)
	}
}

// canUse is MoveToPOIGoal's canUse (0x5272320).
func (g *moveToPOI) canUse() bool {
	return g.canStart() && g.d.IsDweller() && g.begin(g.kind)
}

// begin is 0x5278FD0: aim at the POI and see whether it can be reached.
func (g *moveToPOI) begin(kind POIKind) bool {
	n := g.mob.Nav()
	if n == nil {
		return false
	}
	poi, ok := g.find.lookup(kind)
	if !ok {
		return false
	}
	g.aim = vecOf(poi.Pos)
	g.radius2 = poi.Radius * poi.Radius
	g.box, g.aimHalf = poi.UseBox, !poi.UseBox
	m := g.mob.Pos()
	if g.box {
		g.aim = boxCentre(poi.Box)
		// BDS grows the box by the squared radius it keeps at +68.
		g.goalBox = grow(poi.Box, g.radius2)
		if overlaps(g.goalBox, g.mob.Box()) {
			g.reached = true
			return true
		}
	} else if g.radius2 >= dist2(g.aim, m) {
		g.reached = true
		return true
	}
	end, ok := n.PathToPoint(g.aim)
	if !ok || g.sameY && end.Y != floorInt(g.aim.Y) {
		return false
	}
	// BDS takes a complete path (status 1) as reaching the POI; the end
	// block being the POI's stands in for that status here.
	if end == blockOf(g.aim) {
		return true
	}
	c := Vec3{X: float32(end.X) + 0.5, Y: float32(end.Y) + 0.5, Z: float32(end.Z) + 0.5}
	if !g.box {
		return g.radius2 >= dist2(c, g.aim)
	}
	b := g.mob.Box()
	hx, hy, hz := (b.Max.X-b.Min.X)*0.5, (b.Max.Y-b.Min.Y)*0.5, (b.Max.Z-b.Min.Z)*0.5
	node := Box{Min: Vec3{X: c.X - hx, Y: c.Y - hy, Z: c.Z - hz}, Max: Vec3{X: c.X + hx, Y: c.Y + hy, Z: c.Z + hz}}
	return overlaps(g.goalBox, node)
}

// grow is the box grown by d on every side, with its corners kept in order
// as BDS's AABB grow does.
func grow(b Box, d float32) Box {
	lo := Vec3{X: b.Min.X - d, Y: b.Min.Y - d, Z: b.Min.Z - d}
	hi := Vec3{X: b.Max.X + d, Y: b.Max.Y + d, Z: b.Max.Z + d}
	return Box{Min: Vec3{X: min(lo.X, hi.X), Y: min(lo.Y, hi.Y), Z: min(lo.Z, hi.Z)},
		Max: Vec3{X: max(lo.X, hi.X), Y: max(lo.Y, hi.Y), Z: max(lo.Z, hi.Z)}}
}

// startPOI is the base start (0x48C91B0).
func (g *moveToPOI) startPOI() { g.Start() }

// stopPOI is MoveToPOIGoal's stop (0x5272A30).
func (g *moveToPOI) stopPOI() {
	if _, ok := g.find.lookup(g.kind); ok {
		g.d.POIResult(g.kind, g.cooldown == 0 && g.tries <= moveToGiveUp)
	}
	g.Stop()
	g.box = false
}

// tickPOI is MoveToPOIGoal's tick (0x5273610).
func (g *moveToPOI) tickPOI() {
	if !g.box {
		g.Tick()
		return
	}
	n := g.mob.Nav()
	if overlaps(g.goalBox, g.mob.Box()) {
		g.reached = true
		g.stay++
		if n != nil && !n.Done() {
			n.Stop()
		}
		return
	}
	g.reached = false
	g.tries++
	if g.tries%g.repath != 0 {
		return
	}
	g.hooks.moveToBlock()
	if n != nil && g.stuck() {
		g.cooldown = g.restTime
	}
}

// MoveToPOI is MoveToPOIGoal: a villager walks to its bed, job site or
// meeting area.
type MoveToPOI struct{ moveToPOI }

// NewMoveToPOI makes the goal (0x52B9D00).
func NewMoveToPOI(w World, d Dweller, c MoveToPOIConfig) *MoveToPOI {
	g := &MoveToPOI{}
	g.initPOI(w, d, c, g, g)
	return g
}

// find is findTargetBlock; the POI goals do not search.
func (g *MoveToPOI) find() bool { return false }

// CanUse implements goal.Goal.
func (g *MoveToPOI) CanUse() bool { return g.canUse() }

// CanContinueToUse implements goal.Goal (0x48C8A40).
func (g *MoveToPOI) CanContinueToUse() bool { return g.keepLite() }

// Start implements goal.Goal.
func (g *MoveToPOI) Start() { g.startPOI() }

// Stop implements goal.Goal.
func (g *MoveToPOI) Stop() { g.stopPOI() }

// Tick implements goal.Goal.
func (g *MoveToPOI) Tick() { g.tickPOI() }
