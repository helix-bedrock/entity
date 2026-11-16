package physics

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/helix-bedrock/entity/path"
)

// liquidCase is one record of the oracle's "water" and "lava" jobs: a mob's
// inputs to the liquid after-move systems, and BDS's outputs of each system
// run alone from those inputs. Record layout (136 bytes):
//
//	in:  flags 3 x u64; pos xyz, vel xyz, height, swimSpeed, waterDrag (f32),
//	     depthStrider i32, frictionMod, airDragMod, travelFriction, pad (f32);
//	     u8 onGround, waterMovement, nav, navBits (1 walks in lava, 2 sinks),
//	     physics, frictionModHas, airDragModHas, slowFalling
//	out: vel xyz after the drag; the liquid gravity component (f32 gravity,
//	     u8 slow fall, pad 3; sentinel 12345 / 2 when not written); lava only:
//	     the lava walk gravity component, vel.y after the lava walk vertical
//	     drag, vel xyz after the lava walk friction
type liquidCase struct {
	flags                    [3]uint64
	pos, vel                 Vec3
	height, swim, waterDrag  float32
	depth                    int32
	frMod, adMod, travelFr   float32
	onGround, waterMove, nav bool
	navBits                  uint8
	physics, frHas, adHas    bool
	slowFalling              uint8
	dragVel                  Vec3
	grav, walkGrav           float32
	walkReset                uint8
	walkDragVelY             float32
	walkFrictionVel          Vec3
}

func parseLiquid(r []byte) liquidCase {
	var c liquidCase
	for i := range c.flags {
		c.flags[i] = binary.LittleEndian.Uint64(r[8*i:])
	}
	f := r[24:]
	c.pos, c.vel = vecAt(f, 0), vecAt(f, 3)
	c.height, c.swim, c.waterDrag = f32At(f, 6), f32At(f, 7), f32At(f, 8)
	c.depth = int32(binary.LittleEndian.Uint32(f[36:]))
	c.frMod, c.adMod, c.travelFr = f32At(f, 10), f32At(f, 11), f32At(f, 12)
	u := r[80:]
	c.onGround, c.waterMove, c.nav, c.navBits = u[0] != 0, u[1] != 0, u[2] != 0, u[3]
	c.physics, c.frHas, c.adHas, c.slowFalling = u[4] != 0, u[5] != 0, u[6] != 0, u[7]
	o := r[88:]
	c.dragVel = vecAt(o, 0)
	c.grav = f32At(o, 3)
	c.walkGrav, c.walkReset = f32At(o, 5), o[24]
	c.walkDragVelY = f32At(o, 7)
	c.walkFrictionVel = vecAt(o, 8)
	return c
}

func (c *liquidCase) body(travel Travel) *Body {
	b := &Body{
		Pos: c.pos, Vel: c.vel, Height: c.height, SwimSpeed: c.swim, DepthStrider: c.depth,
		OnGround: c.onGround, HasPhysics: c.physics, SlowFalling: c.slowFalling == 2,
	}
	b.Flags = c.flags
	if c.waterMove {
		v := c.waterDrag
		b.WaterDrag = &v
	}
	if c.nav {
		b.Nav = Nav{Present: true, CanWalkInLava: c.navBits&1 != 0, CanSink: c.navBits&2 != 0}
	}
	if c.frHas {
		v := c.frMod
		b.Attrs.Friction = &v
	}
	if c.adHas {
		v := c.adMod
		b.Attrs.AirDrag = &v
	}
	b.travel, b.friction = travel, c.travelFr
	return b
}

// liquidBox is the oracle scenes' glass box, x and z -3..3 and y 200..207,
// with the liquid inside from y 201 up to top (206 full, 201 shallow).
type liquidBox struct {
	testWorld
	liquid path.Material
	top    int32
}

// Liquid tops of the oracle scenes.
const (
	fullTop    = 206
	shallowTop = 201
)

// glassMaterial stands for any solid block's material.
const glassMaterial path.Material = 1

func (w liquidBox) MaterialAt(p path.Pos) path.Material {
	in := func(v, lo, hi int32) bool { return v >= lo && v <= hi }
	switch {
	case in(p.X, -2, 2) && in(p.Z, -2, 2) && in(p.Y, 201, 206):
		if p.Y <= w.top {
			return w.liquid
		}
		return path.MaterialAir
	case in(p.X, -3, 3) && in(p.Z, -3, 3) && in(p.Y, 200, 207):
		return glassMaterial
	}
	return path.MaterialAir
}

