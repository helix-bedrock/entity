package nav

import "github.com/helix-bedrock/entity/path"

// OpenDoorsConfig is the open_door annotation's numbers. The JSON component
// has no fields; these are BDS's constants.
type OpenDoorsConfig struct {
	// LeaveReach2 is the squared horizontal distance from the oldest door's
	// centre past which the mob closes it.
	LeaveReach2 float32
	// EnterReach2 is the squared horizontal distance from the current node's
	// centre within which a door there is opened.
	EnterReach2 float32
}

// DefaultOpenDoors returns BDS's numbers.
func DefaultOpenDoors() OpenDoorsConfig {
	return OpenDoorsConfig{LeaveReach2: 2.25, EnterReach2: 1}
}

// DoorToggler sets the door at p open or closed as the mob, with the door's
// own sound and events (BDS's code,, with the
// actor). The engine may apply it later in the tick.
type DoorToggler interface {
	SetDoorOpen(p path.Pos, open bool)
}

// OpenDoors is BDS's OpenDoorAnnotationComponent and its system
// (OpenDoorAnnotationSystem, tick): the minecraft:annotation.open_door
// component. Unlike the open_door goal, it needs no goal slot: every tick it
// opens a door on the node the mob is walking to and closes the doors it
// opened once the mob is past them. Mobs with it have actor flag 81
// (DoorOpener) set, which the pathfinder (path.Settings.DoorOpener) and path
// following (Mob.OpensDoors) read.
//
// Runs: the mob's tick, like its navigation.
type OpenDoors struct {
	cfg OpenDoorsConfig
	// doors are the doors the mob toggled, oldest first (BDS keeps a deque
	// of 12-byte BlockPos entries).
	doors []path.Pos
}

// NewOpenDoors makes the component with c.
func NewOpenDoors(c OpenDoorsConfig) *OpenDoors { return &OpenDoors{cfg: c} }

// Config returns the component's numbers.
func (o *OpenDoors) Config() OpenDoorsConfig { return o.cfg }

// SetConfig changes the component's numbers, from the mob's tick.
func (o *OpenDoors) SetConfig(c OpenDoorsConfig) { o.cfg = c }

// Doors returns the doors the mob opened that it has not closed yet, oldest
// first. The slice is the component's own.
func (o *OpenDoors) Doors() []path.Pos { return o.doors }

// Reset forgets the doors without closing them, as BDS does when the
// component is removed.
func (o *OpenDoors) Reset() { o.doors = o.doors[:0] }

// Tick is the system's tick for one mob. BDS runs it only for a
// mob that is not removed; the caller checks that. The doors toggle through t.
//
// BDS quirks kept: without a path every remembered door is forgotten but only
// the oldest one is toggled back; closing toggles, so a door a player shut in
// the meantime opens again; a door is remembered on reaching it even when it
// did not need to toggle.
func (o *OpenDoors) Tick(w World, m *Mob, n *Navigation, t DoorToggler) {
	if n == nil || n.path == nil || n.index >= len(n.path.Points) {
		o.closeOldest(w, t)
		o.doors = o.doors[:0]
		return
	}
	if len(o.doors) > 0 && horizontalDist2(m.Pos, o.doors[0]) > o.cfg.LeaveReach2 {
		o.closeOldest(w, t)
	}
	idx := min(n.index, len(n.path.Points)-1)
	at := n.path.Points[idx].Pos
	info := w.Info(w.Block(at))
	if info.Props&path.PropDoor == 0 || horizontalDist2(m.Pos, at) >= o.cfg.EnterReach2 || n.Flags&FlagCanPassDoors == 0 {
		return
	}
	if !mayOpen(m, n, info.Kind) {
		return
	}
	if passDoor(w, m, n.path, idx) {
		toggleDoor(w, at, t)
	}
	if last := len(o.doors) - 1; last < 0 || o.doors[last] != at {
		o.doors = append(o.doors, at)
	}
}

// mayOpen reports whether the mob may open a door of kind k: iron doors need
// can_open_iron_doors, other doors the DoorOpener flag and can_open_doors.
func mayOpen(m *Mob, n *Navigation, k path.Kind) bool {
	if k == path.KindIronDoor {
		return n.Flags&FlagCanOpenIronDoors != 0
	}
	return m.OpensDoors && n.Flags&FlagCanOpenDoors != 0
}

// closeOldest is: it toggles the oldest door, if it is still a
// door, and forgets it.
func (o *OpenDoors) closeOldest(w World, t DoorToggler) {
	if len(o.doors) == 0 {
		return
	}
	p := o.doors[0]
	if w.Info(w.Block(p)).Props&path.PropDoor != 0 {
		toggleDoor(w, p, t)
	}
	copy(o.doors, o.doors[1:])
	o.doors = o.doors[:len(o.doors)-1]
}

// toggleDoor is: the door at p is set to the opposite of its open
// state.
func toggleDoor(w World, p path.Pos, t DoorToggler) {
	_, open, _ := w.Door(p)
	t.SetDoorOpen(p, !open)
}

// horizontalDist2 is the squared distance from v to the centre of block p,
// leaving height out (BDS computes the y term as v.y - v.y).
func horizontalDist2(v path.Vec3, p path.Pos) float32 {
	dx := v.X - (float32(p.X) + 0.5)
	dz := v.Z - (float32(p.Z) + 0.5)
	return dz*dz + dx*dx
}
