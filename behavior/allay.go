package behavior

// NoteblockListener is a mob that remembers the last note block it heard
// (VibrationDataComponent).
type NoteblockListener interface {
	Mob
	BlockMatcher
	// HeardNoteblock is where and on which tick the mob last heard a note
	// block; ok is false when it remembers none.
	HeardNoteblock() (p Pos, tick uint64, ok bool)
	// ForgetNoteblock drops the memory.
	ForgetNoteblock()
}

// ItemGiver is a mob that carries items to someone and throws them.
type ItemGiver interface {
	Mob
	// Inventory is the mob's ContainerComponent, nil without one.
	Inventory() Container
	// ThrowItem spawns the stack as an item entity at the mob's throw
	// point with velocity v, and runs the item's drop events and
	// achievements (BDS gives the owner one for a cake thrown at a note
	// block). ok is false when no entity was made.
	ThrowItem(it Item, v Vec3, toNoteblock bool) bool
}

// noteblockDescriptors is the block BDS compares the heard spot with.
func noteblockDescriptors() []string { return []string{"minecraft:noteblock"} }

// heardNoteblock is the listen check the noteblock goals share
// (0x5692E80): a memory younger than listenSeconds whose block is still a
// note block. An old memory is forgotten.
func heardNoteblock(w World, m NoteblockListener, listenSeconds int32, blocks []string) (Pos, bool) {
	p, tick, ok := m.HeardNoteblock()
	if !ok {
		return Pos{}, false
	}
	if w.CurrentTick()-tick > uint64(listenSeconds)*ticksPerSecond {
		m.ForgetNoteblock()
		return Pos{}, false
	}
	return p, m.MatchesBlock(p, blocks)
}

func blockCentre(p Pos) Vec3 {
	return Vec3{X: float32(p.X) + 0.5, Y: float32(p.Y) + 0.5, Z: float32(p.Z) + 0.5}
}

// StayNearNoteblockConfig is the JSON of
// minecraft:behavior.stay_near_noteblock.
type StayNearNoteblockConfig struct {
	Speed                       float32
	StartDistance, StopDistance float32
	// ListenTime is in seconds.
	ListenTime int32
	// Noteblock is the block descriptor the heard spot must match.
	Noteblock []string
	// LookYaw is the look control's yaw limit; the pitch limit is the
	// mob's MaxHeadXRot.
	LookYaw float32
	// Repath is how many ticks pass between paths.
	Repath int32
}

// DefaultStayNearNoteblock returns the goal's BDS defaults (definition
// default 0xF0270D0, numbers from tick 0x56932C0).
func DefaultStayNearNoteblock() StayNearNoteblockConfig {
	return StayNearNoteblockConfig{Speed: 1, StartDistance: 10, StopDistance: 2, ListenTime: 30,
		Noteblock: noteblockDescriptors(), LookYaw: 10, Repath: 10}
}

// StayNearNoteblock is StayNearNoteblockGoal (canUse 0x5692C80,
// canContinue 0x5693010, start 0x56931B0, stop 0x56931C0, tick 0x56932C0):
// the mob goes back to the note block it heard when it is farther than
// start_distance.
type StayNearNoteblock struct {
	base
	m     NoteblockListener
	cfg   StayNearNoteblockConfig
	timer int32
}

