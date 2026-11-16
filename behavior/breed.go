package behavior

// The breed goal's fixed numbers.
const (
	breedBox     = 8
	breedEvery   = 60 // ticks between breeding tries; the goal ends after one
	breedLookYaw = 10
	breedBigHalf = 0.5
	breedBoxGap2 = 0.16000001
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

// closeEnough is BDS 1.26.60's breeding reach (inlined in BreedGoal::canUse
// 0x5518130 and tick 0x5518E70). Each mob is a box of its width and height
// centred on its position, feet included, with the mob at at. For mobs half
// a block or bigger (the larger of width and height), each axis takes the
// point halfway between the two centres, clamps it into each box, and the
// two clamped points must be within 0.4 blocks. Otherwise the centres must be
// within 0.6 of the two half sizes.
func closeEnough(at Vec3, self Box, p Actor) bool {
	pb := p.Box()
	return breedReach(at, self.Max.X-self.Min.X, self.Max.Y-self.Min.Y, p.Pos(), pb.Max.X-pb.Min.X, pb.Max.Y-pb.Min.Y)
}

// breedReach is closeEnough on plain numbers: the mob at at with width sw and
// height sh, the partner at pp with width pw and height ph.
func breedReach(at Vec3, sw, sh float32, pp Vec3, pw, ph float32) bool {
	h1 := max32(sw, sh) * 0.5
	h2 := max32(pw, ph) * 0.5
	if h1 >= breedBigHalf || h2 >= breedBigHalf {
		gx := clampGap(pp.X, pw*0.5, at.X, sw*0.5)
		gy := clampGap(pp.Y, ph*0.5, at.Y, sh*0.5)
		gz := clampGap(pp.Z, pw*0.5, at.Z, sw*0.5)
		return gz*gz+(gx*gx+gy*gy) <= breedBoxGap2
	}
	d := sqrt32(dist2(at, pp))
	return d-(h1+h2) <= breedGap
}

// clampGap is one axis of the big-mob reach: the midpoint of the two centres
// clamped into the partner's span minus it clamped into the mob's span. It
// is not the gap between the spans: a short span inside a long one, off its
// middle, still gives a gap.
func clampGap(pc, ph, sc, sh float32) float32 {
	mid := (pc + sc) * 0.5
	return clampSpan(mid, pc, ph) - clampSpan(mid, sc, sh)
}

// clampSpan clamps v into [c-h, c+h] the way BDS's vector code does: the top
// first, then the bottom.
func clampSpan(v, c, h float32) float32 {
	if hi := c + h; hi < v {
		return hi
	}
	return max32(v, c-h)
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
	// BDS keeps breeding without a navigation component.
	n := g.mob.Nav()
	if n == nil {
		return true
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
