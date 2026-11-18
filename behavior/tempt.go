package behavior

import "github.com/helix-bedrock/entity/nav"

// The tempt goal's fixed numbers.
const (
	temptCooldown      = 100 // ticks before a mob can be tempted again
	temptRadius        = 16  // within_radius when it is 0 or less
	temptStop          = 1.5 // stop_distance when it is negative
	temptScareDist2    = 36  // the player is close enough to scare the mob
	temptScareMove2    = 0.01
	temptScareTurn     = 5
	temptLookYaw       = 30
	temptRepathMin     = 8
	temptRepathRandom  = 4
	temptFloatMaxValue = 3.4028235e38
	floatTemptMinLen   = 0.1 // float_tempt's shortest direction length
	temptSoundMin      = 3
	temptSoundMax      = 8
)

// TemptConfig is the JSON of minecraft:behavior.tempt.
type TemptConfig struct {
	SpeedMultiplier     float32 `json:"speed_multiplier"`
	WithinRadius        float32 `json:"within_radius"`
	StopDistance        float32 `json:"stop_distance"`
	CanGetScared        bool    `json:"can_get_scared"`
	CanTemptVertically  bool    `json:"can_tempt_vertically"`
	CanTemptWhileRidden bool    `json:"can_tempt_while_ridden"`
	// Tempts reports that the player holds one of the goal's items.
	Tempts        func(player Actor) bool
	Sound         int
	SoundInterval Range[float32]
	OnTemptStart  string
	OnTemptEnd    string
}

// DefaultTempt returns BDS 1.26.60's tempt defaults (TemptBaseGoal
// and its setup): speed 1, stop_distance 1.5, no sound
// and a 3 to 8 second sound interval. The interval matters without a sound
// too: the goal still draws a random number each time it runs out.
func DefaultTempt() TemptConfig {
	return TemptConfig{SpeedMultiplier: 1, StopDistance: temptStop, Sound: NoSound,
		SoundInterval: Range[float32]{Min: temptSoundMin, Max: temptSoundMax}}
}

// Tempt is TemptGoal: the mob follows the nearest player holding one of its
// items.
type Tempt struct {
	base
	cfg       TemptConfig
	player    ID
	cooldown  int32
	repath    int32
	lastPos   Vec3
	lastPitch float32
	lastYaw   float32
	nextSound uint64
	avoided   bool
	// float is float_tempt: the mob swims or flies straight at the player
	// by adding to its motion, not by pathing.
	float bool
	buf   []Actor
}

// NewTempt makes the goal.
func NewTempt(w World, m Mob, c TemptConfig) *Tempt {
	if c.WithinRadius <= 0 {
		c.WithinRadius = temptRadius
	}
	if c.StopDistance < 0 {
		c.StopDistance = temptStop
	}
	return &Tempt{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c, player: NoID}
}

// NewFloatTempt makes minecraft:behavior.float_tempt (FloatTemptGoal,):
// tempt with the same JSON, but while the player is out of reach
// the mob's motion grows toward the player by movement speed times
// speed_multiplier each tick, and in reach it does nothing.
func NewFloatTempt(w World, m Mob, c TemptConfig) *Tempt {
	g := NewTempt(w, m, c)
	g.float = true
	return g
}

// CanUse implements goal.Goal.
//
// float_tempt runs BDS's code, which
// needs no navigation; only BDS's code checks for it.
func (g *Tempt) CanUse() bool {
	if !g.float && g.mob.Nav() == nil {
		return false
	}
	if g.cooldown > 0 {
		g.cooldown--
		return false
	}
	if len(g.mob.Riders()) != 0 && !g.cfg.CanTemptWhileRidden {
		return false
	}
	r2 := g.cfg.WithinRadius * g.cfg.WithinRadius
	best := float32(temptFloatMaxValue)
	var found Actor
	g.buf = g.world.Players(g.buf[:0])
	for _, p := range g.buf {
		if !p.Pickable() || g.cfg.Tempts == nil || !g.cfg.Tempts(p) {
			continue
		}
		if d := dist2(p.Pos(), g.mob.Pos()); r2 > d && best > d {
			best, found = d, p
		}
	}
	clear(g.buf)
	if found == nil {
		return false
	}
	g.player = found.ID()
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *Tempt) CanContinueToUse() bool {
	if g.cfg.CanGetScared {
		p := g.actor(g.player)
		if p == nil {
			return false
		}
		pitch, yaw := rotation(p)
		if dist2(p.Pos(), g.mob.Pos()) >= temptScareDist2 {
			g.lastPos = p.Pos()
		} else if dist2(p.Pos(), g.lastPos) > temptScareMove2 ||
			abs32(pitch-g.lastPitch) > temptScareTurn || abs32(yaw-g.lastYaw) > temptScareTurn {
			return false
		}
		g.lastPitch, g.lastYaw = pitch, yaw
	}
	return g.CanUse()
}

