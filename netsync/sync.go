package netsync

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/helix-bedrock/protocol"
	"github.com/helix-bedrock/protocol/packet"
)

// EntityID is an entity's unique ID.
type EntityID int64

// ViewerID is a viewing player's unique ID.
type ViewerID int64

// Writer is the engine's write hook for one viewer. It gets one batch per
// tick, in the order the packets should be written. The packets are only
// valid until the next Flush; the engine writes or copies them before
// that.
type Writer interface {
	WriteBatch(pks []packet.Packet)
}

// Phase is the part of a tick a packet goes out in.
type Phase uint8

// The phases of a BDS tick. BDS ticks every actor first: events, swings,
// equipment changes and a player's own view changes (AddActor, RemoveActor)
// go out then, in the order they happen. Then it sends every actor's
// dirty data, then every actor's movement, then the tick's sounds. Order
// confirmed live with gdb.
const (
	PhaseTick Phase = iota
	PhaseData
	PhaseMovement
	PhaseSound
	phaseCount
)

// VanillaOrder is BDS's phase order.
func VanillaOrder() []Phase {
	return []Phase{PhaseTick, PhaseData, PhaseMovement, PhaseSound}
}

// Options are the vanilla behaviours Sync can apply. Each is the engine's
// choice: VanillaOptions turns all of them on, the zero value turns all of
// them off.
type Options struct {
	// Thresholds, when set, only sends moves that are significant. Nil sends
	// every Move call. A teleport is always sent.
	Thresholds *Thresholds
	// Throttle skips movement updates for far viewers by each entity's
	// Bandwidth.
	Throttle bool
	// Deltas sends MoveActorDelta. When false, every move is a
	// MoveActorAbsolute.
	Deltas bool
	// DeltaTicks goes in each MoveActorDelta's Ticks field. BDS's value was
	// not read.
	DeltaTicks uint64
	// SpawnEquipment sends a mob's armour and held items right after its
	// AddActor.
	SpawnEquipment bool
	// DeadEvent sends ActorEvent 3 after the spawn packets of an entity
	// that is not alive.
	DeadEvent bool
	// EquipmentChanges broadcasts an equipment change at once to everyone
	// who sees the mob: armour as one MobArmourEquipment, a hand as one
	// MobEquipment.
	EquipmentChanges bool
	// Order is the order of the phases in each viewer's batch. Nil uses
	// VanillaOrder. A phase left out is not sent.
	Order []Phase
}

// VanillaOptions is BDS's behaviour.
func VanillaOptions() Options {
	t := DefaultThresholds()
	return Options{
		Thresholds:       &t,
		Throttle:         true,
		Deltas:           true,
		SpawnEquipment:   true,
		DeadEvent:        true,
		EquipmentChanges: true,
		Order:            VanillaOrder(),
	}
}

// Entity is an entity's settings in Sync.
type Entity struct {
	Spawn Spawn
	// Mob entities send head yaw, body yaw and equipment.
	Mob       bool
	Equipment Equipment
	// Bandwidth throttles the entity's movement for far viewers. Nil never
	// throttles it, as BDS does for an entity without
	// minecraft:conditional_bandwidth_optimization.
	Bandwidth *Bandwidth
	// AlwaysSend sends every movement update to every viewer, skipping the
	// throttle (a flag component in BDS, checked in sub_8CFC460).
	AlwaysSend bool
	// Dead marks an entity that is not alive.
	Dead bool
	// ZeroRotation starts the last-sent rotation at 0, as BDS does for
	// actor type 64 (sub_8CE9830).
	ZeroRotation bool
}

type entityState struct {
	e    Entity
	last Sent
	// move is the index of this tick's pending move, or -1.
	move int
}

type known struct {
	// lastTick is the tick of the last movement update this viewer got.
	lastTick uint64
}

type viewerState struct {
	id    ViewerID
	w     Writer
	pos   mgl32.Vec3
	known map[EntityID]*known
	out   [phaseCount][]packet.Packet
	batch []packet.Packet
}

type pendingMove struct {
	id     EntityID
	pose   Pose
	motion mgl32.Vec3
	flags  MoveFlags
	// skipThrottle sends to every viewer this tick.
	skipThrottle bool
	// dropped marks the move of an entity removed before Flush.
	dropped bool
}

