package behavior

import "testing"

// vWorld adds the village facts to flatWorld.
type vWorld struct {
	*flatWorld
	sky     bool
	players []Actor
}

func newVWorld(r *seqRandom) *vWorld {
	return &vWorld{flatWorld: &flatWorld{r: r}, sky: true}
}

func (w *vWorld) HeightMap(_, _ int32) int32  { return 64 }
func (w *vWorld) SkyVisible(Pos) bool         { return w.sky }
func (w *vWorld) MobGriefing() bool           { return true }
func (w *vWorld) Players(dst []Actor) []Actor { return append(dst, w.players...) }

// villager is fakeMob with every village hook.
type villager struct {
	*fakeMob
	pois     map[POIKind]POI
	results  []bool
	village  Box
	inVil    bool
	dir      int32
	sleeping bool
	collider [2]float32
	yaw      float32
	vel      Vec3
	look     Vec3
	looks    int
	sounds   []string
	stamps   int
	mingles  map[ID]MingleState
	grown    map[Pos]bool
	planted  []Pos
	seed     int32
	fired    []Vec3
	offers   map[ID]int32
	stopped  []ID
	held     map[ID]Item
	shown    Item
	trading  ID
}

func newVillager(w World, id ID, p Vec3) *villager {
	return &villager{fakeMob: newMob(w, id, p, "minecraft:villager_v2"), pois: map[POIKind]POI{},
		mingles: map[ID]MingleState{}, grown: map[Pos]bool{}, offers: map[ID]int32{}, held: map[ID]Item{},
		seed: -1, trading: NoID}
}

func (m *villager) IsDweller() bool                  { return true }
func (m *villager) POI(k POIKind) (POI, bool)        { p, ok := m.pois[k]; return p, ok }
func (m *villager) FreePOI(POIKind, Pos) (POI, bool) { return POI{}, false }
func (m *villager) POIResult(_ POIKind, ok bool)     { m.results = append(m.results, ok) }
func (m *villager) Village(int32) (Box, bool)        { return m.village, m.inVil }
func (m *villager) BedDirection(Pos) (int32, bool)   { return m.dir, true }
func (m *villager) Sleeping() bool                   { return m.sleeping }
func (m *villager) SetSleeping(on bool)              { m.sleeping = on }
func (m *villager) SetSleepCollider(w, h float32)    { m.collider = [2]float32{w, h} }
func (m *villager) SetSleepYaw(y float32)            { m.yaw = y }
func (m *villager) HealOnWake()                      {}
func (m *villager) StandUpSpot(bed Pos, _ Vec3) (Pos, bool) {
	return Pos{X: bed.X + 1, Y: bed.Y, Z: bed.Z}, true
}
func (m *villager) Teleport(p Vec3)                      { m.pos = p }
func (m *villager) SetVelocity(v Vec3)                   { m.vel = v }
func (m *villager) LookBusy() bool                       { return false }
func (m *villager) LookAt(p Vec3, _, _ float32)          { m.look, m.looks = p, m.looks+1 }
func (m *villager) MaxHeadXRot() float32                 { return 40 }
func (m *villager) FollowRange() float32                 { return 16 }
func (m *villager) Eyes() Vec3                           { return add(m.pos, Vec3{Y: 1.6}) }
func (m *villager) AttachPos(int) Vec3                   { return add(m.pos, Vec3{Y: 1}) }
func (m *villager) PlaySoundAt(int, Vec3)                { m.sounds = append(m.sounds, "level") }
func (m *villager) RainingAt(Pos) bool                   { return false }
func (m *villager) PlayWorkSound(s string)               { m.sounds = append(m.sounds, s) }
func (m *villager) StampWork()                           { m.stamps++ }
func (m *villager) Mingle(id ID) (MingleState, bool)     { s, ok := m.mingles[id]; return s, ok }
func (m *villager) SetMingle(id ID, s MingleState)       { m.mingles[id] = s }
func (m *villager) MatchesBlock(p Pos, _ []string) bool  { return p.Y == 63 }
func (m *villager) Crop(p Pos) (bool, bool)              { g, ok := m.grown[p]; return g, ok }
func (m *villager) HasContainer() bool                   { return true }
func (m *villager) SeedSlot() int32                      { return m.seed }
func (m *villager) HarvestCrop(Pos) bool                 { return true }
func (m *villager) PlantSeed(_ int32, p Pos)             { m.planted = append(m.planted, p) }
func (m *villager) FertilizerSlot() int32                { return -1 }
func (m *villager) UseFertilizer(int32, Pos) bool        { return false }
func (m *villager) LaunchFirework(p Vec3, _, _ int32)    { m.fired = append(m.fired, p) }
func (m *villager) HasGoal(ID, string) bool              { return true }
func (m *villager) OfferFlower()                         { m.offers[m.id] = 0 }
func (m *villager) OfferFlowerTicks(id ID) (int32, bool) { t, ok := m.offers[id]; return t, ok }
func (m *villager) StopOfferingFlower(id ID, _ int32) {
	delete(m.offers, id)
	m.stopped = append(m.stopped, id)
}
func (m *villager) OfferDuration(ID) (int32, bool) { return 400, true }
func (m *villager) HasTradeTable() bool            { return true }
func (m *villager) HeldItem(id ID) Item            { return m.held[id] }
func (m *villager) CanSee(ID) bool                 { return true }
func (m *villager) SetCarriedItem(it Item)         { m.shown = it }
func (m *villager) LooksAtMe(ID) bool              { return false }
func (m *villager) TradingPlayer() ID              { return m.trading }
func (m *villager) TradeInterrupted() bool         { return false }
func (m *villager) Home() (Pos, int32, bool)       { return Pos{X: 10, Y: 64}, 0, true }
func (m *villager) TradeShow(held Item, from int) (Item, int, bool) {
	if held.Name != "minecraft:wheat" {
		return Item{}, from, false
	}
	return Item{Name: "minecraft:emerald", Count: 1}, from + 1, true
}