// TestOracleWater compares the water drag and water gravity with BDS
// 1.26.60's, each from the same inputs, bit for bit.
func TestOracleWater(t *testing.T) {
	checkWater(t, "ORACLE_WATER", "water", fullTop)
}

// TestOracleWaterShallow is TestOracleWater in the one-layer scene, where
// the mob stands on the floor.
func TestOracleWaterShallow(t *testing.T) {
	checkWater(t, "ORACLE_WATER_SHALLOW", "water_shallow", shallowTop)
}

func checkWater(t *testing.T, env, name string, top int32) {
	recs := oracleRecords(t, env, name, 136)
	w := liquidBox{liquid: path.MaterialWater, top: top}
	d := diffs{t: t}
	for i, r := range recs {
		c := parseLiquid(r)
		b := c.body(TravelWater)
		b.waterDrag()
		if !sameVec(b.Vel, c.dragVel) {
			d.report("drag", i, &c, "bds %v, ours %v", c.dragVel, b.Vel)
		}
		b = c.body(TravelWater)
		if g := liquidGravityOut(b, w); !sameBits(g, c.grav) {
			d.report("gravity", i, &c, "bds %v, ours %v", c.grav, g)
		}
	}
	t.Logf("%d cases, differences per step: %v", len(recs), d.bad)
}

// TestOracleLava compares the lava drag, lava gravity and the lava walking
// systems with BDS 1.26.60's.
func TestOracleLava(t *testing.T) {
	checkLava(t, "ORACLE_LAVA", "lava", fullTop)
}

// TestOracleLavaShallow is TestOracleLava in the one-layer scene.
func TestOracleLavaShallow(t *testing.T) {
	checkLava(t, "ORACLE_LAVA_SHALLOW", "lava_shallow", shallowTop)
}

func checkLava(t *testing.T, env, name string, top int32) {
	recs := oracleRecords(t, env, name, 136)
	w := liquidBox{liquid: path.MaterialLava, top: top}
	d := diffs{t: t}
	for i, r := range recs {
		c := parseLiquid(r)
		b := c.body(TravelLava)
		b.lavaDrag()
		if !sameVec(b.Vel, c.dragVel) {
			d.report("drag", i, &c, "bds %v, ours %v", c.dragVel, b.Vel)
		}

		b = c.body(TravelLava)
		var g float32 = afterSentinel
		if !b.lavaWalking() {
			g = liquidGravityOut(b, w)
		}
		if !sameBits(g, c.grav) {
			d.report("gravity", i, &c, "bds %v, ours %v", c.grav, g)
		}

		b = c.body(TravelLava)
		var wantG float32 = afterSentinel
		var wantReset uint8 = 2
		if b.lavaWalking() && b.Nav.CanSink && b.airGravity() {
			wantG, wantReset = b.gravity, 0
			if b.slowFall {
				wantReset = 1
			}
		}
		if !sameBits(wantG, c.walkGrav) || wantReset != c.walkReset {
			d.report("walkGravity", i, &c, "bds %v/%d, ours %v/%d", c.walkGrav, c.walkReset, wantG, wantReset)
		}

		b = c.body(TravelLava)
		if b.lavaWalking() {
			b.verticalDrag()
		}
		if !sameBits(b.Vel.Y, c.walkDragVelY) {
			d.report("walkDrag", i, &c, "bds %v, ours %v", c.walkDragVelY, b.Vel.Y)
		}

		b = c.body(TravelLava)
		b.applyFriction()
		if !sameVec(b.Vel, c.walkFrictionVel) {
			d.report("walkFriction", i, &c, "bds %v, ours %v", c.walkFrictionVel, b.Vel)
		}
	}
	t.Logf("%d cases, differences per step: %v", len(recs), d.bad)
}

// liquidGravityOut is the gravity value the liquid gravity system leaves in
// the component: liquidGravity's, or the sentinel when it writes none.
func liquidGravityOut(b *Body, w World) float32 {
	if b.liquidGravity(w) {
		return b.gravity
	}
	return afterSentinel
}

// diffs counts the differences per step and shows the first few.
type diffs struct {
	t   *testing.T
	bad map[string]int
}

func (d *diffs) report(step string, i int, c any, format string, args ...any) {
	if d.bad == nil {
		d.bad = map[string]int{}
	}
	if d.bad[step] < 5 {
		d.t.Errorf("case %d %s: %s\n  in %+v", i, step, fmt.Sprintf(format, args...), c)
	}
	d.bad[step]++
}
