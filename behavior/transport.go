package behavior

import (
	"math"
	"slices"
)

// TransportContainer is a container block entity, as the engine reports
// it to transport_items.
type TransportContainer struct {
	Pos Pos
	// Kind is the block entity's BDS type (BlockActorType).
	Kind int32
	// CanOpen is the chest and shulker box open test (0xD45A7D0): nothing
	// keeps the lid shut.
	CanOpen bool
	// Pair is the other half of a double chest when HasPair is set.
	Pair    Pos
	HasPair bool
	// Viewers is how many entities have the chest open.
	Viewers int32
}

// TransportRules are the item settings transport_items hands the engine
// with every take and place.
type TransportRules struct {
	MaxStackSize int32
	// PlaceStrategy is TransportPlaceAny, TransportPlaceWithMatching or
	// TransportPlaceWithMatchingOrEmpty.
	PlaceStrategy int
	// AllowedItems and DisallowedItems are item descriptors; at most one
	// of them is set.
	AllowedItems, DisallowedItems []string
}

// The place_strategy values.
const (
	TransportPlaceAny = iota
	TransportPlaceWithMatching
	TransportPlaceWithMatchingOrEmpty
)

// The search_strategy values.
const (
	TransportNearest = iota
	TransportRandom
)

// ItemTransporter is the part of the mob transport_items needs on top of
// Mob. Containers and items are the engine's.
type ItemTransporter interface {
	Mob
	BlockMatcher
	// Containers appends the container block entities touching b to dst,
	// in BDS's order (BlockSource::fetchBlockEntities).
	Containers(dst []TransportContainer, b Box) []TransportContainer
	// Container is the container block entity at p.
	Container(p Pos) (TransportContainer, bool)
	// HoldsItem reports an item in the mob's hand.
	HoldsItem() bool
	// SeesBlock is the line of sight test to the block at p (0xBF2D080):
	// clips from the mob to the middle of the block's shape, then to the
	// middle of each face the mob is outside of, hitting nothing or the
	// block itself.
	SeesBlock(p Pos) bool
	// OpenContainer opens the container at p for the mob and keeps it open
	// until CloseContainer; it replaces a container held open before.
	OpenContainer(p Pos)
	// CloseContainer lets go of the container held open, if any.
	CloseContainer()
	// TakeItems moves items from the container at p to the mob's hand, and
	// PlaceItems from the hand to the container, following the rules. With
	// simulate set they only report whether they would.
	TakeItems(p Pos, r TransportRules, simulate bool) bool
	PlaceItems(p Pos, r TransportRules, simulate bool) bool
}

// TransportItemsConfig is the JSON of minecraft:behavior.transport_items.
type TransportItemsConfig struct {
	// SourceContainerTypes and DestinationContainerTypes are block
	// descriptors; empty takes any container.
	SourceContainerTypes, DestinationContainerTypes []string
	MaxStackSize                                    int32 `json:"max_stack_size"`
	// InteractionTime is in seconds.
	InteractionTime              float32 `json:"interaction_time"`
	AllowSimultaneousInteraction bool    `json:"allow_simultaneous_interaction"`
	SearchStrategy               int
	// SearchDistance is the horizontal and vertical reach of the search.
	SearchDistance       [2]float32
	MaxVisitedContainers int32 `json:"max_visited_containers"`
	// InitialCooldown and IdleCooldown are in whole seconds.
	InitialCooldown int32 `json:"initial_cooldown"`
	IdleCooldown    int32 `json:"idle_cooldown"`
	PlaceStrategy   int
	AllowedItems    []string
	DisallowedItems []string
	// ContainerKinds are the block entity types the goal uses,
	// OpenCheckKinds those that must pass CanOpen, and ChestKind the type
	// whose pair and viewers count.
	ContainerKinds, OpenCheckKinds []int32
	ChestKind                      int32
	// LookYaw is the look control's yaw limit towards the container.
	LookYaw float32
	// The entity events the goal fires on the mob.
	StartTakeFail, StartTakeSucceed, StartPlaceFail, StartPlaceSucceed, StopInteraction string
}

