package behavior

import (
	"testing"

	"github.com/helix-bedrock/entity/nav"
)

// flagNav is fakeNav with real flags and a path end.
type flagNav struct {
	fakeNav
	flags uint32
}

func (n *flagNav) Flags() uint32     { return n.flags }
func (n *flagNav) SetFlags(f uint32) { n.flags = f }

// b12Mob adds the door and love hooks to b11bMob.
type b12Mob struct {
	*b11bMob
	nodes   []Pos
	idx     int
	doors   map[Pos]DoorState
	setOpen []bool
	width   float32
	diff    int
	partner map[ID]ID
	leader  map[ID]bool
	willing map[ID]bool
	babies  int
	near    []Actor
	nv      *flagNav
}

func newB12Mob(w World, p Vec3) *b12Mob {
	m := &b12Mob{b11bMob: newB11bMob(w, p, "minecraft:villager_v2"), doors: map[Pos]DoorState{},
		width: 0.6, partner: map[ID]ID{}, leader: map[ID]bool{}, willing: map[ID]bool{}, nv: &flagNav{}}
	return m
}

func (m *b12Mob) Nav() Navigator                      { return m.nv }
func (m *b12Mob) UsesDoors() bool                     { return true }
func (m *b12Mob) PathNodes() ([]Pos, int, bool)       { return m.nodes, m.idx, len(m.nodes) > 0 }
func (m *b12Mob) Door(p Pos) (DoorState, bool)        { d, ok := m.doors[p]; return d, ok }
func (m *b12Mob) SetDoorOpen(_ Pos, open bool)        { m.setOpen = append(m.setOpen, open) }
func (m *b12Mob) Width() float32                      { return m.width }
func (m *b12Mob) Difficulty() int                     { return m.diff }
func (m *b12Mob) DoorEvent(id int32, _ Vec3, _ int32) { m.actorEv = append(m.actorEv, id) }
func (m *b12Mob) LovePartner(id ID) ID {
	if p, ok := m.partner[id]; ok {
		return p
	}
	return NoID
}
func (m *b12Mob) SetLovePartner(id, p ID) {
	m.partner[id] = p
	if id == m.id {
		m.flags[FlagInLove] = p != NoID
	}
}
func (m *b12Mob) LoveLeader(id ID) bool            { return m.leader[id] }
func (m *b12Mob) SetLoveLeader(id ID, on bool)     { m.leader[id] = on }
func (m *b12Mob) Willing(id ID) bool               { return m.willing[id] }
func (m *b12Mob) SetWilling(id ID, on bool)        { m.willing[id] = on }
func (m *b12Mob) HasBreedFood(ID) bool             { return false }
func (m *b12Mob) UseBreedFood(ID)                  {}
func (m *b12Mob) WakeBreedSensors(ID)              {}
func (m *b12Mob) VillageWantsBabies() (bool, bool) { return true, true }
func (m *b12Mob) SpawnBaby(ID) bool                { m.babies++; return true }
func (m *b12Mob) EntitiesOfType(dst []Actor, _ int, _ ID, _ Box) []Actor {
	return append(dst, m.near...)
}

