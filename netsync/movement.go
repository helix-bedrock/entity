package netsync

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/helix-bedrock/protocol"
	"github.com/helix-bedrock/protocol/packet"
)

// Thresholds decide when a move is worth sending.
type Thresholds struct {
	// Distance2 is the squared distance from the last-sent position above
	// which a move is sent.
	Distance2 float32
	// Rotation is the sum of the pitch, yaw and head yaw changes, in
	// degrees, above which a turn is sent.
	Rotation float32
	// StoppedMotion2 and SettleDistance2 send the last small step of a mob
	// that stopped: when its squared motion is below StoppedMotion2 and it
	// is more than SettleDistance2 (squared) from the last-sent position.
	StoppedMotion2, SettleDistance2 float32
}

// DefaultThresholds are BDS's (shouldSendUpdate, sub_8CE9330): 0.0001, 4
// degrees, and 1e-6 for both settle values.
func DefaultThresholds() Thresholds {
	return Thresholds{Distance2: 0.0001, Rotation: 4, StoppedMotion2: 1e-6, SettleDistance2: 1e-6}
}

// Significant reports whether now differs enough from last to send, the
// way BDS checks it every tick (sub_8CE9330). Rotations are compared as
// their byte values in degrees, with no wrap-around handling, as BDS does:
// a turn from 179 to -180 degrees counts as almost 360.
func Significant(last, now Sent, motion mgl32.Vec3, t Thresholds) bool {
	d2 := now.Position.Sub(last.Position).LenSqr()
	if d2 > t.Distance2 {
		return true
	}
	turn := abs32(ByteDegrees(now.Pitch)-ByteDegrees(last.Pitch)) +
		abs32(ByteDegrees(now.Yaw)-ByteDegrees(last.Yaw)) +
		abs32(ByteDegrees(now.HeadYaw)-ByteDegrees(last.HeadYaw))
	if turn > t.Rotation {
		return true
	}
	return motion.LenSqr() < t.StoppedMotion2 && d2 > t.SettleDistance2
}

func abs32(f float32) float32 {
	return float32(math.Abs(float64(f)))
}

// MoveFlags are the forced parts of a move.
type MoveFlags struct {
	// Teleported marks the move as a teleport. BDS sends a teleport even
	// when it is not significant.
	Teleported bool
	// ForceMoveLocal makes the client move its own copy of the entity
	// (BDS sets it after setPos on a list of actors, and on dismount).
	ForceMoveLocal bool
	// ForceCompletion is the third flag BDS's sendUpdate takes; it comes
	// from a one-byte component left by a teleport (sub_B3C5260).
	ForceCompletion bool
}

// The MoveActorDelta header bits, as BDS sets them in sendUpdate
// (sub_8CE8C80).
const (
	DeltaX uint16 = 1 << iota
	DeltaY
	DeltaZ
	DeltaPitch
	DeltaYaw
	DeltaHeadYaw
	DeltaOnGround
	DeltaTeleported
	DeltaForceMoveLocal
	DeltaForceCompletion
)

// DeltaHeader is the 16-bit MoveActorDelta header for a move from last to
// now. A position bit is set when that float changed at all (exact !=), a
// rotation bit when its byte changed. Bits 10 to 15 are never set.
func DeltaHeader(last, now Sent, f MoveFlags) uint16 {
	var h uint16
	if now.Position[0] != last.Position[0] {
		h |= DeltaX
	}
	if now.Position[1] != last.Position[1] {
		h |= DeltaY
	}
	if now.Position[2] != last.Position[2] {
		h |= DeltaZ
	}
	if now.Pitch != last.Pitch {
		h |= DeltaPitch
	}
	if now.Yaw != last.Yaw {
		h |= DeltaYaw
	}
	if now.HeadYaw != last.HeadYaw {
		h |= DeltaHeadYaw
	}
	if now.OnGround {
		h |= DeltaOnGround
	}
	if f.Teleported {
		h |= DeltaTeleported
	}
	if f.ForceMoveLocal {
		h |= DeltaForceMoveLocal
	}
	if f.ForceCompletion {
		h |= DeltaForceCompletion
	}
	return h
}

// Delta builds the MoveActorDelta BDS sends for a move from last to now.
// Changed axes carry the full new float, not a difference. ticks goes in
// the packet's Ticks field; BDS's value for it was not read, so the engine
// chooses it.
func Delta(runtimeID uint64, last, now Sent, f MoveFlags, ticks uint64) packet.MoveActorDelta {
	h := DeltaHeader(last, now, f)
	pk := packet.MoveActorDelta{
		EntityRuntimeID:      runtimeID,
		OnGround:             h&DeltaOnGround != 0,
		ForceMove:            h&DeltaTeleported != 0,
		ForceMoveLocalEntity: h&DeltaForceMoveLocal != 0,
		ForceCompletion:      h&DeltaForceCompletion != 0,
		Ticks:                ticks,
	}
	if h&DeltaX != 0 {
		pk.PositionX = protocol.Option(now.Position[0])
	}
	if h&DeltaY != 0 {
		pk.PositionY = protocol.Option(now.Position[1])
	}
	if h&DeltaZ != 0 {
		pk.PositionZ = protocol.Option(now.Position[2])
	}
	if h&DeltaPitch != 0 {
		pk.RotationX = protocol.Option(WireDegrees(now.Pitch))
	}
	if h&DeltaYaw != 0 {
		pk.RotationY = protocol.Option(WireDegrees(now.Yaw))
	}
	if h&DeltaHeadYaw != 0 {
		pk.RotationYHead = protocol.Option(WireDegrees(now.HeadYaw))
	}
	return pk
}

// AbsoluteFlags is the MoveActorAbsolute flags byte. BDS starts the header
// at 0xFE and only rewrites bits 0 to 3, so bits 4 to 7 stay set (seen as
// 0xFC in memory at runtime). Whether those high bits reach the wire was
// not checked in a capture.
func AbsoluteFlags(onGround bool, f MoveFlags) byte {
	b := byte(0xF0)
	if onGround {
		b |= packet.MoveFlagOnGround
	}
	if f.Teleported {
		b |= packet.MoveFlagTeleport
	}
	if f.ForceMoveLocal {
		b |= packet.MoveFlagForceMove
	}
	if f.ForceCompletion {
		b |= packet.MoveFlagForceCompletion
	}
	return b
}

// Absolute builds the MoveActorAbsolute BDS sends when an update is forced
// absolute. BDS only does that when a player rider is dismounted
// (sub_7158500); every other move is a delta.
func Absolute(runtimeID uint64, now Sent, f MoveFlags) packet.MoveActorAbsolute {
	return packet.MoveActorAbsolute{
		EntityRuntimeID: runtimeID,
		Flags:           AbsoluteFlags(now.OnGround, f),
		Position:        now.Position,
		Rotation:        mgl32.Vec3{WireDegrees(now.Pitch), WireDegrees(now.Yaw), WireDegrees(now.HeadYaw)},
	}
}
