package physics

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// afterCase is one record of the oracle's "after" job: a ground mob's inputs
// to the after-move systems, and BDS's outputs of each system run alone from
// those inputs. Record layout (96 bytes):
//
//	in:  flags 3 x u64; slowFalling u8 (0 short effect list, 1 empty slot,
//	     2 slow falling), verticalFriction u8, frictionModHas u8,
//	     airDragModHas u8, applyReset u8, applied u8, pad 2;
//	     vel xyz, frictionMod, airDragMod, travelFriction, fall, applyGravity (f32)
//	out: the gravity component (f32 gravity, u8 slow fall; sentinel 12345 / 2
//	     when not written), vel.y and fall after apply gravity, vel.y after
//	     vertical drag, vel xyz after friction
type afterCase struct {
	flags                  [3]uint64
	slowFalling            uint8
	vertFriction, frHas    bool
	adHas, applyReset, ran bool
	vel                    Vec3
	frMod, adMod, travelFr float32
	fall, applyG           float32
	gravOut                float32
	gravReset              uint8
	applyVelY, applyFall   float32
	dragVelY               float32
	frictionVel            Vec3
}

const afterSentinel = 12345

func parseAfter(r []byte) afterCase {
	var c afterCase
	for i := range c.flags {
		c.flags[i] = binary.LittleEndian.Uint64(r[8*i:])
	}
	c.slowFalling = r[24]
	c.vertFriction, c.frHas, c.adHas, c.applyReset, c.ran = r[25] != 0, r[26] != 0, r[27] != 0, r[28] != 0, r[29] != 0
	f := r[32:]
	c.vel = vecAt(f, 0)
	c.frMod, c.adMod, c.travelFr, c.fall, c.applyG = f32At(f, 3), f32At(f, 4), f32At(f, 5), f32At(f, 6), f32At(f, 7)
	o := r[64:]
	c.gravOut, c.gravReset = f32At(o, 0), o[4]
	c.applyVelY, c.applyFall = f32At(o, 2), f32At(o, 3)
	c.dragVelY = f32At(o, 4)
	c.frictionVel = vecAt(o, 5)
	return c
}

// body builds a mob's Body from the case's inputs.
func (c *afterCase) body(travel Travel) *Body {
	b := &Body{Vel: c.vel, FallDistance: c.fall, SlowFalling: c.slowFalling == 2, VerticalFriction: c.vertFriction}
	b.Flags = c.flags
	if c.frHas {
		v := c.frMod
		b.Attrs.Friction = &v
	}
	if c.adHas {
		v := c.adMod
		b.Attrs.AirDrag = &v
	}
	b.travel = travel
	b.friction = c.travelFr
	return b
}

// TestOracleAfter compares the ground after-move systems (gravity choice,
// apply gravity, vertical drag, friction) with BDS 1.26.60's, each from the
// same inputs, bit for bit.
func TestOracleAfter(t *testing.T) { checkAfter(t, "ORACLE_AFTER", "after", TravelGround) }

// TestOracleAir is the same for the air versions of those systems.
func TestOracleAir(t *testing.T) { checkAfter(t, "ORACLE_AIR", "air", TravelAir) }

func checkAfter(t *testing.T, env, job string, travel Travel) {
	recs := oracleRecords(t, env, job, 96)
	bad := map[string]int{}
	report := func(step string, i int, c *afterCase, format string, args ...any) {
		if bad[step] < 5 {
			t.Errorf("case %d %s: %s\n  in %+v", i, step, fmt.Sprintf(format, args...), *c)
		}
		bad[step]++
	}
	for i, r := range recs {
		c := parseAfter(r)

		b := c.body(travel)
		var wantG float32 = afterSentinel
		var wantReset uint8 = 2
		if b.airGravity() {
			wantG, wantReset = b.gravity, 0
			if b.slowFall {
				wantReset = 1
			}
		}
		if !sameBits(wantG, c.gravOut) || wantReset != c.gravReset {
			report("gravity", i, &c, "bds %v/%d, ours %v/%d", c.gravOut, c.gravReset, wantG, wantReset)
		}

		if c.ran {
			b = c.body(travel)
			b.gravity, b.slowFall = c.applyG, c.applyReset
			b.applyGravity()
			if !sameBits(b.Vel.Y, c.applyVelY) || !sameBits(b.FallDistance, c.applyFall) {
				report("apply", i, &c, "bds %v %v, ours %v %v", c.applyVelY, c.applyFall, b.Vel.Y, b.FallDistance)
			}
		}

		b = c.body(travel)
		b.verticalDrag()
		if !sameBits(b.Vel.Y, c.dragVelY) {
			report("drag", i, &c, "bds %v, ours %v", c.dragVelY, b.Vel.Y)
		}

		b = c.body(travel)
		b.applyFriction()
		if !sameVec(b.Vel, c.frictionVel) {
			report("friction", i, &c, "bds %v, ours %v", c.frictionVel, b.Vel)
		}
	}
	t.Logf("%d cases, differences per step: %v", len(recs), bad)
}
