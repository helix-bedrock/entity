package behavior

import (
	"testing"

	"github.com/helix-bedrock/entity/path"
)

// dragonWorld is a calmWorld with players, a podium top at 64 and clear
// sight.
type dragonWorld struct {
	calmWorld
	players []Actor
}

func (w dragonWorld) Players(dst []Actor) []Actor    { return append(dst, w.players...) }
func (dragonWorld) AboveTopSolid(int32, int32) int32 { return 64 }
func (dragonWorld) MinHeight() int32                 { return -64 }
func (dragonWorld) ClearLine(Vec3, Vec3) bool        { return true }

// Block gives the floor a material that is not air.
func (w dragonWorld) Block(p Pos) Block {
	b := w.calmWorld.Block(p)
	if b.Solid {
		b.Material = path.Material10
	}
	return b
}

// dragonMob is a b2Mob with the dragon's hooks over a ring of 24 nodes.
type dragonMob struct {
	*b2Mob
	fly                  Vec3
	crystals, flames     int32
	nodes                []Pos
	paths                [][2]int
	finals               []Pos
	air, turn            float32
	yaw, yawSpeed        float32
	look                 Vec3
	landings, takeoffs   int
	health               float32
	events, particles    []int32
	shots                []ID
	breath               []Vec3
	hCollide, sittingSet bool
}

func newDragon(w World) *dragonMob {
	d := &dragonMob{b2Mob: newB2Mob(w, 1, Vec3{X: 60, Y: 90, Z: 0}, "minecraft:ender_dragon"), crystals: 10, health: 200}
	for i := range 24 {
		d.nodes = append(d.nodes, Pos{X: int32(i) * 10, Y: 90, Z: 0})
	}
	return d
}

func (d *dragonMob) FlyTarget() Vec3           { return d.fly }
func (d *dragonMob) SetFlyTarget(p Vec3)       { d.fly = p }
func (d *dragonMob) CrystalsAlive() int32      { return d.crystals }
func (d *dragonMob) FlameCount() int32         { return d.flames }
func (d *dragonMob) SetFlameCount(n int32)     { d.flames = n }
func (d *dragonMob) SetSitting(on bool)        { d.flags[FlagSitting], d.sittingSet = on, true }
func (d *dragonMob) Nodes() []Pos              { return d.nodes }
func (d *dragonMob) ClosestNode() int          { return 0 }
func (d *dragonMob) SetAirSpeed(v float32)     { d.air = v }
func (d *dragonMob) SetTurnSpeed(v float32)    { d.turn = v }
func (d *dragonMob) HeadLook(float32) Vec3     { return d.look }
func (d *dragonMob) HeadBox() Box              { return Box{Min: d.pos, Max: d.pos} }
func (d *dragonMob) Yaw() float32              { return d.yaw }
func (d *dragonMob) SetYaw(v float32)          { d.yaw = v }
func (d *dragonMob) YawSpeed() float32         { return d.yawSpeed }
func (d *dragonMob) SetYawSpeed(v float32)     { d.yawSpeed = v }
func (d *dragonMob) HorizontalCollision() bool { return d.hCollide }
func (d *dragonMob) VerticalCollision() bool   { return false }
func (d *dragonMob) StartLanding()             { d.landings++ }
func (d *dragonMob) StartTakeoff()             { d.takeoffs++ }
func (d *dragonMob) Health() float32           { return d.health }
func (d *dragonMob) SetHealth(v float32)       { d.health = v }
func (d *dragonMob) ActorEvent(id, _ int32)    { d.events = append(d.events, id) }
func (d *dragonMob) ShootFireball(t Actor)     { d.shots = append(d.shots, t.ID()) }
func (d *dragonMob) Particle(id int32, _ Vec3) { d.particles = append(d.particles, id) }
func (d *dragonMob) SpawnBreath(p Vec3, _ float32, _ int32, _ string) {
	d.breath = append(d.breath, p)
}

// DragonPath is a straight run of nodes, then the final node.
func (d *dragonMob) DragonPath(dst []Pos, from, to int, final Pos, hasFinal bool) []Pos {
	d.paths = append(d.paths, [2]int{from, to})
	step := 1
	if to < from {
		step = -1
	}
	for i := from; i != to+step; i += step {
		dst = append(dst, d.nodes[i])
	}
	if hasFinal {
		d.finals = append(d.finals, final)
		dst = append(dst, final)
	}
	return dst
}

