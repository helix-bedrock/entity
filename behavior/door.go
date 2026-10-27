package behavior

import "github.com/helix-bedrock/entity/nav"

// The actor flags that keep a mob from using doors.
const (
	FlagDoorBlocked1 = 79
	FlagDoorBlocked2 = 81
)

// The door goal numbers.
const (
	doorReach2        = 2.25    // squared horizontal distance within which a door counts
	doorPassMargin    = 0.09375 // how far past the door's edge the mob must be
	doorWideMob       = 0.8125  // mobs wider than this always keep the door goal
	doorWideSlack     = 0.005
	doorPathLookAhead = 2  // path nodes past the current one searched for doors
	openDoorTime      = 20 // ticks an opened door is held before closing
	breakDoorTime     = 240
	// breakDoorCrackSpeed is floor(65535 / breakDoorTime), the crack
	// event's speed.
	breakDoorCrackSpeed = 65535 / breakDoorTime
	breakDoorKnock      = 20 // one in this many ticks knocks on the door
	breakDoorReach2     = 4
	difficultyHard      = 3
	eventDoorCrack      = 3600
	eventDoorCrackStop  = 3601
	eventDoorKnock      = 1010
	eventDoorBreak      = 1012
	eventDestroyBlock   = 2021
)

// noDir is no path direction through a door.
const noDir int8 = -1

// DoorState is a door block's states, as the door goals read them.
type DoorState struct {
	// Direction is the door's direction state, 0 to 3.
	Direction int32
	Open      bool
	// Hinge is door_hinge_bit.
	Hinge bool
}

// DoorUser is the part of the mob the door goals need on top of Mob.
type DoorUser interface {
	Mob
	// UsesDoors reports the component BDS keys 0xD5EE0BD8 that the door
	// goals need.
	UsesDoors() bool
	// PathNodes is the navigation path and the index of the node being
	// walked to; ok is false without a path.
	PathNodes() (nodes []Pos, index int, ok bool)
	// Door is the door at p; ok is false when the block is not a door.
	Door(p Pos) (DoorState, bool)
	// SetDoorOpen opens or closes the door at p as the mob (DoorBlock
	// 0xC580E70).
	SetDoorOpen(p Pos, open bool)
	// Width is the mob's collision box width.
	Width() float32
	// Difficulty is the level's difficulty, 0 (peaceful) to 3 (hard).
	Difficulty() int
	// DoorEvent broadcasts a level event at p with a number as its data
	// (the door crack events).
	DoorEvent(id int32, p Vec3, data int32)
}

// DoorInteract is DoorInteractGoal: the shared part of the door goals. It
// finds a door on the mob's path that the path goes through and notices
// when the mob is through it.
type DoorInteract struct {
	base
	m    DoorUser
	door Pos
	have bool
	// in and out are the path's directions into and out of the door.
	in, out int8
	wasOpen bool
	wide    bool
	passed  bool
	toDoor  Vec3
}

// NewDoorInteract makes the goal.
func NewDoorInteract(w World, m DoorUser) *DoorInteract {
	return &DoorInteract{base: base{mob: m, world: w}, m: m}
}

// CanUse implements goal.Goal (0x576D6C0): the door at the mob's head
// height, or else at one of the next path nodes.
func (g *DoorInteract) CanUse() bool {
	m := g.m
	n := m.Nav()
	if !m.UsesDoors() || n == nil {
		return false
	}
	nodes, idx, ok := m.PathNodes()
	if !ok || idx >= len(nodes) || nav.Flags(n.Flags())&nav.FlagCanOpenDoors == 0 {
		return false
	}
	if m.Flag(FlagDoorBlocked1) || m.Flag(FlagDoorBlocked2) {
		return false
	}
	p := m.Pos()
	g.door = Pos{X: floorInt(p.X), Y: floorInt(p.Y + 1), Z: floorInt(p.Z)}
	g.have = g.find(nodes, idx)
	limit := min(idx+doorPathLookAhead, len(nodes))
	for i := 0; !g.have && i < limit; i++ {
		node := nodes[min(i, len(nodes)-1)]
		g.door = Pos{X: node.X, Y: node.Y + 1, Z: node.Z}
		g.have = g.find(nodes, idx)
	}
	return g.have
}

// find is 0x576D9F0: the door at g.door counts when the mob is near it and
// the path goes into or out of it through its closed side.
func (g *DoorInteract) find(nodes []Pos, idx int) bool {
	g.in, g.out = noDir, noDir
	p, d := g.m.Pos(), g.door
	dx, dz := p.X-float32(d.X), p.Z-float32(d.Z)
	if dz*dz+dx*dx > doorReach2 {
		return false
	}
	st, ok := g.m.Door(d)
	if !ok {
		return false
	}
	limit := min(idx+doorPathLookAhead, len(nodes))
	last := len(nodes) - 1
	for i := 1; i < limit; i++ {
		n := nodes[min(i, last)]
		if n.X != d.X || n.Z != d.Z || abs32(float32(n.Y-d.Y)) > 1 {
			continue
		}
		if prev := nodes[min(i-1, last)]; prev.X != d.X || prev.Z != d.Z {
			g.in = pathDir(prev.X-d.X, prev.Z-d.Z)
		}
		if i+1 < limit {
			if next := nodes[min(i+1, last)]; next.X != d.X || next.Z != d.Z {
				g.out = pathDir(next.X-d.X, next.Z-d.Z)
			}
		}
		break
	}
	if g.in == g.out {
		return false
	}
	side := doorSide(st)
	return side == g.out || side == g.in
}

