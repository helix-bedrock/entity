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
	// Pos is the POI block.
	Pos Pos
	// Radius is how near counts as there.
	Radius float32
	// UseBox is: the goal aims at the middle of Box and counts as
	// there once the mob's box touches it.
	UseBox bool
	// Box is the POI block's box, joined with the other half of a two block
	// POI such as a bed.
	Box Box
	// Sound is the job site's work sound event.
	Sound string
}

// Dweller is the part of the mob the village goals need on top of Mob:
// the DwellerComponent and its village.
type Dweller interface {
	Mob
	// IsDweller reports a DwellerComponent.
	IsDweller() bool
	// POI is the dweller's own POI of the kind in its village (
	// then).
	POI(kind POIKind) (POI, bool)
	// FreePOI is the village's nearest unclaimed POI of the kind to p
	FreePOI(kind POIKind, p Pos) (POI, bool)
	// POIResult records how a trip to the POI ended: ok clears
	// its fail count; otherwise the count goes up and, while it is 2 or
	// less, the engine shows level event 2009 three times on the POI's top
	POIResult(kind POIKind, ok bool)
	// Village is the bounds of the dweller's village, or of the nearest
	// village within 64 blocks of the mob when it has none, searching
	// search blocks (-1 for no limit).
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

// DefaultMoveToPOI returns the goal's BDS defaults (TypeSchema).
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
	// box is: the POI is a box, and goalBox is it grown by the radius.
	box     bool
	goalBox Box
	// sameY is: a path whose end is not at the POI's height fails.
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

// lookup is the base POI lookup: the dweller's own POI.
func (g *moveToPOI) lookup(kind POIKind) (POI, bool) { return g.d.POI(kind) }

// valid is isValidTarget: the POI still exists.
func (g *moveToPOI) valid(Pos) bool {
	_, ok := g.find.lookup(g.kind)
	return ok
}

// reachable is the base path check.
func (g *moveToPOI) reachable(p Pos) bool { return g.moveTo.reachable(p) }

// moveToBlock is moveMobToBlock: BDS walks the path canUse
// made, which goes to the stored point.
func (g *moveToPOI) moveToBlock() {
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.aim, g.cfg.SpeedMultiplier)
	}
}

// canUse is MoveToPOIGoal's canUse.
func (g *moveToPOI) canUse() bool {
	return g.canStart() && g.d.IsDweller() && g.begin(g.kind)
}

// begin is: aim at the POI and see whether it can be reached.
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
		// BDS grows the box by the squared radius it keeps.
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

// startPOI is the base start.
func (g *moveToPOI) startPOI() { g.Start() }

// stopPOI is MoveToPOIGoal's stop.
func (g *moveToPOI) stopPOI() {
	if _, ok := g.find.lookup(g.kind); ok {
		g.d.POIResult(g.kind, g.cooldown == 0 && g.tries <= moveToGiveUp)
	}
	g.Stop()
	g.box = false
}

// tickPOI is MoveToPOIGoal's tick.
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

// NewMoveToPOI makes the goal.
func NewMoveToPOI(w World, d Dweller, c MoveToPOIConfig) *MoveToPOI {
	g := &MoveToPOI{}
	g.initPOI(w, d, c, g, g)
	return g
}

// find is findTargetBlock; the POI goals do not search.
func (g *MoveToPOI) find() bool { return false }

// CanUse implements goal.Goal.
func (g *MoveToPOI) CanUse() bool { return g.canUse() }

// CanContinueToUse implements goal.Goal.
func (g *MoveToPOI) CanContinueToUse() bool { return g.keepLite() }

// Start implements goal.Goal.
func (g *MoveToPOI) Start() { g.startPOI() }

// Stop implements goal.Goal.
func (g *MoveToPOI) Stop() { g.stopPOI() }

// Tick implements goal.Goal.
func (g *MoveToPOI) Tick() { g.tickPOI() }