func TestMoveToPOIReachesTheBed(t *testing.T) {
	w := newVWorld(&seqRandom{floats: []float32{0}})
	m := newVillager(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5})
	m.pois[POIBed] = POI{Pos: Pos{X: 5, Y: 64}}
	g := NewMoveToPOI(w, m, DefaultMoveToPOI())
	if !g.CanUse() {
		t.Fatal("no path to the bed")
	}
	g.Start()
	if m.nav.to != (Vec3{X: 5, Y: 64}) {
		t.Fatalf("walked to %v", m.nav.to)
	}
	g.Stop()
	if len(m.results) != 1 || !m.results[0] {
		t.Fatalf("POI results %v", m.results)
	}
}

func TestSleepLiesInTheBedAndGetsUp(t *testing.T) {
	w := newVWorld(&seqRandom{floats: []float32{0}})
	m := newVillager(w, 1, Vec3{X: 5.5, Y: 64, Z: 0.5})
	m.pois[POIBed] = POI{Pos: Pos{X: 5, Y: 64}, Radius: 1}
	m.dir = 1
	g := NewSleep(w, m, DefaultSleep())
	if !g.CanUse() {
		t.Fatal("did not go to bed")
	}
	g.Start()
	g.Tick()
	if !m.sleeping || m.pos != (Vec3{X: 6.125, Y: 64, Z: 0.5}) || m.yaw != 270 {
		t.Fatalf("sleeping %v at %v yaw %v", m.sleeping, m.pos, m.yaw)
	}
	m.SetFlag(FlagWantsToWake, true)
	if g.CanContinueToUse() {
		t.Fatal("kept sleeping after the wake request")
	}
	g.Stop()
	if m.sleeping || m.pos != (Vec3{X: 6.5, Y: 64, Z: 0.5}) || m.Flag(FlagWantsToWake) {
		t.Fatalf("after waking: sleeping %v at %v", m.sleeping, m.pos)
	}
	if g.CanUse() {
		t.Fatal("slept again with no cooldown passed")
	}
}