// Sync keeps what each viewer last saw of each entity and builds the
// packets the engine asks for, the BDS way. The engine decides who sees
// what (See, Unsee), what moved (Move) and what else to send (Broadcast,
// Send); Flush writes it all out.
//
// A Sync is not safe for concurrent use. Use one per dimension or region,
// called from the goroutine that ticks it. Output depends only on the order
// of calls, so it is the same on one core and on many.
type Sync struct {
	opts      Options
	entities  map[EntityID]*entityState
	viewers   []*viewerState
	viewerIdx map[ViewerID]*viewerState

	moves     []pendingMove
	deltas    []packet.MoveActorDelta
	absolutes []packet.MoveActorAbsolute
}

// New returns an empty Sync.
func New(o Options) *Sync {
	return &Sync{
		opts:      o,
		entities:  make(map[EntityID]*entityState),
		viewerIdx: make(map[ViewerID]*viewerState),
	}
}

// Options returns the options in use.
func (s *Sync) Options() Options { return s.opts }

// SetOptions changes the options; they apply from the next call.
func (s *Sync) SetOptions(o Options) { s.opts = o }

// AddEntity starts tracking an entity at pose p, which becomes its
// last-sent state. It returns false if the ID is already tracked.
func (s *Sync) AddEntity(e Entity, p Pose) bool {
	id := EntityID(e.Spawn.UniqueID)
	if _, ok := s.entities[id]; ok {
		return false
	}
	last := Quantize(p, e.Mob)
	if e.ZeroRotation {
		last.Pitch, last.Yaw, last.HeadYaw, last.BodyYaw = 0, 0, 0, 0
	}
	s.entities[id] = &entityState{e: e, last: last, move: -1}
	return true
}

// SetEntity replaces a tracked entity's settings and keeps its last-sent
// state. It sends nothing; equipment changes go through SetArmour and
// SetHand.
func (s *Sync) SetEntity(e Entity) bool {
	es, ok := s.entities[EntityID(e.Spawn.UniqueID)]
	if ok {
		es.e = e
	}
	return ok
}

// Entity returns a tracked entity's settings.
func (s *Sync) Entity(id EntityID) (Entity, bool) {
	es, ok := s.entities[id]
	if !ok {
		return Entity{}, false
	}
	return es.e, true
}

// Last returns what viewers last saw of an entity.
func (s *Sync) Last(id EntityID) (Sent, bool) {
	es, ok := s.entities[id]
	if !ok {
		return Sent{}, false
	}
	return es.last, true
}

// RemoveEntity unsees the entity for every viewer that sees it, so each
// gets a RemoveActor, and stops tracking it.
func (s *Sync) RemoveEntity(id EntityID) bool {
	if _, ok := s.entities[id]; !ok {
		return false
	}
	for _, v := range s.viewers {
		s.unsee(v, id)
	}
	s.dropMove(id)
	delete(s.entities, id)
	return true
}

// AddViewer adds a viewer at pos that sees nothing yet.
func (s *Sync) AddViewer(id ViewerID, w Writer, pos mgl32.Vec3) bool {
	if _, ok := s.viewerIdx[id]; ok || w == nil {
		return false
	}
	v := &viewerState{id: id, w: w, pos: pos, known: make(map[EntityID]*known)}
	s.viewers = append(s.viewers, v)
	s.viewerIdx[id] = v
	return true
}

// RemoveViewer forgets a viewer and drops its unsent packets. It sends no
// RemoveActors: the viewer is gone.
func (s *Sync) RemoveViewer(id ViewerID) bool {
	v, ok := s.viewerIdx[id]
	if !ok {
		return false
	}
	delete(s.viewerIdx, id)
	for i, o := range s.viewers {
		if o == v {
			s.viewers = append(s.viewers[:i], s.viewers[i+1:]...)
			break
		}
	}
	return true
}

// MoveViewer sets the position the throttle measures a viewer from.
func (s *Sync) MoveViewer(id ViewerID, pos mgl32.Vec3) bool {
	v, ok := s.viewerIdx[id]
	if ok {
		v.pos = pos
	}
	return ok
}

