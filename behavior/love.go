package behavior

import "math"

// The make_love and receive_love numbers.
const (
	actorTypeVillagerV2 = 0x1000300 // the villager type make_love looks for
	loveSearchXZ        = 8
	loveSearchY         = 3
	loveChance          = 500 // one in this many ticks a villager looks for a mate
	loveTime            = 300 // ticks the pair stays together before the baby
	loveNear2           = 2.25
	loveFollow2         = 9 // receive_love walks over when farther than this
	loveWalkSpeed       = 0.25
	lovePathSpeed       = 0.6
	loveLookYaw         = 10
	loveLookPitch       = 30
	loveHeartChance     = 35 // one in this many ticks shows the hearts
	actorEventLove      = 21 // the love hearts actor event
)

// Lover is the part of the mob make_love and receive_love need on top of
// Mob. The BDS fields are the villager's (Mob +640, +1096, +1097).
type Lover interface {
	Mob
	// LovePartner is the entity's love partner, NoID for none.
	LovePartner(id ID) ID
	// SetLovePartner sets the entity's love partner and its in-love flag
	// (0xB309560); NoID clears both.
	SetLovePartner(id, partner ID)
	// LoveLeader reports the partner that started the pairing (+1096).
	LoveLeader(id ID) bool
	SetLoveLeader(id ID, on bool)
	// Willing is the villager's willing flag (+1097).
	Willing(id ID) bool
	SetWilling(id ID, on bool)
	// HasBreedFood reports enough food in the inventory to breed
	// (0xBB22DE0 finds a slot); UseBreedFood eats it (0xBB233B0).
	HasBreedFood(id ID) bool
	UseBreedFood(id ID)
	// WakeBreedSensors is 0xBB23D70 on the partner: it resets the
	// sensing entries of component 0xADEC332E.
	WakeBreedSensors(id ID)
	// VillageWantsBabies reports the village of the dweller component and
	// whether it has room for more villagers (0xBD13AB0); dweller is false
	// when the mob has no dweller component.
	VillageWantsBabies() (wants, dweller bool)
	// SpawnBaby spawns a baby of the mob's type with the partner as its
	// other parent; false when none could be made.
	SpawnBaby(partner ID) bool
	// EntitiesOfType appends the entities of the BDS actor type whose boxes
	// touch b to dst, excluding self.
	EntitiesOfType(dst []Actor, actorType int, self ID, b Box) []Actor
	// ActorEvent broadcasts an actor event of the mob.
	ActorEvent(id, data int32)
}

// ReceiveLove is ReceiveLoveGoal: the partner that did not start the
// pairing walks to the other and shows hearts.
type ReceiveLove struct {
	base
	m Lover
}

// NewReceiveLove makes the goal.
func NewReceiveLove(w World, m Lover) *ReceiveLove {
	return &ReceiveLove{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m}
}

// CanUse implements goal.Goal (0x526E110).
func (g *ReceiveLove) CanUse() bool { return g.m.Flag(FlagInLove) }

// CanContinueToUse implements goal.Goal (0x526E130).
func (g *ReceiveLove) CanContinueToUse() bool { return g.m.Flag(FlagInLove) }

// Tick implements goal.Goal (0x526E150).
func (g *ReceiveLove) Tick() {
	m := g.m
	p := g.actor(m.LovePartner(m.ID()))
	if p == nil || !p.IsMob() {
		return
	}
	m.LookAt(lookPoint(p), loveLookYaw, loveLookPitch)
	if dist2(m.Pos(), p.Pos()) > loveFollow2 {
		if n := m.Nav(); n != nil {
			n.MoveToActor(p, loveWalkSpeed)
		}
	}
	if g.world.Random().NextInt(loveHeartChance) == 0 {
		m.ActorEvent(actorEventLove, 0)
	}
}

// MakeLove is MakeLoveGoal: a villager with food now and then pairs up
// with another nearby and they make a baby.
type MakeLove struct {
	base
	m     Lover
	timer int32
	near  []Actor
}

// NewMakeLove makes the goal.
func NewMakeLove(w World, m Lover) *MakeLove {
	return &MakeLove{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m}
}

// villageOK is the dweller test: no dweller component, or a village with
// room.
func (g *MakeLove) villageOK() bool {
	wants, dweller := g.m.VillageWantsBabies()
	return !dweller || wants
}