// DefaultTransportItems returns the goal's BDS defaults (the definition's
// default 0xF259F80, copied into the goal by 0xBED4D50; kinds from the
// container test 0xBF2E960, events from 0xBF2B990).
func DefaultTransportItems() TransportItemsConfig {
	return TransportItemsConfig{
		MaxStackSize: 64, InteractionTime: 3, SearchStrategy: TransportRandom,
		SearchDistance: [2]float32{64, 32}, MaxVisitedContainers: 16, IdleCooldown: 20,
		ContainerKinds: []int32{2, 25, 42}, OpenCheckKinds: []int32{2, 25}, ChestKind: 2,
		LookYaw:           10,
		StartTakeFail:     "minecraft:transport_items.start_take_fail",
		StartTakeSucceed:  "minecraft:transport_items.start_take_succeed",
		StartPlaceFail:    "minecraft:transport_items.start_place_fail",
		StartPlaceSucceed: "minecraft:transport_items.start_place_succeed",
		StopInteraction:   "minecraft:transport_items.stop_interaction",
	}
}

// The goal's states.
const (
	transportSearch = iota
	transportWalk
	transportArrive
	transportInteract
)

// The reach box around a container's middle, and how much the mob's box
// grows on x and z when a finished path is checked.
const (
	transportReach = 1
	transportGrow  = 0.25
	transportSpeed = 1 // the speed of the path to a container
)

// TransportItems is TransportItemsGoal (canUse 0xBF2B520, canContinue
// 0xBF2B630, start 0xBF2B720, stop 0xBF2B7D0, tick 0xBF2BD60): the mob
// walks to a container in range it has not visited, takes items when its
// hand is empty or places them when it holds some, and rests once nothing
// is left to visit.
type TransportItems struct {
	base
	m     ItemTransporter
	cfg   TransportItemsConfig
	rules TransportRules
	state int
	// visits counts the containers walked to; visited holds them and
	// their chest halves.
	visits                int32
	visited               []Pos
	target                Pos
	hasTarget             bool
	interactEnd           uint64
	interacting           bool
	cooldownEnd           uint64
	cooling               bool
	hadItem, seen         bool
	needSight             bool
	found                 []TransportContainer
	interactTicks, idleAt uint64
}

// NewTransportItems makes the goal (create 0x56C8180).
func NewTransportItems(w World, m ItemTransporter, c TransportItemsConfig) *TransportItems {
	g := &TransportItems{base: base{mob: m, world: w, flags: flagsMove | flagsJump}, m: m}
	g.SetConfig(c)
	if t := uint64(c.InitialCooldown) * ticksPerSecond; t != 0 {
		g.cooling, g.cooldownEnd = true, w.CurrentTick()+t
	}
	return g
}

// Config returns the goal's settings.
func (g *TransportItems) Config() TransportItemsConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *TransportItems) SetConfig(c TransportItemsConfig) {
	g.cfg = c
	g.rules = TransportRules{MaxStackSize: c.MaxStackSize, PlaceStrategy: c.PlaceStrategy, AllowedItems: c.AllowedItems, DisallowedItems: c.DisallowedItems}
	g.interactTicks = uint64(int64(c.InteractionTime * ticksPerSecond))
	g.idleAt = uint64(c.IdleCooldown) * ticksPerSecond
}

// free is the shared start and continue test: a navigation, and neither
// leashed nor sitting.
func (g *TransportItems) free() bool {
	return g.mob.Nav() != nil && !g.mob.Flag(FlagLeashed) && !g.mob.Flag(FlagSitting)
}

// CanUse implements goal.Goal.
func (g *TransportItems) CanUse() bool {
	return g.free() && (!g.cooling || g.world.CurrentTick() > g.cooldownEnd)
}

// CanContinueToUse implements goal.Goal.
func (g *TransportItems) CanContinueToUse() bool { return g.free() && !g.cooling }

// Start implements goal.Goal.
func (g *TransportItems) Start() { g.reset() }

// reset clears the goal's walk, as Start and a hand change do.
func (g *TransportItems) reset() {
	g.state, g.visits = transportSearch, 0
	g.visited = g.visited[:0]
	g.interacting, g.cooling, g.hasTarget = false, false, false
	g.m.CloseContainer()
	g.seen, g.needSight = false, false
}

// Stop implements goal.Goal.
func (g *TransportItems) Stop() {
	g.m.CloseContainer()
	g.fire(g.cfg.StopInteraction)
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
}

func (g *TransportItems) fire(event string) {
	if event != "" {
		g.mob.Event(event, g.mob.ID())
	}
}

