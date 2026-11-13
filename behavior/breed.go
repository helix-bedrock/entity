package behavior

import "math"

// The breed goal's fixed numbers.
const (
	breedBox     = 8
	breedEvery   = 60 // ticks between breeding tries; the goal ends after one
	breedLookYaw = 10
	breedBigHalf = 0.5
	breedBoxGap2 = 0.16
	breedGap     = 0.6
	breedFar2    = 9
)

// Breeder is the part of the mob the breed goal needs on top of Mob.
type Breeder interface {
	Mob
	// Breed makes the baby with the partner (BreedableComponent).
	Breed(partner Actor)
}

// Breed is BreedGoal: a mob in love walks to a partner in love and breeds.
type Breed struct {
	base
	m       Breeder
	speed   float32
	partner ID
	timer   int32
	buf     []Actor
}

// NewBreed makes minecraft:behavior.breed; speed is its speed_multiplier
// (1 by default).
func NewBreed(w World, m Breeder, speed float32) *Breed {
	return &Breed{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, speed: speed, partner: NoID}
}

// closeEnough is the breeding reach: boxes within 0.4 for mobs half a
// block or wider, otherwise a 0.6 gap between their sizes.
func closeEnough(at Vec3, self Box, p Actor) bool {
	h1 := max32(self.Max.X-self.Min.X, self.Max.Y-self.Min.Y) * 0.5
	pb := p.Box()
	h2 := max32(pb.Max.X-pb.Min.X, pb.Max.Y-pb.Min.Y) * 0.5
	pp := p.Pos()
	if h1 >= breedBigHalf || h2 >= breedBigHalf {
		hw := (self.Max.X - self.Min.X) * 0.5
		gx := gapAxis(at.X-hw, at.X+hw, pb.Min.X, pb.Max.X)
		gy := gapAxis(at.Y, at.Y+(self.Max.Y-self.Min.Y), pb.Min.Y, pb.Max.Y)
		gz := gapAxis(at.Z-hw, at.Z+hw, pb.Min.Z, pb.Max.Z)
		return gx*gx+gy*gy+gz*gz <= breedBoxGap2
	}
	d := float32(math.Sqrt(float64(dist2(at, pp))))
	return d-(h1+h2) <= breedGap
}

func gapAxis(aMin, aMax, bMin, bMax float32) float32 {
	switch {
	case aMax < bMin:
		return bMin - aMax
	case bMax < aMin:
		return aMin - bMax
	}
	return 0
}

// CanUse implements goal.Goal.
func (g *Breed) CanUse() bool {
	n := g.mob.Nav()
	if !g.mob.Flag(FlagInLove) || n == nil {
		return false
	}
	b := g.mob.Box()
	b.Min = Vec3{X: b.Min.X - breedBox, Y: b.Min.Y - breedBox, Z: b.Min.Z - breedBox}
	b.Max = Vec3{X: b.Max.X + breedBox, Y: b.Max.Y + breedBox, Z: b.Max.Z + breedBox}
	g.buf = g.world.Entities(g.buf[:0], g.mob.ID(), b)
	defer clear(g.buf)
	best := float32(temptFloatMaxValue)
	g.partner = NoID
	for _, e := range g.buf {
		if !e.IsMob() || !g.mob.CanMate(e) {
			continue
		}
		d := dist2(e.Pos(), g.mob.Pos())
		if d >= best || !n.MoveToActor(e, g.speed) {
			continue
		}
		end, ok := n.PathEnd()
		if !ok {
			continue
		}
		if closeEnough(Vec3{X: float32(end.X), Y: float32(end.Y), Z: float32(end.Z)}, g.mob.Box(), e) {
			best, g.partner = d, e.ID()
		}
	}
	p := g.actor(g.partner)
	return p != nil && !p.Removed()
}

// CanContinueToUse implements goal.Goal.
func (g *Breed) CanContinueToUse() bool {
	p := g.actor(g.partner)
	if p == nil || !p.Alive() || !p.Flag(FlagInLove) || g.timer > breedEvery-1 {
		return false
	}
	if dist2(g.mob.Pos(), p.Pos()) <= breedFar2 {
		return true
	}
	n := g.mob.Nav()
	if n == nil {
		return false
	}
	if end, ok := n.PathEnd(); ok && end.X == floorInt(p.Pos().X) && end.Z == floorInt(p.Pos().Z) {
		return true
	}
	if !n.MoveToActor(p, g.speed) {
		return false
	}
	end, ok := n.PathEnd()
	return ok && closeEnough(Vec3{X: float32(end.X), Y: float32(end.Y), Z: float32(end.Z)}, g.mob.Box(), p)
}

// Start implements goal.Goal.
func (g *Breed) Start() {}

// Stop implements goal.Goal.
func (g *Breed) Stop() {
	g.partner = NoID
	g.timer = 0
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
}

// Tick implements goal.Goal.
func (g *Breed) Tick() {
	p := g.actor(g.partner)
	if p == nil {
		return
	}
	if !g.mob.LookBusy() {
		g.mob.LookAt(lookPoint(p), breedLookYaw, g.mob.MaxHeadXRot())
	}
	if n := g.mob.Nav(); n != nil {
		n.MoveToActor(p, g.speed)
	}
	g.timer++
	if g.timer%breedEvery != 0 {
		return
	}
	if closeEnough(g.mob.Pos(), g.mob.Box(), p) && !p.Removed() {
		g.m.Breed(p)
	}
}