// Sees reports whether a viewer sees an entity.
func (s *Sync) Sees(viewer ViewerID, entity EntityID) bool {
	v, ok := s.viewerIdx[viewer]
	if !ok {
		return false
	}
	_, ok = v.known[entity]
	return ok
}

// See makes a viewer see an entity: AddActor, then by the options its
// equipment and the dead event, in the tick phase. It returns false if the
// viewer or entity is unknown or already seen.
func (s *Sync) See(viewer ViewerID, entity EntityID) bool {
	v, ok := s.viewerIdx[viewer]
	if !ok {
		return false
	}
	es, ok := s.entities[entity]
	if !ok {
		return false
	}
	if _, seen := v.known[entity]; seen {
		return false
	}
	v.known[entity] = &known{}
	add := AddActor(es.e.Spawn, es.last)
	v.out[PhaseTick] = append(v.out[PhaseTick], &add)
	if es.e.Mob && s.opts.SpawnEquipment {
		v.out[PhaseTick] = SpawnEquipment(es.e.Spawn.RuntimeID, es.e.Equipment, v.out[PhaseTick])
	}
	if es.e.Dead && s.opts.DeadEvent {
		ev := ActorEvent(es.e.Spawn.RuntimeID, ActorEventDead, 0)
		v.out[PhaseTick] = append(v.out[PhaseTick], &ev)
	}
	return true
}

// Unsee makes a viewer stop seeing an entity: RemoveActor in the tick
// phase.
func (s *Sync) Unsee(viewer ViewerID, entity EntityID) bool {
	v, ok := s.viewerIdx[viewer]
	if !ok {
		return false
	}
	return s.unsee(v, entity)
}

func (s *Sync) unsee(v *viewerState, entity EntityID) bool {
	if _, seen := v.known[entity]; !seen {
		return false
	}
	delete(v.known, entity)
	rm := RemoveActor(int64(entity))
	v.out[PhaseTick] = append(v.out[PhaseTick], &rm)
	return true
}

// Move tells Sync where an entity is now. Flush decides, by the options,
// whether and to whom the move is sent. A second Move for the same entity
// before Flush replaces the first.
func (s *Sync) Move(id EntityID, p Pose, motion mgl32.Vec3, f MoveFlags) bool {
	return s.move(id, pendingMove{id: id, pose: p, motion: motion, flags: f})
}

// MoveUnthrottled is Move, sent to every viewer that sees the entity this
// tick. BDS does this when the on-ground state changed, and when a value it
// tracks per entity changed (not identified yet).
func (s *Sync) MoveUnthrottled(id EntityID, p Pose, motion mgl32.Vec3, f MoveFlags) bool {
	return s.move(id, pendingMove{id: id, pose: p, motion: motion, flags: f, skipThrottle: true})
}

func (s *Sync) move(id EntityID, m pendingMove) bool {
	es, ok := s.entities[id]
	if !ok {
		return false
	}
	if es.move >= 0 && es.move < len(s.moves) {
		s.moves[es.move] = m
		return true
	}
	es.move = len(s.moves)
	s.moves = append(s.moves, m)
	return true
}

func (s *Sync) dropMove(id EntityID) {
	es, ok := s.entities[id]
	if !ok || es.move < 0 || es.move >= len(s.moves) {
		return
	}
	// Keep the slot so other entities' indexes stay valid; Flush skips it.
	s.moves[es.move].dropped = true
	es.move = -1
}

// Broadcast queues a packet for every viewer that sees the entity, in the
// given phase. It returns how many viewers got it.
func (s *Sync) Broadcast(entity EntityID, pk packet.Packet, phase Phase) int {
	if phase >= phaseCount || pk == nil {
		return 0
	}
	n := 0
	for _, v := range s.viewers {
		if _, seen := v.known[entity]; seen {
			v.out[phase] = append(v.out[phase], pk)
			n++
		}
	}
	return n
}

// Send queues a packet for one viewer in the given phase, such as a sound
// the engine decided this viewer hears.
func (s *Sync) Send(viewer ViewerID, pk packet.Packet, phase Phase) bool {
	v, ok := s.viewerIdx[viewer]
	if !ok || phase >= phaseCount || pk == nil {
		return false
	}
	v.out[phase] = append(v.out[phase], pk)
	return true
}