// CanUse implements goal.Goal (0x526CDD0).
func (g *MakeLove) CanUse() bool {
	m := g.m
	if m.Nav() == nil {
		return false
	}
	if m.Flag(FlagInLove) && !m.LoveLeader(m.ID()) || m.Flag(FlagBaby) {
		return false
	}
	if g.world.Random().NextInt(loveChance) != 0 || !g.villageOK() {
		return false
	}
	p := g.find()
	if p == nil {
		return false
	}
	m.SetLovePartner(m.ID(), p.ID())
	m.SetLoveLeader(m.ID(), true)
	m.SetLovePartner(p.ID(), m.ID())
	m.SetLoveLeader(p.ID(), false)
	m.WakeBreedSensors(p.ID())
	return true
}

// ready reports a villager that can breed now: willing, or with food.
func (g *MakeLove) ready(id ID) bool { return g.m.Willing(id) || g.m.HasBreedFood(id) }

// find is 0x526D1C0: a villager already paired with this one, or else the
// nearest free one the mob can walk to. Walking there starts on the way.
func (g *MakeLove) find() Actor {
	m := g.m
	b := m.Box()
	area := Box{Min: Vec3{X: b.Min.X - loveSearchXZ, Y: b.Min.Y - loveSearchY, Z: b.Min.Z - loveSearchXZ},
		Max: Vec3{X: b.Max.X + loveSearchXZ, Y: b.Max.Y + loveSearchY, Z: b.Max.Z + loveSearchXZ}}
	g.near = m.EntitiesOfType(g.near[:0], actorTypeVillagerV2, m.ID(), area)
	n := m.Nav()
	var best Actor
	bestD := float32(math.MaxFloat32)
	for _, c := range g.near {
		if partner := m.LovePartner(c.ID()); partner != NoID {
			if partner == m.ID() {
				return c
			}
			continue
		}
		if n == nil {
			continue
		}
		d := dist2(c.Pos(), m.Pos())
		if d > bestD || c.Flag(FlagBaby) || !g.ready(c.ID()) {
			continue
		}
		if !n.MoveToActor(c, lovePathSpeed) || !pathEndsUnder(n, c) {
			continue
		}
		best, bestD = c, d
	}
	return best
}

// pathEndsUnder reports that the path's last node is in the column of a.
func pathEndsUnder(n Navigator, a Actor) bool {
	end, ok := n.PathEnd()
	p := a.Pos()
	return ok && end.X == floorInt(p.X) && end.Z == floorInt(p.Z)
}

// CanContinueToUse implements goal.Goal (0x526D720).
func (g *MakeLove) CanContinueToUse() bool {
	m := g.m
	if g.timer < 0 || m.Flag(FlagBaby) || !m.Flag(FlagInLove) || !g.villageOK() {
		return false
	}
	p := g.actor(m.LovePartner(m.ID()))
	if p == nil || !p.IsMob() || !g.ready(p.ID()) {
		return false
	}
	if dist2(m.Pos(), p.Pos()) <= loveNear2 {
		return true
	}
	n := m.Nav()
	if n == nil {
		return true
	}
	if pathEndsUnder(n, p) {
		return true
	}
	return n.MoveToActor(p, lovePathSpeed) && pathEndsUnder(n, p)
}

// Start implements goal.Goal (0x526D6A0).
func (g *MakeLove) Start() { g.timer = loveTime }

// Stop implements goal.Goal (0x526D6B0): both villagers fall out of love.
func (g *MakeLove) Stop() {
	m := g.m
	if p := m.LovePartner(m.ID()); g.actor(p) != nil {
		m.SetLovePartner(p, NoID)
	}
	m.SetLovePartner(m.ID(), NoID)
}

// Tick implements goal.Goal (0x526DAB0).
func (g *MakeLove) Tick() {
	m := g.m
	g.timer--
	p := g.actor(m.LovePartner(m.ID()))
	if p == nil || !p.IsMob() {
		return
	}
	m.LookAt(lookPoint(p), loveLookYaw, loveLookPitch)
	if g.timer == 0 && p.Flag(FlagInLove) && dist2(m.Pos(), p.Pos()) <= loveNear2 {
		if m.SpawnBaby(p.ID()) {
			for _, id := range [...]ID{m.ID(), p.ID()} {
				if !m.Willing(id) {
					m.UseBreedFood(id)
				}
				m.SetWilling(id, false)
			}
		}
	}
	if g.world.Random().NextInt(loveHeartChance) == 0 {
		m.ActorEvent(actorEventLove, 0)
	}
}

// The base Goal methods ReceiveLove keeps (1.26.60 vtable: start and stop are no-ops).

// Start implements goal.Goal.
func (g *ReceiveLove) Start() {}

// Stop implements goal.Goal.
func (g *ReceiveLove) Stop() {}
