package netsync

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// Pose is where an entity is and where it looks, as the engine knows it.
type Pose struct {
	Position mgl32.Vec3
	// Pitch, Yaw, HeadYaw and BodyYaw are in degrees. HeadYaw and BodyYaw
	// are only sent for mobs.
	Pitch, Yaw, HeadYaw, BodyYaw float32
	OnGround                     bool
}

// Sent is a Pose the way BDS keeps it for the network
// (MoveActorAbsoluteData, built by sub_85A44D0): the position as float32,
// the rotations as bytes. It is what a viewer last saw.
type Sent struct {
	Position                     mgl32.Vec3
	Pitch, Yaw, HeadYaw, BodyYaw byte
	OnGround                     bool
}

// Quantize turns a Pose into what BDS sends. For a non-mob, head yaw and
// body yaw are 0, as BDS only reads them from a Mob's components.
func Quantize(p Pose, mob bool) Sent {
	s := Sent{
		Position: p.Position,
		Pitch:    RotationByte(p.Pitch),
		Yaw:      RotationByte(p.Yaw),
		OnGround: p.OnGround,
	}
	if mob {
		s.HeadYaw = RotationByte(p.HeadYaw)
		s.BodyYaw = RotationByte(p.BodyYaw)
	}
	return s
}

// WrapDegrees wraps d into [-180, 180), as BDS does before rounding:
// fmod(d+180, 360), plus 360 if negative, minus 180, in float32.
func WrapDegrees(d float32) float32 {
	// fmod is exact, so doing it in float64 gives the float32 result.
	r := float32(math.Mod(float64(d+180), 360))
	if r < 0 {
		r += 360
	}
	return r - 180
}

// RotationByte is BDS's network rotation: int32((wrap(d) / 360) * 256),
// truncated, in float32 (sub_85A44D0). The divide comes before the
// multiply; the other order rounds differently for some angles.
func RotationByte(d float32) byte {
	w := WrapDegrees(d)
	if w != w {
		return 0
	}
	return byte(int32((w / 360) * 256))
}

// ByteDegrees turns a rotation byte back into degrees the way BDS does in
// its significance check and in AddActor: int8(b) * (1/256) * 360. The
// result is in [-180, 180) and always a multiple of 1.40625.
func ByteDegrees(b byte) float32 {
	return float32(int8(b)) * (1.0 / 256) * 360
}

// WireDegrees is the angle to put in a protocol packet's byte-rotation
// field so that the protocol writer emits exactly b. The protocol encodes
// such fields as byte(deg / 1.40625), which is exact for this value.
func WireDegrees(b byte) float32 {
	return float32(b) * (360.0 / 256.0)
}