// NewStayNearNoteblock makes the goal.
func NewStayNearNoteblock(w World, m NoteblockListener, c StayNearNoteblockConfig) *StayNearNoteblock {
	return &StayNearNoteblock{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// Config returns the goal's settings.
func (g *StayNearNoteblock) Config() StayNearNoteblockConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *StayNearNoteblock) SetConfig(c StayNearNoteblockConfig) { g.cfg = c }

// CanUse implements goal.Goal.
func (g *StayNearNoteblock) CanUse() bool {
	if g.m.Nav() == nil {
		return false
	}
	p, ok := heardNoteblock(g.world, g.m, g.cfg.ListenTime, g.cfg.Noteblock)
	return ok && g.cfg.StartDistance*g.cfg.StartDistance <= dist2(g.m.Pos(), blockCentre(p))
}

// CanContinueToUse implements goal.Goal: while a path runs and the mob is
// beyond stop_distance.
func (g *StayNearNoteblock) CanContinueToUse() bool {
	n := g.m.Nav()
	p, ok := heardNoteblock(g.world, g.m, g.cfg.ListenTime, g.cfg.Noteblock)
	return ok && n != nil && !n.Done() && dist2(g.m.Pos(), blockCentre(p)) > g.cfg.StopDistance*g.cfg.StopDistance
}

// Start implements goal.Goal.
func (g *StayNearNoteblock) Start() { g.timer = 0 }

// Stop implements goal.Goal.
func (g *StayNearNoteblock) Stop() {
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
}

// Tick implements goal.Goal: look at the remembered block, and walk to it
// every Repath ticks. BDS reads the memory here without the listen check.
func (g *StayNearNoteblock) Tick() {
	p, _, ok := g.m.HeardNoteblock()
	if !ok {
		return
	}
	c := blockCentre(p)
	g.m.LookAt(c, g.cfg.LookYaw, g.m.MaxHeadXRot())
	g.timer--
	if g.timer+1 > 1 {
		return
	}
	g.timer = g.cfg.Repath
	if n := g.m.Nav(); n != nil {
		n.MoveTo(c, g.cfg.Speed)
	}
}

// GiveItemsConfig is the JSON the two give goals share.
type GiveItemsConfig struct {
	RunSpeed float32
	// ReachDistance is reach_block_distance or reach_mob_distance.
	ReachDistance float32
	ThrowForce    float32
	// VerticalThrowMul scales the throw's upward part.
	VerticalThrowMul float32
	ThrowSound       int
	// OnItemThrow are the events fired on the mob each tick it throws.
	OnItemThrow []string
	// SoundEvery and SoundChance gate the throw sound: on level ticks that
	// divide by SoundEvery, when an IRandom float is below SoundChance.
	SoundEvery  uint64
	SoundChance float32
}

func defaultGiveItems() GiveItemsConfig {
	return GiveItemsConfig{RunSpeed: 1, ReachDistance: 3, ThrowForce: 0.2, VerticalThrowMul: 1.5, ThrowSound: 134,
		SoundEvery: 7, SoundChance: 0.9}
}

// throwAll is the throw both give goals share: the on_item_throw events,
// then one item from every filled slot towards to.
func throwAll(w World, m ItemGiver, c *GiveItemsConfig, inv Container, to Vec3, toNoteblock bool) {
	for _, e := range c.OnItemThrow {
		m.Event(e, NoID)
	}
	for k := range inv.Size() {
		it := inv.Slot(k)
		if it.Empty() {
			continue
		}
		rest := it
		rest.Count--
		if rest.Count <= 0 {
			rest = Item{}
		}
		inv.SetSlot(k, rest)
		d := normalize(sub(to, m.Pos()))
		v := Vec3{X: d.X * c.ThrowForce, Y: d.Y * c.VerticalThrowMul * c.ThrowForce, Z: d.Z * c.ThrowForce}
		if !m.ThrowItem(it.one(), v, toNoteblock) {
			continue
		}
		if c.SoundEvery > 0 && w.CurrentTick()%c.SoundEvery == 0 && w.IRandom().NextFloat() < c.SoundChance {
			m.PlaySoundAt(c.ThrowSound, m.Eyes())
		}
	}
}

func hasItems(inv Container) bool {
	if inv == nil {
		return false
	}
	for k := range inv.Size() {
		if !inv.Slot(k).Empty() {
			return true
		}
	}
	return false
}

// NoteblockGiver is the mob go_and_give_items_to_noteblock runs on.
type NoteblockGiver interface {
	ItemGiver
	NoteblockListener
}

// GiveToNoteblockConfig is the JSON of
// minecraft:behavior.go_and_give_items_to_noteblock.
type GiveToNoteblockConfig struct {
	GiveItemsConfig
	// ListenTime is in seconds.
	ListenTime int32
	Noteblock  []string
}

// DefaultGiveToNoteblock returns the goal's BDS defaults (definition
// default 0xE817DB0, create 0x5740680, numbers from tick 0x59FDFC0).
func DefaultGiveToNoteblock() GiveToNoteblockConfig {
	return GiveToNoteblockConfig{GiveItemsConfig: defaultGiveItems(), ListenTime: 30, Noteblock: noteblockDescriptors()}
}

// GiveToNoteblock is GoAndGiveItemsToNoteblockGoal (canUse 0x59FD660,
// canContinue 0x59FDB90, start 0x59FDE10, stop 0x59FE980, tick
// 0x59FDFC0): the mob carries its items to the note block it heard and
// throws them onto it.
type GiveToNoteblock struct {
	base
	m    NoteblockGiver
	cfg  GiveToNoteblockConfig
	done bool
	at   Pos
}

// NewGiveToNoteblock makes the goal.
func NewGiveToNoteblock(w World, m NoteblockGiver, c GiveToNoteblockConfig) *GiveToNoteblock {
	return &GiveToNoteblock{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, done: true}
}

// Config returns the goal's settings.
func (g *GiveToNoteblock) Config() GiveToNoteblockConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *GiveToNoteblock) SetConfig(c GiveToNoteblockConfig) { g.cfg = c }

// CanUse implements goal.Goal: items, a heard note block and a path to it.
func (g *GiveToNoteblock) CanUse() bool {
	n := g.m.Nav()
	if n == nil || !hasItems(g.m.Inventory()) {
		return false
	}
	p, ok := heardNoteblock(g.world, g.m, g.cfg.ListenTime, g.cfg.Noteblock)
	if !ok {
		return false
	}
	g.at = p
	return n.CanPathTo(blockCentre(p))
}