func TestWorkArrivesStampsAndPlaysTheSound(t *testing.T) {
	w := newVWorld(&seqRandom{floats: []float32{0}})
	m := newVillager(w, 1, Vec3{X: 3.5, Y: 64, Z: 0.5})
	m.pois[POIJobSite] = POI{Pos: Pos{X: 3, Y: 64}, Radius: 1, Sound: "block.barrel"}
	c := DefaultWork()
	c.OnArrival = []string{"arrived"}
	g := NewWork(w, m, c)
	if !g.CanUse() {
		t.Fatal("did not go to work")
	}
	g.Start()
	g.Tick()
	g.Tick()
	if m.stamps != 1 || len(m.events) != 1 || len(m.sounds) != 1 || m.sounds[0] != "block.barrel" {
		t.Fatalf("stamps %d events %v sounds %v", m.stamps, m.events, m.sounds)
	}
	if m.look != (Vec3{X: 3.5, Y: 65.5, Z: 0.5}) {
		t.Fatalf("looked at %v", m.look)
	}
}

func TestMinglePairsTwoVillagers(t *testing.T) {
	w := newVWorld(&seqRandom{floats: []float32{0}})
	a := newVillager(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5})
	b := newVillager(w, 2, Vec3{X: 1.5, Y: 64, Z: 0.5})
	w.entities = []Actor{a, b}
	a.pois[POIBed] = POI{Pos: Pos{Y: 64}, Radius: 4}
	a.mingles[2] = MingleState{State: mingleSearching, Partner: NoID, Last: NoID}
	c := DefaultMingle()
	c.MinglePartnerType = "minecraft:villager_v2"
	c.CooldownTime = 1
	g := NewMingle(w, a, c)
	g.left = 10
	g.search = mingleScanEvery + 1
	a.mingles[1] = MingleState{State: mingleSearching, Partner: NoID, Last: NoID}
	g.reached = true
	g.Tick()
	if s := a.mingles[1]; s.State != mingleApproaching || s.Partner != 2 {
		t.Fatalf("mine %+v", s)
	}
	if s := a.mingles[2]; s.State != minglePartnered || s.Partner != 1 {
		t.Fatalf("theirs %+v", s)
	}
}

func TestMoveToVillageWalksInFromOutside(t *testing.T) {
	w := newVWorld(&seqRandom{ints: []int32{0, 3, 4}, floats: []float32{0}})
	m := newVillager(w, 1, Vec3{X: -40.5, Y: 64, Z: 0.5})
	m.village, m.inVil = Box{Min: Vec3{X: 0, Y: 60, Z: 0}, Max: Vec3{X: 16, Y: 70, Z: 16}}, true
	g := NewMoveToVillage(w, m, DefaultMoveToVillage())
	if !g.CanUse() {
		t.Fatal("did not start outside the village")
	}
	g.Start()
	if g.aim != (Vec3{X: 3, Y: 64, Z: 4}) {
		t.Fatalf("target %v", g.aim)
	}
	m.pos = Vec3{X: 2.5, Y: 64, Z: 2.5}
	g.nextStart = 0
	if g.CanUse() {
		t.Fatal("started inside the village")
	}
}

func TestEdgePointWalksTheVillageRim(t *testing.T) {
	b := Box{Max: Vec3{X: 10, Z: 4}}
	for _, c := range []struct {
		r    int32
		x, z float32
	}{{3, 3, 0}, {12, 10, 2}, {16, 8, 4}, {26, 0, 2}} {
		if x, z := edgePoint(b, c.r); x != c.x || z != c.z {
			t.Errorf("edgePoint(%d) = %v, %v, want %v, %v", c.r, x, z, c.x, c.z)
		}
	}
}