// Start implements goal.Goal.
func (g *Tempt) Start() {
	if p := g.actor(g.player); p != nil {
		g.lastPos = p.Pos()
		g.lastPitch, g.lastYaw = rotation(p)
		g.mob.SetFlag(FlagTempted, true)
		// BDS fires on_tempt_start with only the mob in the context, so
		// the player is not the event's other.
		if g.cfg.OnTemptStart != "" {
			g.mob.Event(g.cfg.OnTemptStart, NoID)
		}
	}
	if n := g.mob.Nav(); n != nil && !g.float {
		f := n.Flags()
		g.avoided = f&uint32(nav.FlagAvoidWater) != 0
		n.SetFlags(f &^ uint32(nav.FlagAvoidWater))
	}
}

// Stop implements goal.Goal.
func (g *Tempt) Stop() {
	g.player = NoID
	g.cooldown = temptCooldown
	g.mob.SetFlag(FlagTempted, false)
	if g.cfg.OnTemptEnd != "" {
		g.mob.Event(g.cfg.OnTemptEnd, NoID)
	}
	// BDS's code/stop leave the navigation
	// alone; only TemptGoal's touch it.
	if n := g.mob.Nav(); n != nil && !g.float {
		n.Stop()
		f := n.Flags() &^ uint32(nav.FlagAvoidWater)
		if g.avoided {
			f |= uint32(nav.FlagAvoidWater)
		}
		n.SetFlags(f)
	}
}

// Tick implements goal.Goal.
func (g *Tempt) Tick() {
	g.repath--
	p := g.actor(g.player)
	if p == nil {
		return
	}
	if !g.mob.LookBusy() && !p.Removed() {
		g.mob.LookAt(lookPoint(p), temptLookYaw, g.mob.MaxHeadXRot())
	}
	// BDS aims at the middle of the box around the player and its vehicles,
	// but at the height of the middle of the player's own box.
	b := riddenBox(g.world, p)
	c, mp := boxCentre(b), g.mob.Pos()
	dx, dy, dz := c.X-mp.X, boxCentre(p.Box()).Y-mp.Y, c.Z-mp.Z
	if !g.cfg.CanTemptVertically {
		dy = 0
	}
	mb := g.mob.Box()
	stop := (b.Max.X-b.Min.X+(mb.Max.X-mb.Min.X))*0.5 + g.cfg.StopDistance
	n := g.mob.Nav()
	far := dz*dz+(dx*dx+dy*dy) >= stop*stop
	switch {
	case g.float:
		if far {
			g.floatToward(Vec3{X: dx, Y: dy, Z: dz})
		}
	case n == nil:
	case far:
		if g.repath <= 0 {
			n.MoveToActor(p, g.cfg.SpeedMultiplier)
			g.repath = g.world.Random().NextInt(temptRepathRandom) + temptRepathMin
		}
	default:
		n.Stop()
	}
	playSound(g.world, g.mob, g.cfg.Sound, g.cfg.SoundInterval, g.world.CurrentTick(), &g.nextSound)
}

// floatToward adds to the mob's motion along d, which is at least
// floatTemptMinLen long for the normalising.
func (g *Tempt) floatToward(d Vec3) {
	inv := 1 / max(floatTemptMinLen, sqrt32(d.Z*d.Z+(d.X*d.X+d.Y*d.Y)))
	s, k := g.mob.Speed(), g.cfg.SpeedMultiplier
	v := g.mob.Velocity()
	g.mob.SetVelocity(Vec3{X: v.X + k*(s*(d.X*inv)), Y: v.Y + k*(s*(d.Y*inv)), Z: v.Z + k*(s*(d.Z*inv))})
}

// riddenBox is the box around the entity and everything it rides.
func riddenBox(w World, a Actor) Box {
	b := a.Box()
	// The cap guards against a vehicle loop in host data.
	for i, v := 0, w.Actor(a.Vehicle()); v != nil && i < maxRideDepth; i, v = i+1, w.Actor(v.Vehicle()) {
		vb := v.Box()
		b.Min = Vec3{X: min(b.Min.X, vb.Min.X), Y: min(b.Min.Y, vb.Min.Y), Z: min(b.Min.Z, vb.Min.Z)}
		b.Max = Vec3{X: max(b.Max.X, vb.Max.X), Y: max(b.Max.Y, vb.Max.Y), Z: max(b.Max.Z, vb.Max.Z)}
	}
	return b
}

// rotation is ActorRotationComponent's pitch and yaw, which the scare check
// reads. For a player its yaw is the view yaw, which entity's Actor gives as
// BodyYaw.
func rotation(a Actor) (pitch, yaw float32) {
	pitch, _ = a.HeadRotation()
	return pitch, a.BodyYaw()
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