// CanContinueToUse implements goal.Goal.
func (g *GiveToNoteblock) CanContinueToUse() bool {
	if g.done || !hasItems(g.m.Inventory()) {
		return false
	}
	n := g.m.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal.
func (g *GiveToNoteblock) Start() {
	if n := g.m.Nav(); n != nil {
		n.MoveTo(blockCentre(g.at), g.cfg.RunSpeed)
	}
	g.done = false
}

// Stop implements goal.Goal.
func (g *GiveToNoteblock) Stop() {
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
}

// Tick implements goal.Goal: walk until within reach, then throw at the
// top of the block.
func (g *GiveToNoteblock) Tick() {
	n, inv := g.m.Nav(), g.m.Inventory()
	if n == nil || inv == nil {
		g.done = true
		return
	}
	if g.done {
		return
	}
	if g.cfg.ReachDistance <= length(sub(g.m.Pos(), blockCentre(g.at))) {
		if n.Done() {
			g.done = true
		}
		return
	}
	top := blockCentre(Pos{X: g.at.X, Y: g.at.Y + 1, Z: g.at.Z})
	throwAll(g.world, g.m, &g.cfg.GiveItemsConfig, inv, top, true)
}

// GiveToOwnerConfig is the JSON of
// minecraft:behavior.go_and_give_items_to_owner.
type GiveToOwnerConfig struct {
	GiveItemsConfig
	// Repath is how many ticks pass between paths to the owner.
	Repath uint64
}

// DefaultGiveToOwner returns the goal's BDS defaults (definition default
// 0xE818DE0, numbers from tick 0x59FF300).
func DefaultGiveToOwner() GiveToOwnerConfig {
	return GiveToOwnerConfig{GiveItemsConfig: defaultGiveItems(), Repath: 10}
}

// GiveToOwner is GoAndGiveItemsToOwnerGoal (canUse 0x59FEB10, canContinue
// 0x59FEEE0, start 0x59FF140, stop 0x5A00050, tick 0x59FF300, throw
// 0x59FF5C0): the mob carries its items to its owner and throws them to
// it.
type GiveToOwner struct {
	base
	m        ItemGiver
	cfg      GiveToOwnerConfig
	done     bool
	lastPath uint64
}

// NewGiveToOwner makes the goal.
func NewGiveToOwner(w World, m ItemGiver, c GiveToOwnerConfig) *GiveToOwner {
	return &GiveToOwner{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, done: true}
}

// Config returns the goal's settings.
func (g *GiveToOwner) Config() GiveToOwnerConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *GiveToOwner) SetConfig(c GiveToOwnerConfig) { g.cfg = c }

// owner is the owner when it is a player.
func (g *GiveToOwner) owner() Actor {
	o := g.actor(g.m.Owner())
	if o == nil || !o.IsPlayer() {
		return nil
	}
	return o
}

// CanUse implements goal.Goal: items, a player owner, and a path to it
// unless the mob is already within reach.
func (g *GiveToOwner) CanUse() bool {
	o := g.owner()
	n := g.m.Nav()
	if o == nil || n == nil || !hasItems(g.m.Inventory()) {
		return false
	}
	_, path := n.PathTo(o)
	g.lastPath = g.world.CurrentTick()
	return g.cfg.ReachDistance > length(sub(g.m.Pos(), o.Pos())) || path
}

// CanContinueToUse implements goal.Goal.
func (g *GiveToOwner) CanContinueToUse() bool {
	if g.done || g.actor(g.m.Owner()) == nil || !hasItems(g.m.Inventory()) {
		return false
	}
	n := g.m.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal.
func (g *GiveToOwner) Start() {
	if o, n := g.owner(), g.m.Nav(); o != nil && n != nil {
		n.MoveToActor(o, g.cfg.RunSpeed)
	}
	g.done = false
}

// Stop implements goal.Goal.
func (g *GiveToOwner) Stop() {
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
}

// Tick implements goal.Goal: throw when within reach, else keep a path
// to the owner, renewed every Repath ticks.
func (g *GiveToOwner) Tick() {
	n := g.m.Nav()
	if n == nil {
		g.done = true
		return
	}
	if g.done {
		return
	}
	o := g.actor(g.m.Owner())
	if o == nil {
		return
	}
	if inv := g.m.Inventory(); g.cfg.ReachDistance > length(sub(g.m.Pos(), o.Pos())) {
		if inv != nil {
			throwAll(g.world, g.m, &g.cfg.GiveItemsConfig, inv, add(o.Pos(), Vec3{Y: 1}), false)
		}
		return
	}
	if n.Done() {
		g.done = true
		return
	}
	now := g.world.CurrentTick()
	if g.lastPath+g.cfg.Repath >= now {
		return
	}
	if !n.MoveToActor(o, g.cfg.RunSpeed) {
		g.done = true
	}
	g.lastPath = now
}