func TestHarvestPlantsEmptyFarmland(t *testing.T) {
	w := newVWorld(&seqRandom{})
	m := newVillager(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5})
	m.seed = 2
	c := DefaultHarvestFarmBlock()
	c.SearchRange = 1
	g := NewHarvestFarmBlock(w, m, c)
	if !g.CanUse() || g.task != farmTaskPlant {
		t.Fatalf("no planting task (%d)", g.task)
	}
	g.Start()
	g.reached = true
	g.target = Pos{Y: 63}
	m.pos = Vec3{X: 0.5, Y: 63.5, Z: 0.5}
	g.Tick()
	if len(m.planted) != 1 || g.task != farmTaskNone || g.wait != 10 {
		t.Fatalf("planted %v task %d wait %d", m.planted, g.task, g.wait)
	}
}

func TestCelebrateSurviveLaunchesUnderTheSky(t *testing.T) {
	w := newVWorld(&seqRandom{floats: []float32{0}})
	m := newVillager(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5})
	g := NewCelebrateSurvive(w, m, DefaultCelebrateSurvive())
	g.Start()
	if !m.Flag(FlagCelebrating) {
		t.Fatal("not celebrating")
	}
	w.tick = 201
	w.sky = false
	g.Tick()
	if len(m.fired) != 0 {
		t.Fatal("fired under a roof")
	}
	w.sky = true
	g.Tick()
	if len(m.fired) != 1 || m.fired[0] != (Vec3{Y: 65}) {
		t.Fatalf("fired %v", m.fired)
	}
	g.Stop()
	if m.Flag(FlagCelebrating) {
		t.Fatal("still celebrating")
	}
}

func TestTakeFlowerWaitsThenTakes(t *testing.T) {
	w := newVWorld(&seqRandom{ints: []int32{0}})
	m := newVillager(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5})
	golem := newVillager(w, 2, Vec3{X: 1.5, Y: 64, Z: 0.5})
	w.entities = []Actor{m, golem}
	m.offers[2] = 0
	g := NewTakeFlower(w, m, DefaultTakeFlower())
	if !g.CanUse() || g.target != 2 {
		t.Fatal("did not see the offer")
	}
	g.Start()
	g.Tick()
	if g.walking {
		t.Fatal("walked before the wait was over")
	}
	m.offers[2] = 396
	g.Tick()
	if !g.walking || len(m.stopped) != 1 {
		t.Fatalf("walking %v taken %v", g.walking, m.stopped)
	}
}

func TestTradeInterestShowsTheTrade(t *testing.T) {
	w := newVWorld(&seqRandom{})
	m := newVillager(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5})
	p := &fakePlayer{newMob(w, 9, Vec3{X: 2.5, Y: 64, Z: 0.5}, "minecraft:player")}
	w.players = []Actor{p}
	w.entities = []Actor{m, p}
	m.held[9] = Item{Name: "minecraft:wheat", Count: 4}
	c := DefaultTradeInterest()
	c.WithinRadius = 6
	g := NewTradeInterest(w, m, c)
	if !g.CanUse() || m.shown.Name != "minecraft:emerald" {
		t.Fatalf("showed %v", m.shown)
	}
	g.Start()
	if !m.Flag(FlagTradeInterest) || !g.CanContinueToUse() {
		t.Fatal("lost interest at once")
	}
	g.Stop()
	if !m.shown.Empty() || m.Flag(FlagTradeInterest) {
		t.Fatal("still showing after stop")
	}
}

func TestGoHomeFiresOnHome(t *testing.T) {
	w := newVWorld(&seqRandom{})
	m := newVillager(w, 1, Vec3{X: 10.5, Y: 64.5, Z: 0.5})
	c := DefaultGoHome()
	c.Interval = 0
	c.OnHome = []string{"home"}
	g := NewGoHome(w, m, c)
	if !g.CanUse() || g.CanContinueToUse() {
		t.Fatal("at home but kept going")
	}
	g.Stop()
	if len(m.events) != 1 || m.events[0] != "home" {
		t.Fatalf("events %v", m.events)
	}
}

func TestMoveThroughVillageNeverRuns(t *testing.T) {
	w := newVWorld(&seqRandom{})
	g := NewMoveThroughVillage(w, newVillager(w, 1, Vec3{}), DefaultMoveThroughVillage())
	if g.CanUse() || g.CanContinueToUse() {
		t.Fatal("move_through_village ran")
	}
}