// SetArmour stores a mob's armour and, with EquipmentChanges, broadcasts
// it at once.
func (s *Sync) SetArmour(id EntityID, a [ArmourSlots]protocol.ItemInstance) bool {
	es, ok := s.entities[id]
	if !ok {
		return false
	}
	es.e.Equipment.Armour = a
	if s.opts.EquipmentChanges {
		pk := ArmourEquipment(es.e.Spawn.RuntimeID, a)
		s.Broadcast(id, &pk, PhaseTick)
	}
	return true
}

// SetHand stores a mob's main-hand (offHand false) or off-hand item and,
// with EquipmentChanges, broadcasts it at once.
func (s *Sync) SetHand(id EntityID, offHand bool, h Hand) bool {
	es, ok := s.entities[id]
	if !ok {
		return false
	}
	if offHand {
		es.e.Equipment.OffHand = h
	} else {
		es.e.Equipment.MainHand = h
	}
	if s.opts.EquipmentChanges {
		pk := HandEquipment(es.e.Spawn.RuntimeID, h)
		s.Broadcast(id, &pk, PhaseTick)
	}
	return true
}

// Flush works out this tick's movement and writes each viewer one batch,
// in viewer order. tick is the server tick, used by the throttle.
func (s *Sync) Flush(tick uint64) {
	s.flushMoves(tick)
	order := s.opts.Order
	if order == nil {
		order = VanillaOrder()
	}
	for _, v := range s.viewers {
		batch := v.batch[:0]
		for _, p := range order {
			if p < phaseCount {
				batch = append(batch, v.out[p]...)
			}
		}
		for p := range v.out {
			clear(v.out[p])
			v.out[p] = v.out[p][:0]
		}
		v.batch = batch
		if len(batch) > 0 {
			v.w.WriteBatch(batch)
		}
	}
}

func (s *Sync) flushMoves(tick uint64) {
	n := len(s.moves)
	// Packets are shared by every viewer that gets them, so the buffers
	// must not grow while pointers into them are handed out.
	if cap(s.deltas) < n {
		s.deltas = make([]packet.MoveActorDelta, 0, n)
	}
	if cap(s.absolutes) < n {
		s.absolutes = make([]packet.MoveActorAbsolute, 0, n)
	}
	s.deltas, s.absolutes = s.deltas[:0], s.absolutes[:0]
	for _, m := range s.moves {
		if m.dropped {
			continue
		}
		es, ok := s.entities[m.id]
		if !ok || es.move < 0 {
			continue
		}
		es.move = -1
		s.sendMove(es, m, tick)
	}
	clear(s.moves)
	s.moves = s.moves[:0]
}

func (s *Sync) sendMove(es *entityState, m pendingMove, tick uint64) {
	now := Quantize(m.pose, es.e.Mob)
	if !m.flags.Teleported && s.opts.Thresholds != nil && !Significant(es.last, now, m.motion, *s.opts.Thresholds) {
		return
	}
	rid := es.e.Spawn.RuntimeID
	if !s.opts.Deltas {
		// BDS sends an absolute update to everyone and always stores it
		// (sub_8CE8C80).
		s.absolutes = append(s.absolutes, Absolute(rid, now, m.flags))
		s.Broadcast(m.id, &s.absolutes[len(s.absolutes)-1], PhaseMovement)
		es.last = now
		return
	}
	s.deltas = append(s.deltas, Delta(rid, es.last, now, m.flags, s.opts.DeltaTicks))
	pk := &s.deltas[len(s.deltas)-1]
	throttle := s.opts.Throttle && es.e.Bandwidth != nil && !es.e.AlwaysSend &&
		!m.skipThrottle && now.OnGround == es.last.OnGround
	sent := false
	for _, v := range s.viewers {
		k, seen := v.known[m.id]
		if !seen {
			continue
		}
		if throttle && !Due(tick, k.lastTick, Score(Distance(m.pose.Position, v.pos), *es.e.Bandwidth)) {
			continue
		}
		v.out[PhaseMovement] = append(v.out[PhaseMovement], pk)
		k.lastTick = tick
		sent = true
	}
	// BDS only moves last-sent forward when at least one player got the
	// delta, so the next one is still measured from what viewers saw.
	if sent {
		es.last = now
	}
}
