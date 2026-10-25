package move

// Glider is minecraft:movement.glide's settings and state.
type Glider struct {
	// StartSpeed is start_speed: the speed after bumping into a wall.
	StartSpeed float32
	// TurnSpeed is speed_when_turning.
	TurnSpeed float32

	speed  float32
	saved  Vec3
	bumped int32
}

// NewGlider returns a glider at its start speed.
func NewGlider(start, turn float32) *Glider {
	return &Glider{StartSpeed: start, TurnSpeed: turn, speed: start}
}

// wrapDegrees maps d into [-180, 180).
func wrapDegrees(d float32) float32 {
	d = fmod(d+180, 360)
	if d < 0 {
		d += 360
	}
	return d - 180
}

// toward moves cur towards target by at most step.
func toward(cur, target, step float32) float32 {
	if target > cur {
		return min(target, cur+step)
	}
	return max32(cur-step, target)
}

// Glide is minecraft:movement.glide (the phantom): the mob swoops towards
// the wanted position, turning at most 4 degrees a tick, slowing in turns
// and turning back from walls. bumped reports a horizontal collision.
func Glide(b *Body, c *Control, g *Glider, bumped bool) {
	if !c.HasWanted {
		return
	}
	check := g.bumped > 3
	if bumped {
		b.Yaw += 180
		g.speed = g.StartSpeed
		if g.bumped == 0 {
			g.saved = c.Wanted
		}
		g.bumped++
		check = g.bumped > 3
	}
	if check {
		if g.saved == c.Wanted {
			c.HasWanted = false
		}
		g.bumped = 0
	}

	dx, dy, dz := c.Wanted.X-b.Pos.X, c.Wanted.Y-b.Pos.Y, c.Wanted.Z-b.Pos.Z
	f := float32(1)
	if h2 := dx*dx + dz*dz; h2 != 0 {
		f = 1 - abs32(0.7*dy)/sqrt(h2)
	}
	dx *= f
	dz *= f
	h2 := dx*dx + dz*dz
	hx := sqrt(h2)
	dist := sqrt(dy*dy + h2)

	yaw0 := b.Yaw
	cur := wrapDegrees(yaw0 + 90)
	target := wrapDegrees(atan2(dz, dx) * degrees)
	diff := wrapDegrees(target - cur)
	if diff >= 180 {
		diff -= 360
	}
	yaw := toward(cur, diff+cur, 4) - 90
	b.faceYaw(yaw)

	turn := wrapDegrees(yaw0 - yaw)
	if turn >= 180 {
		turn -= 360
	}
	if abs32(turn) >= 3 {
		g.speed = toward(g.speed, g.TurnSpeed*c.Speed, 0.025)
	} else {
		g.speed = toward(g.speed, b.MovementSpeed*c.Speed, abs32(1.8/g.speed*0.005))
	}

	pitch := atan2(-dy, hx) * degrees
	if l := b.Look; l != nil && !l.Has {
		l.HasYaw = true
		l.Pitch = -pitch
		l.Yaw = yaw
		l.YawSpeed, l.PitchSpeed = 60, 20
	}

	a := (yaw + 90) * 0.017453292 * sinScale
	var tx, ty, tz float32
	if abs32(dist) >= 0.00000011920929 {
		tx = abs32(dx/dist) * (sinTable[uint16(int32(a+16384))] * g.speed)
		ty = abs32(dy/dist) * (sinTable[uint16(int32(pitch*-0.017453292*sinScale))] * g.speed)
		tz = abs32(dz/dist) * (sinTable[uint16(int32(a))] * g.speed)
	}
	b.Vel.X += (tx - b.Vel.X) * 0.2
	b.Vel.Y += (ty - b.Vel.Y) * 0.2
	b.Vel.Z += (tz - b.Vel.Z) * 0.2
	b.FallDistance = 0
}