func newDragonTest() (*dragonMob, *playerMob, *dragonWorld, *flatWorld) {
	fw := &flatWorld{r: &seqRandom{}}
	w := &dragonWorld{calmWorld: calmWorld{fw}}
	d := newDragon(w)
	p := &playerMob{newB2Mob(w, 2, Vec3{X: 5, Y: 65, Z: 0}, "minecraft:player")}
	fw.entities = []Actor{d, p}
	w.players = []Actor{p}
	return d, p, w, fw
}

func TestDragonHoldingPatternCircles(t *testing.T) {
	d, _, w, fw := newDragonTest()
	w.players = nil
	g := NewDragonHoldingPattern(w, d, DefaultDragonHoldingPattern())
	if !g.CanUse() {
		t.Fatal("did not start flying with no target")
	}
	g.Start()
	fw.r.ints = []int32{1}
	fw.r.floats = []float32{0.5}
	g.Tick()
	// Not turning: one node on, the first node skipped, raised by half of 20.
	if len(d.paths) != 1 || d.paths[0] != [2]int{0, 11} {
		t.Fatalf("paths %v, want 0 to 11 around the outer ring", d.paths)
	}
	if d.fly != (Vec3{X: 10, Y: 100, Z: 0}) {
		t.Fatalf("fly target %v, want node 1 raised by 10", d.fly)
	}
	// Close to the target: the next node.
	d.pos = Vec3{X: 11, Y: 100}
	g.Tick()
	if d.fly.X != 20 {
		t.Fatalf("fly target %v, want node 2", d.fly)
	}
	// Path done: 1 in (crystals + 3) lands.
	g.next = len(g.path)
	d.pos = d.fly
	fw.r.ints = []int32{0}
	g.Tick()
	if d.landings != 1 {
		t.Fatal("did not land on a zero roll")
	}
}

func TestDragonHoldingPatternPicksPlayer(t *testing.T) {
	d, p, w, fw := newDragonTest()
	g := NewDragonHoldingPattern(w, d, DefaultDragonHoldingPattern())
	g.Start()
	g.Tick()
	g.next = len(g.path)
	d.pos = d.fly
	// No landing, then the distance roll picks the player.
	fw.r.ints = []int32{1, 0}
	g.Tick()
	if d.target != p.ID() {
		t.Fatalf("target %v, want the player near the podium", d.target)
	}
	if g.CanContinueToUse() {
		t.Fatal("kept circling with a target")
	}
}

func TestDragonTakeoffAndLanding(t *testing.T) {
	d, _, w, _ := newDragonTest()
	d.flags[FlagSitting] = true
	d.pos = Vec3{X: 0.5, Y: 66, Z: 0.5}
	d.look = Vec3{X: -1}
	g := NewDragonTakeoff(w, d, DefaultDragonTakeoff())
	if !g.CanUse() || !g.CanContinueToUse() {
		t.Fatal("did not take off from the podium")
	}
	g.Start()
	if d.flags[FlagSitting] {
		t.Fatal("still sitting")
	}
	g.Tick()
	// Looking -x aims 40 blocks +x at y 105: node 4, kept on the outer ring.
	if len(d.paths) != 1 || d.paths[0] != [2]int{0, 4} || d.fly.X != 0 {
		t.Fatalf("paths %v fly %v, want 0 to 4 starting at node 0", d.paths, d.fly)
	}

	d2, _, w2, _ := newDragonTest()
	w2.players = nil
	l := NewDragonLanding(w2, d2, DefaultDragonLanding())
	if !l.CanUse() {
		t.Fatal("did not land while flying")
	}
	l.Start()
	if d2.air != 0.6 || d2.turn != 0.7 {
		t.Fatalf("air %v turn %v", d2.air, d2.turn)
	}
	for range 10 {
		d2.pos = d2.fly
		l.Tick()
		if l.landing {
			break
		}
	}
	if !l.landing || d2.fly != (Vec3{Y: 64}) || d2.air != 1.5 || d2.finals[0] != (Pos{Y: 64}) {
		t.Fatalf("landing %v fly %v air %v finals %v, want the drop onto the podium", l.landing, d2.fly, d2.air, d2.finals)
	}
	d2.pos = Vec3{X: 0.2, Y: 64}
	if l.CanContinueToUse() || !d2.flags[FlagSitting] {
		t.Fatal("did not perch on the podium")
	}
}