// idle ends the walk and rests for idle_cooldown.
func (g *TransportItems) idle() {
	g.state = transportSearch
	g.cooling, g.cooldownEnd = true, g.world.CurrentTick()+g.idleAt
	g.m.CloseContainer()
}

// types are the containers wanted: sources with an empty hand,
// destinations with an item.
func (g *TransportItems) types(holds bool) []string {
	if holds {
		return g.cfg.DestinationContainerTypes
	}
	return g.cfg.SourceContainerTypes
}

// Tick implements goal.Goal.
func (g *TransportItems) Tick() {
	holds := g.m.HoldsItem()
	if g.seen && g.hadItem != holds {
		g.reset()
		return
	}
	switch {
	case g.state == transportSearch:
		g.search(holds)
	case !g.hasTarget:
		g.idle()
	default:
		if c, ok := g.m.Container(g.target); ok && g.usable(c, false, g.types(holds)) {
			switch g.state {
			case transportWalk:
				g.walk()
			case transportArrive:
				g.arrive()
			case transportInteract:
				g.interact()
			}
			break
		}
		if g.state == transportInteract {
			g.fire(g.cfg.StopInteraction)
		}
		g.state = transportSearch
	}
	if g.hasTarget && !g.mob.LookBusy() {
		g.mob.LookAt(blockCentre(g.target), g.cfg.LookYaw, g.mob.MaxHeadXRot())
	}
	g.hadItem, g.seen = g.m.HoldsItem(), true
}

// usable is the container test (0xBF2E960); visited containers only fail
// it during the search.
func (g *TransportItems) usable(c TransportContainer, search bool, types []string) bool {
	if !slices.Contains(g.cfg.ContainerKinds, c.Kind) {
		return false
	}
	p, d := g.mob.Pos(), g.cfg.SearchDistance
	if float32(math.Abs(float64(p.Y-float32(c.Pos.Y)))) > d[1] {
		return false
	}
	dx, dz := float32(c.Pos.X)-p.X, float32(c.Pos.Z)-p.Z
	if dz*dz+dx*dx > d[0]*d[0] {
		return false
	}
	if search && slices.Contains(g.visited, c.Pos) {
		return false
	}
	if slices.Contains(g.cfg.OpenCheckKinds, c.Kind) && !c.CanOpen {
		return false
	}
	return len(types) == 0 || g.m.MatchesBlock(c.Pos, types)
}

// search picks the next container and starts walking to it.
func (g *TransportItems) search(holds bool) {
	if uint32(g.cfg.MaxVisitedContainers-1) < uint32(g.visits) {
		g.idle()
		return
	}
	types := g.types(holds)
	p := g.mob.Pos()
	h, v := float32(math.Abs(float64(g.cfg.SearchDistance[0]))), float32(math.Abs(float64(g.cfg.SearchDistance[1])))
	g.found = g.m.Containers(g.found[:0], Box{Min: Vec3{X: p.X - h, Y: p.Y - v, Z: p.Z - h}, Max: Vec3{X: p.X + h, Y: p.Y + v, Z: p.Z + h}})
	g.found = slices.DeleteFunc(g.found, func(c TransportContainer) bool { return !g.usable(c, true, types) })
	if len(g.found) == 0 {
		g.idle()
		return
	}
	var c TransportContainer
	switch g.cfg.SearchStrategy {
	case TransportNearest:
		best := float32(0)
		for i, f := range g.found {
			d := dist2(blockCentre(f.Pos), p)
			if i == 0 || d < best {
				c, best = f, d
			}
		}
	case TransportRandom:
		c = g.found[g.world.Random().NextInt(int32(len(g.found)))]
	default:
		g.idle()
		return
	}
	g.target, g.hasTarget = c.Pos, true
	g.visited = append(g.visited, c.Pos)
	n := g.mob.Nav()
	if n == nil {
		g.idle()
		return
	}
	if intersects(g.mob.Box(), reachBox(c.Pos)) {
		n.Stop()
	} else if !g.pathTo(n, c.Pos) {
		return
	}
	g.state = transportWalk
	g.visits++
	if c.Kind == g.cfg.ChestKind && c.HasPair {
		g.visited = append(g.visited, c.Pair)
	}
}