// pathDir is the direction of a step from the door: 0 and 2 along z, 1
// and 3 along x.
func pathDir(dx, dz int32) int8 {
	if abs32(float32(dx)) <= abs32(float32(dz)) {
		if dz > 0 {
			return 2
		}
		return 0
	}
	if dx <= 0 {
		return 3
	}
	return 1
}

// doorSide is the side the door's panel blocks, from its direction, open
// state and hinge.
func doorSide(s DoorState) int8 {
	h := int8(0)
	if s.Hinge {
		h = 1
	}
	switch s.Direction {
	case 0:
		if s.Open {
			return 2 * h
		}
		return 3
	case 1:
		if s.Open {
			return 2*h + 1
		}
		return 0
	case 2:
		if s.Open {
			return 2 * (h ^ 1)
		}
		return 1
	case 3:
		if s.Open {
			return 2*(h^1) + 1
		}
		return 2
	}
	return noDir
}

// CanContinueToUse implements goal.Goal (0x576DDA0).
func (g *DoorInteract) CanContinueToUse() bool { return g.have && !g.passed }

// Start implements goal.Goal (0x576DDC0).
func (g *DoorInteract) Start() {
	g.wide = g.m.Width()+doorWideSlack > doorWideMob
	st, _ := g.m.Door(g.door)
	g.wasOpen = st.Open
	g.passed = false
	p := g.m.Pos()
	g.toDoor = Vec3{X: float32(g.door.X) + 0.5 - p.X, Z: float32(g.door.Z) + 0.5 - p.Z}
}

// Tick implements goal.Goal (0x576DE90): the mob is through once it is past
// the door's edge on the way out.
func (g *DoorInteract) Tick() { g.passed = g.through() }

func (g *DoorInteract) through() bool {
	x, z := float32(g.door.X), float32(g.door.Z)
	p := g.m.Pos()
	switch g.out {
	case 0:
		return z+doorPassMargin > p.Z
	case 1:
		return p.X > x+1-doorPassMargin
	case 2:
		return p.Z > z+1-doorPassMargin
	case 3:
		return x+doorPassMargin > p.X
	}
	return (x+0.5-p.X)*g.toDoor.X+(z+0.5-p.Z)*g.toDoor.Z < 0
}

// OpenDoorConfig is the JSON of minecraft:behavior.open_door.
type OpenDoorConfig struct {
	CloseDoorAfter bool `json:"close_door_after"`
}

// DefaultOpenDoor returns the goal's BDS defaults (0xDCA3AE0).
func DefaultOpenDoor() OpenDoorConfig { return OpenDoorConfig{CloseDoorAfter: true} }

// OpenDoor is OpenDoorGoal: the mob opens a door on its path and closes it
// again behind it.
type OpenDoor struct {
	DoorInteract
	cfg   OpenDoorConfig
	timer int32
}

// NewOpenDoor makes the goal.
func NewOpenDoor(w World, m DoorUser, c OpenDoorConfig) *OpenDoor {
	return &OpenDoor{DoorInteract: DoorInteract{base: base{mob: m, world: w}, m: m}, cfg: c}
}

// CanContinueToUse implements goal.Goal (0x5755030): only while the door is
// to be closed again.
func (g *OpenDoor) CanContinueToUse() bool {
	return g.cfg.CloseDoorAfter && g.timer > 0 && g.have && !g.passed
}

// Start implements goal.Goal (0x5755050).
func (g *OpenDoor) Start() {
	g.DoorInteract.Start()
	g.timer = openDoorTime
	g.m.SetDoorOpen(g.door, !g.wasOpen)
}

// Stop implements goal.Goal (0x57550E0).
func (g *OpenDoor) Stop() {
	if g.cfg.CloseDoorAfter && g.have {
		g.m.SetDoorOpen(g.door, g.wasOpen)
	}
}

// Tick implements goal.Goal (0x5755180).
func (g *OpenDoor) Tick() {
	g.timer--
	g.DoorInteract.Tick()
}

// BreakDoor is BreakDoorGoal: on hard difficulty a mob with a target
// breaks down a door in its way.
type BreakDoor struct {
	DoorInteract
	time int32
}

// NewBreakDoor makes the goal.
func NewBreakDoor(w World, m DoorUser) *BreakDoor {
	return &BreakDoor{DoorInteract: DoorInteract{base: base{mob: m, world: w}, m: m}}
}

// CanUse implements goal.Goal (0x48CA070). The width test reads the value
// the last start left.
func (g *BreakDoor) CanUse() bool {
	if !g.DoorInteract.CanUse() || g.have && g.wide {
		return false
	}
	if !g.world.MobGriefing() {
		return false
	}
	return g.angry()
}

