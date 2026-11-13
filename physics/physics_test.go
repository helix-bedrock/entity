package physics

import (
	"math"
	"testing"

	"github.com/helix-bedrock/entity/path"
)

// testWorld is a stone floor whose top is at y 64, with extra boxes.
type testWorld struct{ extra []Box }

func (testWorld) Block(p path.Pos) Block {
	if p.Y < 64 {
		return Block{Material: 3, Friction: 0.6}
	}
	return Block{Air: true}
}

func (testWorld) MaterialAt(p path.Pos) path.Material {
	if p.Y < 64 {
		return 3
	}
	return path.MaterialAir
}

func (w testWorld) CollisionBoxes(dst []Box, b Box) []Box {
	x0, x1 := floorInt(b.Min.X), floorInt(b.Max.X)
	z0, z1 := floorInt(b.Min.Z), floorInt(b.Max.Z)
	y0, y1 := floorInt(b.Min.Y), min(floorInt(b.Max.Y), 63)
	for x := x0; x <= x1; x++ {
		for z := z0; z <= z1; z++ {
			for y := y0; y <= y1; y++ {
				c := Box{Min: Vec3{X: float32(x), Y: float32(y), Z: float32(z)}, Max: Vec3{X: float32(x + 1), Y: float32(y + 1), Z: float32(z + 1)}}
				if overlapsOrTouches(c, b) {
					dst = append(dst, c)
				}
			}
		}
	}
	for _, e := range w.extra {
		if overlapsOrTouches(e, b) {
			dst = append(dst, e)
		}
	}
	return dst
}

func overlapsOrTouches(a, b Box) bool {
	return a.Min.X <= b.Max.X && b.Min.X <= a.Max.X && a.Min.Y <= b.Max.Y && b.Min.Y <= a.Max.Y && a.Min.Z <= b.Max.Z && b.Min.Z <= a.Max.Z
}

func (testWorld) ContainsMaterial(Box, path.Material) bool { return false }
func (testWorld) ContainsLiquid(Box) bool                  { return false }

func mob(x, y, z float32) *Body {
	b := &Body{
		Pos: Vec3{X: x, Y: y, Z: z}, Width: 0.6, Height: 1.8,
		MovementSpeed: 0.1, AirSpeed: 0.02, SwimSpeed: 1,
		JumpBoost: -1, Levitation: -1, MaxAutoStep: 0.5625,
	}
	b.Box = Box{Min: Vec3{X: x - 0.3, Y: y, Z: z - 0.3}, Max: Vec3{X: x + 0.3, Y: y + 1.8, Z: z + 0.3}}
	b.Flags.Set(FlagHasGravity, true)
	return b
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-5 }

func TestFallAndLand(t *testing.T) {
	w := testWorld{}
	b := mob(0.5, 66, 0.5)
	b.Tick(w)
	if b.Travel() != TravelAir || !near(b.Vel.Y, -0.08*0.98) {
		t.Fatalf("first tick: travel %d vel %v", b.Travel(), b.Vel)
	}
	for i := 0; i < 40 && !b.OnGround; i++ {
		b.Tick(w)
	}
	if !b.OnGround || !near(b.Pos.Y, 64) {
		t.Fatalf("did not land: %v ground %v", b.Pos, b.OnGround)
	}
	// The landing tick's own move is not counted.
	if !b.Landed || b.LandedFall < 1.5 || b.LandedFall >= 2 {
		t.Fatalf("landed %v fall %v", b.Landed, b.LandedFall)
	}
}

func TestWalkSpeed(t *testing.T) {
	w := testWorld{}
	b := mob(0.5, 64, 0.5)
	b.OnGround = true
	b.Input = Vec3{Z: 1}
	b.Tick(w)
	// 0.546/(0.6*0.91) is 1, so the travel speed is the movement speed.
	want := float32(0.1*0.98) * 0.546
	if b.Travel() != TravelGround || !near(b.Vel.Z, want) || !near(b.Pos.Z, 0.5+0.1*0.98) {
		t.Fatalf("travel %d vel %v pos %v", b.Travel(), b.Vel, b.Pos)
	}
}

func TestWallStops(t *testing.T) {
	w := testWorld{extra: []Box{{Min: Vec3{X: 1, Y: 64, Z: 0}, Max: Vec3{X: 2, Y: 66, Z: 1}}}}
	b := mob(0.6, 64, 0.5)
	b.OnGround = true
	b.Vel.X = 0.5
	b.Tick(w)
	if !b.HorizontalCollision[0] || b.Vel.X != 0 || !near(b.Box.Max.X, 1) {
		t.Fatalf("wall: hc %v vel %v box %v", b.HorizontalCollision, b.Vel, b.Box)
	}
}

func TestAutoStepSlab(t *testing.T) {
	w := testWorld{extra: []Box{{Min: Vec3{X: 1, Y: 64, Z: 0}, Max: Vec3{X: 2, Y: 64.5, Z: 1}}}}
	b := mob(0.6, 64, 0.5)
	b.OnGround = true
	b.Vel.X = 0.3
	b.Tick(w)
	if !near(b.Box.Min.Y, 64.5) || b.HorizontalCollision[0] {
		t.Fatalf("no step: box %v hc %v", b.Box, b.HorizontalCollision)
	}
}

func TestClipStopsAtContact(t *testing.T) {
	s := Box{Min: Vec3{Y: 0}, Max: Vec3{X: 1, Y: 1, Z: 1}}
	m := Box{Min: Vec3{X: 0.2, Y: 1.5, Z: 0.2}, Max: Vec3{X: 0.8, Y: 2, Z: 0.8}}
	r := clip(s, m, Vec3{Y: -1})
	if !near(r.clipped.Y, -0.5) || r.depth != 0 {
		t.Fatalf("clip %+v", r)
	}
	if r := clip(s, m, Vec3{X: 2, Y: -1}); r.clipped.X != 2 {
		t.Fatalf("diagonal clipped x: %+v", r)
	}
}

func TestDepenetrate(t *testing.T) {
	s := Box{Max: Vec3{X: 1, Y: 1, Z: 1}}
	m := Box{Min: Vec3{X: 0.2, Y: 0.9, Z: 0.2}, Max: Vec3{X: 0.8, Y: 2.9, Z: 0.8}}
	r := clip(s, m, Vec3{})
	if r.axis != 1 || !near(r.depth, 0.1) || !near(r.clipped.Y, 0.1) || r.free.Y != 0 {
		t.Fatalf("depenetrate %+v", r)
	}
}

func BenchmarkTick(b *testing.B) {
	w := testWorld{}
	m := mob(0.5, 64, 0.5)
	m.OnGround = true
	b.ReportAllocs()
	for b.Loop() {
		m.Input = Vec3{Z: 1}
		m.Tick(w)
	}
}

func TestJumpPower(t *testing.T) {
	b := Body{}
	if b.jumpPower() != groundJump {
		t.Fatal("zero jump power is not the default")
	}
	b.JumpPower = 0.6
	if b.jumpPower() != 0.6 {
		t.Error("jump.static power ignored")
	}
}