// pathTo is the path to a container (0xBF2D640): taken only when the mob
// would stand within reach of it at the path's end.
func (g *TransportItems) pathTo(n Navigator, p Pos) bool {
	c := blockCentre(p)
	end, ok := n.PathToPoint(c)
	if !ok {
		return false
	}
	b := g.mob.Box()
	hw, hd := (b.Max.X-b.Min.X)*0.5, (b.Max.Z-b.Min.Z)*0.5
	at := Box{
		Min: Vec3{X: float32(end.X) + 0.5 - hw, Y: float32(end.Y), Z: float32(end.Z) + 0.5 - hd},
		Max: Vec3{X: float32(end.X) + 0.5 + hw, Y: float32(end.Y) + (b.Max.Y - b.Min.Y), Z: float32(end.Z) + 0.5 + hd},
	}
	if !intersects(at, reachBox(p)) {
		return false
	}
	return n.MoveTo(c, transportSpeed)
}

// walk is the walking state (0xBF2CBE0).
func (g *TransportItems) walk() {
	n := g.mob.Nav()
	if n == nil {
		g.idle()
		return
	}
	r := reachBox(g.target)
	b := g.mob.Box()
	if intersects(b, r) {
		n.Stop()
		g.state, g.needSight = transportArrive, true
		return
	}
	if !n.Done() {
		return
	}
	grown := Box{Min: Vec3{X: b.Min.X - transportGrow, Y: b.Min.Y, Z: b.Min.Z - transportGrow}, Max: Vec3{X: b.Max.X + transportGrow, Y: b.Max.Y, Z: b.Max.Z + transportGrow}}
	switch {
	case intersects(grown, r):
		g.state, g.needSight = transportArrive, true
	case g.pathTo(n, g.target):
		g.state = transportWalk
	default:
		g.state = transportSearch
	}
}

// arrive is the arrived state (0xBF2C8B0): wait for other viewers, open
// the container and announce the take or place.
func (g *TransportItems) arrive() {
	if g.needSight {
		g.needSight = false
		if !g.m.SeesBlock(g.target) {
			g.state = transportSearch
			return
		}
	}
	if !g.cfg.AllowSimultaneousInteraction {
		if c, ok := g.m.Container(g.target); ok && c.Kind == g.cfg.ChestKind && c.Viewers > 0 {
			return
		}
	}
	if !g.m.SeesBlock(g.target) {
		g.state = transportSearch
		return
	}
	g.state = transportInteract
	g.interactEnd, g.interacting = g.world.CurrentTick()+g.interactTicks, true
	g.m.OpenContainer(g.target)
	if g.m.HoldsItem() {
		g.fire(pick(g.m.PlaceItems(g.target, g.rules, true), g.cfg.StartPlaceSucceed, g.cfg.StartPlaceFail))
		return
	}
	g.fire(pick(g.m.TakeItems(g.target, g.rules, true), g.cfg.StartTakeSucceed, g.cfg.StartTakeFail))
}

// interact is the interacting state (0xBF2CF60): once the time is up, move
// the items; a change of hand starts the visits over.
func (g *TransportItems) interact() {
	if g.interacting && g.world.CurrentTick() < g.interactEnd {
		return
	}
	g.fire(g.cfg.StopInteraction)
	g.m.CloseContainer()
	had := g.m.HoldsItem()
	if had {
		g.m.PlaceItems(g.target, g.rules, false)
	} else {
		g.m.TakeItems(g.target, g.rules, false)
	}
	if had != g.m.HoldsItem() {
		g.visited, g.visits = g.visited[:0], 0
	}
	g.interacting, g.hasTarget = false, false
	g.state = transportSearch
}

func pick(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

// reachBox is the box one block around a container's middle.
func reachBox(p Pos) Box {
	c := blockCentre(p)
	return Box{Min: Vec3{X: c.X - transportReach, Y: c.Y - transportReach, Z: c.Z - transportReach}, Max: Vec3{X: c.X + transportReach, Y: c.Y + transportReach, Z: c.Z + transportReach}}
}

// intersects is AABB::intersects: the boxes overlap with room to spare.
func intersects(a, b Box) bool {
	return a.Min.X < b.Max.X && b.Min.X < a.Max.X && a.Min.Y < b.Max.Y && b.Min.Y < a.Max.Y && a.Min.Z < b.Max.Z && b.Min.Z < a.Max.Z
}