func TestDragonScanningTargetsNearPlayer(t *testing.T) {
	d, p, w, _ := newDragonTest()
	d.flags[FlagSitting] = true
	d.pos = Vec3{X: 0.5, Y: 64, Z: 0.5}
	g := NewDragonScanning(w, d, DefaultDragonScanning())
	if !g.CanUse() {
		t.Fatal("did not scan while perched")
	}
	g.Start()
	g.Tick()
	if d.yawSpeed == 0 {
		t.Fatal("did not turn toward the player")
	}
	if !g.CanContinueToUse() {
		t.Fatal("stopped before the notice time")
	}
	g.time = 26
	if g.CanContinueToUse() || d.target != p.ID() {
		t.Fatalf("target %v, want the near player", d.target)
	}
}

func TestDragonFlamingBreathes(t *testing.T) {
	d, p, w, _ := newDragonTest()
	d.target = p.ID()
	d.pos = Vec3{X: 0.5, Y: 70, Z: 0.5}
	c := DefaultDragonFlaming()
	g := NewDragonFlaming(w, d, c)
	if !g.CanUse() {
		t.Fatal("did not flame with a target")
	}
	g.Start()
	for range c.RoarTime + c.FlameTime {
		g.Tick()
	}
	if len(d.sounds) != int(c.RoarTime) || len(d.events) != 4 || len(d.breath) != 1 {
		t.Fatalf("roars %d flame events %d breath %v", len(d.sounds), len(d.events), d.breath)
	}
	if d.breath[0].Y != 64 {
		t.Fatalf("breath at %v, want on the floor", d.breath[0])
	}
	g.flame = c.Cooldown
	d.flames = c.GroundFlameCount
	if g.CanContinueToUse() || d.takeoffs != 1 {
		t.Fatal("did not take off after the last flame")
	}
}

func TestDragonChargeThreshold(t *testing.T) {
	d, p, w, _ := newDragonTest()
	d.flags[FlagSitting] = true
	d.target = p.ID()
	g := NewDragonChargePlayer(w, d, DefaultDragonChargePlayer())
	if !g.CanUse() {
		t.Fatal("did not charge")
	}
	g.Start()
	if d.fly != p.pos || d.air != 3 {
		t.Fatalf("fly %v air %v, want the player at the active speed", d.fly, d.air)
	}
	d.pos = Vec3{X: 6, Y: 65}
	g.Tick()
	for i := 0; i < 9; i++ {
		if !g.CanContinueToUse() {
			t.Fatalf("stopped after %d checks", i)
		}
	}
	if g.CanContinueToUse() {
		t.Fatal("kept charging past the threshold")
	}
}

func TestDragonStrafeShoots(t *testing.T) {
	d, p, w, _ := newDragonTest()
	d.target = p.ID()
	d.pos = Vec3{X: 5, Y: 85, Z: 20}
	d.yaw = 0 // facing -z, at the player
	g := NewDragonStrafePlayer(w, d, DefaultDragonStrafePlayer())
	if !g.CanUse() {
		t.Fatal("did not strafe")
	}
	g.Start()
	if len(d.finals) != 1 || d.finals[0].X != 5 {
		t.Fatalf("finals %v, want a last node above the player", d.finals)
	}
	for range 5 {
		g.Tick()
	}
	if len(d.shots) != 1 || !g.fired || g.CanContinueToUse() {
		t.Fatalf("shots %v fired %v, want one fireball after the view time", d.shots, g.fired)
	}
}

func TestDragonDeathAtPodium(t *testing.T) {
	d, _, w, _ := newDragonTest()
	d.health = 1
	g := NewDragonDeath(w, d, DefaultDragonDeath())
	if !g.CanUse() {
		t.Fatal("did not start dying")
	}
	g.Start()
	for range 10 {
		g.Tick()
	}
	if len(d.particles) != 1 || d.health != 1 || d.fly != (Vec3{Y: 64}) {
		t.Fatalf("particles %v health %v fly %v", d.particles, d.health, d.fly)
	}
	d.pos = Vec3{X: 1, Y: 65}
	g.Tick()
	if d.health != 0 || d.events[0] != actorEventDragonDeath || g.CanContinueToUse() {
		t.Fatalf("health %v events %v, want death at the podium", d.health, d.events)
	}
}