func TestOpenDoorOpensAndClosesTheDoorOnThePath(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB12Mob(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	m.nv.flags = uint32(nav.FlagCanOpenDoors)
	door := Pos{Y: 65, Z: 1}
	m.doors[door] = DoorState{Direction: 1}
	// The path walks +z through the door; a closed direction-1 door blocks
	// side 0, the way in.
	m.nodes = []Pos{{Y: 64}, {Y: 64, Z: 1}, {Y: 64, Z: 2}}
	m.idx = 1
	g := NewOpenDoor(w, m, DefaultOpenDoor())
	if !g.CanUse() || g.door != door {
		t.Fatalf("found no door, at %v", g.door)
	}
	g.Start()
	if len(m.setOpen) != 1 || !m.setOpen[0] {
		t.Fatalf("door opens %v", m.setOpen)
	}
	m.pos.Z = 2.5
	g.Tick()
	if g.CanContinueToUse() {
		t.Fatal("still at the door once through it")
	}
	g.Stop()
	if len(m.setOpen) != 2 || m.setOpen[1] {
		t.Fatalf("door not closed behind: %v", m.setOpen)
	}
}

func TestDoorSideFollowsHingeAndOpenState(t *testing.T) {
	cases := []struct {
		s    DoorState
		want int8
	}{
		{DoorState{Direction: 0}, 3},
		{DoorState{Direction: 0, Open: true, Hinge: true}, 2},
		{DoorState{Direction: 2, Open: true}, 2},
		{DoorState{Direction: 3, Open: true, Hinge: true}, 1},
		{DoorState{Direction: 7}, noDir},
	}
	for _, c := range cases {
		if got := doorSide(c.s); got != c.want {
			t.Errorf("doorSide %+v = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestBreakDoorBreaksOnlyOnHard(t *testing.T) {
	w := newB11World(&seqRandom{ints: []int32{1}})
	m := newB12Mob(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	door := Pos{Y: 65, Z: 1}
	g := NewBreakDoor(w, m)
	g.door, g.have = door, true
	m.diff = difficultyHard
	g.Start()
	for range breakDoorTime {
		g.Tick()
	}
	if len(w.placed) != 1 || w.placed[0] != door {
		t.Fatalf("broke %v", w.placed)
	}
	m.diff = 2
	g.Start()
	for range breakDoorTime {
		g.Tick()
	}
	if len(w.placed) != 1 {
		t.Fatal("broke a door below hard")
	}
}

func TestRestrictOpenDoorTogglesTheDoorFlags(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB12Mob(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	m.nv.flags = uint32(nav.FlagCanOpenDoors | nav.FlagCanPassDoors | nav.FlagCanJump)
	g := NewRestrictOpenDoor(w, m)
	g.Start()
	if m.nv.flags != uint32(nav.FlagCanJump) {
		t.Fatalf("flags after start %#x", m.nv.flags)
	}
	m.nv.flags = 0
	g.Stop()
	if m.nv.flags != uint32(nav.FlagCanOpenDoors|nav.FlagCanPassDoors) {
		t.Fatalf("flags after stop %#x", m.nv.flags)
	}
}

func TestPlayerRideTamedNeedsARiderAndTaming(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB12Mob(w, Vec3{})
	g := NewPlayerRideTamed(w, m)
	m.flags[FlagTamed] = true
	if g.CanUse() {
		t.Fatal("ran with no rider")
	}
}

func TestMakeLovePairsWithAWillingVillager(t *testing.T) {
	w := newB11World(&seqRandom{ints: []int32{0}})
	m := newB12Mob(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	other := newB12Mob(w, Vec3{X: 2.5, Y: 64, Z: 0.5})
	other.id = 5
	w.entities = append(w.entities, other)
	m.near = []Actor{other}
	m.willing[other.id] = true
	g := NewMakeLove(w, m)
	if !g.CanUse() {
		t.Fatal("no pairing")
	}
	if m.LovePartner(m.id) != other.id || m.LovePartner(other.id) != m.id || !m.LoveLeader(m.id) {
		t.Fatalf("partners %v leaders %v", m.partner, m.leader)
	}
	other.flags[FlagInLove] = true
	m.pos = Vec3{X: 1.5, Y: 64, Z: 0.5}
	g.Start()
	for range loveTime {
		g.Tick()
	}
	if m.babies != 1 || m.willing[other.id] {
		t.Fatalf("babies %d, willing %v", m.babies, m.willing)
	}
}

// b12Pet adds the pet sleep and mount hooks to b12Mob.
type b12Pet struct {
	*b12Mob
	bed      Pos
	inBed    bool
	claimed  bool
	told     int
	yaw      float32
	body     float32
	vel      Vec3
	accepts  bool
	riding   ID
	gaveUp   int
	followed int
}

func newB12Pet(w World, p Vec3) *b12Pet {
	return &b12Pet{b12Mob: newB12Mob(w, p), accepts: true, riding: NoID}
}

func (m *b12Pet) OwnerBed() (Pos, bool)          { return m.bed, m.inBed }
func (m *b12Pet) OwnerSleeping() bool            { return m.inBed }
func (m *b12Pet) ClaimBed(Pos) bool              { m.claimed = true; return true }
func (m *b12Pet) TellOwner()                     { m.told++ }
func (m *b12Pet) SetExactPathEnd(float32)        {}
func (m *b12Pet) SetPathSpeed(float32)           {}
func (m *b12Pet) Velocity() Vec3                 { return m.vel }
func (m *b12Pet) SetVelocity(v Vec3)             { m.vel = v }
func (m *b12Pet) Yaw() float32                   { return m.yaw }
func (m *b12Pet) SetYaw(y float32)               { m.yaw = y }
func (m *b12Pet) BodyYaw() float32               { return m.body }
func (m *b12Pet) SetBodyYaw(y float32)           { m.body = y }
func (m *b12Pet) AcceptsRider(ID) bool           { return m.accepts }
func (m *b12Pet) Sleeping(ID) bool               { return false }
func (m *b12Pet) Wet(ID) bool                    { return false }
func (m *b12Pet) CanRide(ID) bool                { return true }
func (m *b12Pet) StartRiding(id ID)              { m.riding = id }
func (m *b12Pet) GiveUpRiding()                  { m.gaveUp++ }
func (m *b12Pet) PathToActor(Actor) (bool, bool) { return true, true }
func (m *b12Pet) FollowPath(float32)             { m.followed++ }
func (m *b12Pet) Vehicle() ID                    { return m.riding }

func TestPetSleepWithOwnerHopsOntoTheBed(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB12Pet(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	owner := newB12Mob(w, Vec3{X: 3.5, Y: 64, Z: 0.5})
	owner.id = 8
	w.entities = append(w.entities, owner)
	m.owner = owner.id
	m.flags[FlagTamed] = true
	m.bed, m.inBed = Pos{X: 3, Y: 64}, true
	g := NewPetSleepWithOwner(w, m, DefaultPetSleepWithOwner())
	if !g.CanUse() || !m.claimed {
		t.Fatal("did not go to the owner's bed")
	}
	g.Start()
	if m.told != 1 || g.state != petSleepApproach {
		t.Fatalf("state %d, told %d", g.state, m.told)
	}
	m.pos = Vec3{X: 3.5, Y: 63.6, Z: 0.5} // box middle at the bed centre
	for range petSleepHop + 2 {
		g.Tick()
	}
	if g.state != petSleepSleeping || !m.Flag(FlagResting) {
		t.Fatalf("state %d after the hop", g.state)
	}
	g.Stop()
	if m.Flag(FlagResting) || g.state != petSleepWalk {
		t.Fatal("still lying down after stop")
	}
}

func TestFindMountRidesTheNearestMount(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB12Pet(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	far := newB12Mob(w, Vec3{X: 6.5, Y: 64, Z: 0.5})
	near := newB12Mob(w, Vec3{X: 1.5, Y: 64, Z: 0.5})
	far.id, near.id = 20, 21
	w.entities = append(w.entities, far, near)
	m.near = []Actor{far, near}
	c := DefaultFindMount()
	c.WithinRadius = 10
	g := NewFindMount(w, m, c)
	if !g.CanUse() || g.mount != near.id {
		t.Fatalf("picked %d", g.mount)
	}
	g.Start()
	g.Tick()
	if m.riding != near.id {
		t.Fatalf("riding %d", m.riding)
	}
}
