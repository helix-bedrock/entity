package behavior

// knockbackExtra is the fourth knockback argument every batch 6 caller
// passes; what the host does with it is not mapped yet.
const knockbackExtra = 0.5

// Knockback is the argument block of Actor::knockback (Actor vtable +1096)
// as aquatic_charge_attack, ram_attack and knockback_roar fill it.
type Knockback struct {
	Power     float32
	Height    float32
	HeightCap float32
	// Extra is the unnamed fourth float; BDS passes 0.5.
	Extra float32
	// DirX and DirZ are the direction arguments, as BDS passes them.
	DirX, DirZ float32
}

// Knocker is a mob that can knock another entity back.
type Knocker interface {
	// Knockback knocks t back with the mob as the source.
	Knockback(t Actor, k Knockback)
}

// MoveController is a mob that may have a MoveControlComponent.
type MoveController interface {
	// MoveControl reports the move control's maximum turn per tick (its
	// +20); ok is false when the mob has none.
	MoveControl() (maxTurn float32, ok bool)
	// SetBodyYaw sets the body rotation (MobBodyRotationComponent).
	SetBodyYaw(yaw float32)
}

// faceYaw turns the mob's rotation and body towards yaw by at most limit
// degrees, the turn step fire_at_target and ram_attack share.
func faceYaw(m Mob, c MoveController, yaw, limit float32) float32 {
	_, cur := m.HeadRotation()
	v := approachDegrees(cur, yaw, limit)
	c.SetBodyYaw(v)
	m.SetYaw(v)
	return v
}