// angry is the shared test: a target, and no mob that hurt this one.
func (g *BreakDoor) angry() bool {
	m := g.m
	return g.actor(m.Target()) != nil && g.actor(m.LastHurtByMob()) == nil
}

// CanContinueToUse implements goal.Goal (0x48CA310).
func (g *BreakDoor) CanContinueToUse() bool {
	if !g.angry() || g.time > breakDoorTime {
		return false
	}
	d := Vec3{X: float32(g.door.X), Y: float32(g.door.Y), Z: float32(g.door.Z)}
	if dist2(g.m.Pos(), d) >= breakDoorReach2 {
		return false
	}
	return g.blocking() || g.wide
}

// blocking is 0x576E040: the door still closes the path's way in or out.
func (g *BreakDoor) blocking() bool {
	st, ok := g.m.Door(g.door)
	if !ok {
		return false
	}
	side := doorSide(st)
	return side == g.in || side == g.out
}

// Start implements goal.Goal (0x48CA150).
func (g *BreakDoor) Start() {
	g.DoorInteract.Start()
	g.time = 0
	if g.m.Difficulty() == difficultyHard {
		g.m.DoorEvent(eventDoorCrack, Vec3{X: float32(g.door.X), Y: float32(g.door.Y), Z: float32(g.door.Z)}, breakDoorCrackSpeed)
	}
}

// Stop implements goal.Goal (0x48CA250).
func (g *BreakDoor) Stop() {
	if g.m.Difficulty() == difficultyHard {
		g.m.DoorEvent(eventDoorCrackStop, Vec3{X: float32(g.door.X), Y: float32(g.door.Y), Z: float32(g.door.Z)}, 0)
	}
}

// Tick implements goal.Goal (0x48CA400): the mob knocks now and then, and
// after breakDoorTime ticks the door breaks.
func (g *BreakDoor) Tick() {
	g.DoorInteract.Tick()
	w := g.world
	centre := Vec3{X: float32(g.door.X) + 0.5, Y: float32(g.door.Y) + 0.5, Z: float32(g.door.Z) + 0.5}
	if w.IRandom().NextInt(breakDoorKnock) == 0 {
		g.m.DoorEvent(eventDoorKnock, centre, 0)
	}
	g.time++
	if g.time != breakDoorTime || g.m.Difficulty() != difficultyHard {
		return
	}
	door := w.Block(g.door)
	w.SetBlock(g.door, PlacedBlock{Name: airName}, g.m.ID())
	g.m.DoorEvent(eventDoorBreak, centre, 0)
	w.LevelEvent(eventDestroyBlock, centre, door.Name)
}

// RestrictOpenDoor is RestrictOpenDoorGoal: at night, while the mob is
// under cover, its navigation may not open or pass doors.
type RestrictOpenDoor struct{ base }

// NewRestrictOpenDoor makes the goal.
func NewRestrictOpenDoor(w World, m Mob) *RestrictOpenDoor {
	return &RestrictOpenDoor{base: base{mob: m, world: w}}
}

// indoorsAtNight is the shared test of canUse and canContinue: night (or
// no sky) and the mob below the column's height map.
func (g *RestrictOpenDoor) indoorsAtNight() bool {
	w := g.world
	if w.IsDay() {
		return false
	}
	b := blockOf(g.mob.Pos())
	if b.Y >= w.MaxHeight() {
		return false
	}
	return b.Y < w.MinHeight() || b.Y < w.HeightMap(b.X, b.Z)
}

// CanUse implements goal.Goal (0x5925190).
func (g *RestrictOpenDoor) CanUse() bool { return g.mob.Nav() != nil && g.indoorsAtNight() }

// CanContinueToUse implements goal.Goal (0x59253A0).
func (g *RestrictOpenDoor) CanContinueToUse() bool { return g.indoorsAtNight() }

// Start implements goal.Goal (0x59254E0).
func (g *RestrictOpenDoor) Start() {
	if n := g.mob.Nav(); n != nil {
		n.SetFlags(n.Flags() &^ uint32(nav.FlagCanOpenDoors|nav.FlagCanPassDoors))
	}
}

// Stop implements goal.Goal (0x59255C0): both switches come back on, even
// when they were off before.
func (g *RestrictOpenDoor) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.SetFlags(n.Flags() | uint32(nav.FlagCanOpenDoors|nav.FlagCanPassDoors))
	}
}

// PlayerRideTamed is PlayerVehicleTamedGoal: a tamed mob with a rider does
// nothing else.
type PlayerRideTamed struct{ base }

// NewPlayerRideTamed makes the goal.
func NewPlayerRideTamed(w World, m Mob) *PlayerRideTamed {
	return &PlayerRideTamed{base: base{mob: m, world: w}}
}

// CanUse implements goal.Goal (0x575C1C0).
func (g *PlayerRideTamed) CanUse() bool { return len(g.mob.Riders()) > 0 && g.mob.Flag(FlagTamed) }

// CanContinueToUse implements goal.Goal (0x575C200).
func (g *PlayerRideTamed) CanContinueToUse() bool { return g.CanUse() }
