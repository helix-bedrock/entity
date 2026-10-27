package behavior

// Sleeper is the part of the mob sleep needs on top of Dweller.
type Sleeper interface {
	Dweller
	// BedDirection is the bed block's direction state at p; ok is false
	// when the block has no direction.
	BedDirection(p Pos) (dir int32, ok bool)
	// Sleeping is Actor vt+336.
	Sleeping() bool
	// SetSleeping is Actor vt+344. Waking also clears the bounding box
	// update request.
	SetSleeping(on bool)
	// SetSleepCollider clears the movement interpolation and sets the
	// collision box (0xB2D5FC0).
	SetSleepCollider(width, height float32)
	// SetSleepYaw sets the body, head and look yaw while lying down.
	SetSleepYaw(yaw float32)
	// HealOnWake heals the missing health (0xB303CA0) unless the mob may
	// not (Actor +761).
	HealOnWake()
	// StandUpSpot is where the mob gets out of the bed at p, from where it
	// lay down (0xC736290); ok is false when p is not a bed or no spot is
	// free.
	StandUpSpot(bed Pos, from Vec3) (Pos, bool)
}

// SleepConfig is the JSON of minecraft:behavior.sleep and the bed
// geometry BDS writes into the goal.
type SleepConfig struct {
	MoveToPOIConfig
	SleepColliderHeight float32 `json:"sleep_collider_height"`
	SleepColliderWidth  float32 `json:"sleep_collider_width"`
	SleepYOffset        float32 `json:"sleep_y_offset"`
	// CooldownTime is counted in ticks, the way BDS copies it.
	CooldownTime        float32 `json:"cooldown_time"`
	CanSleepWhileRiding bool    `json:"can_sleep_while_riding"`
	// DirYaw and DirOffset are the yaw and the x, z spot in the bed block
	// for each bed direction.
	DirYaw    [4]float32    `json:"-"`
	DirOffset [4][2]float32 `json:"-"`
}

// DefaultSleep returns the goal's BDS defaults (TypeSchema 0xDF84F60; the
// bed table from the goal 0x592D090).
func DefaultSleep() SleepConfig {
	return SleepConfig{MoveToPOIConfig: DefaultMoveToPOI(), SleepColliderHeight: 1, SleepColliderWidth: 1,
		DirYaw:    [4]float32{180, 270, 0, 90},
		DirOffset: [4][2]float32{{0.5, -0.125}, {1.125, 0.5}, {0.5, 1.125}, {-0.125, 0.5}}}
}

// Sleep is SleepGoal: a villager walks to its bed at night and lies in it.
type Sleep struct {
	moveToPOI
	m   Sleeper
	cfg SleepConfig
	bed Pos
	// spot and yaw are where and how the mob lies; laidFrom is where it
	// stood when it lay down.
	spot, laidFrom Vec3
	yaw            float32
	// resting and rest are +336 and +328: the cooldown after waking.
	resting bool
	rest    int64
	woke    bool
}

// NewSleep makes the goal.
func NewSleep(w World, m Sleeper, c SleepConfig) *Sleep {
	g := &Sleep{m: m, cfg: c}
	c.POIType = POIBed
	g.initPOI(w, m, c.MoveToPOIConfig, g, g)
	g.flags = flagsMoveLook | flagsJump
	return g
}

// find is unused by the POI goals.
func (g *Sleep) find() bool { return false }

// CanUse implements goal.Goal (0x592D200).
func (g *Sleep) CanUse() bool {
	if !g.canUse() || !g.cfg.CanSleepWhileRiding && g.mob.Vehicle() != NoID {
		return false
	}
	poi, ok := g.m.POI(POIBed)
	if !ok {
		return false
	}
	if g.resting {
		g.rest++
		if float32(g.rest) <= g.cfg.CooldownTime {
			return false
		}
		g.resting = false
	}
	g.bed = poi.Pos
	dir, ok := g.m.BedDirection(g.bed)
	if !ok {
		return false
	}
	off, yaw := [2]float32{0.5, 0.5}, float32(0)
	if dir >= 0 && int(dir) < len(g.cfg.DirOffset) {
		off, yaw = g.cfg.DirOffset[dir], g.cfg.DirYaw[dir]
	}
	g.spot = Vec3{X: float32(g.bed.X) + off[0], Y: float32(g.bed.Y) + g.cfg.SleepYOffset, Z: float32(g.bed.Z) + off[1]}
	g.yaw = yaw
	return true
}

// CanContinueToUse implements goal.Goal (0x592DB20): until something
// wakes the mob (flag 77).
func (g *Sleep) CanContinueToUse() bool {
	ok := g.keepLite()
	if g.reached {
		ok = g.valid(g.targetBlock())
	}
	g.woke = !ok || g.mob.Flag(FlagWantsToWake)
	return !g.woke
}

// Start implements goal.Goal (0x592D6B0).
func (g *Sleep) Start() { g.startPOI() }

// Stop implements goal.Goal (0x592D740).
func (g *Sleep) Stop() {
	m := g.m
	if m.Sleeping() {
		if !g.woke {
			m.HealOnWake()
		}
		at := blockOf(g.spot)
		if p, ok := m.StandUpSpot(g.bed, g.laidFrom); ok {
			at = p
		}
		m.Teleport(Vec3{X: float32(at.X) + 0.5, Y: float32(at.Y), Z: float32(at.Z) + 0.5})
		m.SetVelocity(Vec3{})
		g.resting = true
		m.SetSleeping(false)
	}
	g.rest, g.woke = 0, false
	m.SetFlag(FlagWantsToWake, false)
	g.stopPOI()
}

// Tick implements goal.Goal (0x592DC30).
func (g *Sleep) Tick() {
	g.tickPOI()
	m := g.m
	if !m.Sleeping() {
		if !g.reached {
			return
		}
		m.SetSleeping(true)
		m.SetSleepCollider(g.cfg.SleepColliderWidth, g.cfg.SleepColliderHeight)
		g.laidFrom = m.Pos()
		m.SetVelocity(Vec3{})
		if n := m.Nav(); n != nil {
			n.Stop()
		}
	}
	m.Teleport(g.spot)
	m.SetSleepYaw(g.yaw)
}
