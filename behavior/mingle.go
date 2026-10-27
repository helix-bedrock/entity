package behavior

// The mingle states (MingleComponent +0).
const (
	mingleNone = iota
	mingleSearching
	mingleApproaching
	minglePartnered
	mingleMingling
)

// The mingle numbers that are pure maths.
const (
	mingleScanEvery = 10 // searching ticks between partner scans
	mingleLook      = 30
)

// MingleState is a mob's MingleComponent: its state, partner and last
// partner.
type MingleState struct {
	State         int32
	Partner, Last ID
}

// Mingler is the part of the mob mingle needs on top of Dweller.
type Mingler interface {
	Dweller
	// Mingle is the entity's MingleComponent; ok is false without one.
	Mingle(id ID) (MingleState, bool)
	// SetMingle replaces the entity's MingleComponent, adding it when it
	// has none.
	SetMingle(id ID, s MingleState)
}

// MingleConfig is the JSON of minecraft:behavior.mingle.
type MingleConfig struct {
	MoveToPOIConfig
	// Duration and CooldownTime are in seconds.
	Duration          float32 `json:"duration"`
	CooldownTime      float32 `json:"cooldown_time"`
	MinglePartnerType string  `json:"mingle_partner_type"`
	MingleDistance    float32 `json:"mingle_distance"`
	// Sound is the level sound event played while mingling.
	Sound int `json:"-"`
	// SoundMin and SoundSpan give the ticks between sounds.
	SoundMin, SoundSpan int32 `json:"-"`
}

// DefaultMingle returns the goal's BDS defaults (TypeSchema 0xDF74BB0,
// the sound from the tick 0x5272A50).
func DefaultMingle() MingleConfig {
	return MingleConfig{MoveToPOIConfig: DefaultMoveToPOI(), Duration: 1, MingleDistance: 2,
		Sound: 122, SoundMin: 50, SoundSpan: 50}
}

// Mingle is MingleGoal: villagers meet at the meeting area and chat in
// pairs.
type Mingle struct {
	moveToPOI
	m   Mingler
	cfg MingleConfig
	// waited, left, search and soundIn are +184, +192, +200 and +208.
	waited, left, search, soundIn int32
	cooldown, duration            int32
	dist2                         float32
	near                          []Actor
}

// NewMingle makes the goal (0x532D3C0).
func NewMingle(w World, m Mingler, c MingleConfig) *Mingle {
	g := &Mingle{m: m, cfg: c, cooldown: int32(c.CooldownTime * ticksPerSecond),
		duration: int32(c.Duration * ticksPerSecond), dist2: c.MingleDistance * c.MingleDistance}
	g.initPOI(w, m, c.MoveToPOIConfig, g, g)
	g.flags = flagsMoveLook | flagsBit3
	return g
}

// find is unused by the POI goals.
func (g *Mingle) find() bool { return false }

func (g *Mingle) state(id ID) MingleState {
	s, _ := g.m.Mingle(id)
	return s
}

// CanUse implements goal.Goal (0x5272300).
func (g *Mingle) CanUse() bool {
	w := g.waited
	g.waited++
	return w >= g.cooldown && g.canUse()
}

// CanContinueToUse implements goal.Goal (0x52723F0).
func (g *Mingle) CanContinueToUse() bool {
	if g.left <= 0 || !g.keepLite() {
		return false
	}
	s := g.state(g.mob.ID())
	switch s.State {
	case mingleNone:
		return !g.reached
	case mingleSearching:
		return true
	case mingleApproaching:
		return g.partnerIs(s.Partner, minglePartnered, false)
	case minglePartnered:
		return g.partnerIs(s.Partner, mingleApproaching, false)
	case mingleMingling:
		return g.partnerIs(s.Partner, mingleMingling, true)
	}
	return false
}

// partnerIs is 0x5272520: the partner is in the state with the mob as its
// partner, and near enough when near is set.
func (g *Mingle) partnerIs(id ID, state int32, near bool) bool {
	p := g.actor(id)
	if p == nil {
		return false
	}
	s, ok := g.m.Mingle(id)
	if !ok || s.Partner != g.mob.ID() || s.State != state {
		return false
	}
	return !near || g.dist2 > dist2(g.mob.Pos(), p.Pos())
}

// Start implements goal.Goal (0x52726F0).
func (g *Mingle) Start() {
	g.startPOI()
	s := g.state(g.mob.ID())
	s.State = mingleNone
	g.m.SetMingle(g.mob.ID(), s)
	g.left, g.search = g.duration, g.cooldown
	g.soundIn = g.cfg.SoundMin + g.world.IRandom().NextInt(g.cfg.SoundSpan)
}

// Stop implements goal.Goal (0x5272840).
func (g *Mingle) Stop() {
	id := g.mob.ID()
	s := g.state(id)
	if g.actor(s.Partner) != nil {
		if p, ok := g.m.Mingle(s.Partner); ok {
			p.State, p.Partner = mingleNone, NoID
			g.m.SetMingle(s.Partner, p)
		}
	}
	if s.State == mingleSearching {
		s.Last = NoID
	}
	s.State, s.Partner = mingleNone, NoID
	g.m.SetMingle(id, s)
	g.waited = 0
	g.stopPOI()
}

// Tick implements goal.Goal (0x5272A50).
func (g *Mingle) Tick() {
	g.tickPOI()
	id := g.mob.ID()
	s := g.state(id)
	switch s.State {
	case mingleNone:
		if g.reached {
			s.State = mingleSearching
			g.m.SetMingle(id, s)
			g.stopNav()
		}
	case mingleSearching:
		if g.search <= 0 {
			s.State = mingleNone
			g.m.SetMingle(id, s)
			return
		}
		g.search--
		if g.search%mingleScanEvery == 0 {
			g.scan(s)
		}
	case mingleApproaching:
		p := g.actor(s.Partner)
		if p == nil {
			return
		}
		if g.dist2 <= dist2(g.mob.Pos(), p.Pos()) {
			if n := g.mob.Nav(); n != nil && n.Done() {
				n.MoveTo(p.Pos(), g.cfg.SpeedMultiplier)
			}
			return
		}
		s.State = mingleMingling
		g.m.SetMingle(id, s)
		if ps, ok := g.m.Mingle(s.Partner); ok {
			ps.State = mingleMingling
			g.m.SetMingle(s.Partner, ps)
		}
		g.stopNav()
	case minglePartnered:
		g.stopNav()
		g.lookAt(s.Partner)
	case mingleMingling:
		g.left--
		g.lookAt(s.Partner)
		if g.soundIn > 0 && g.left%g.soundIn == 0 {
			g.mob.PlaySoundAt(g.cfg.Sound, g.mob.Eyes())
			g.soundIn = g.cfg.SoundMin + g.world.IRandom().NextInt(g.cfg.SoundSpan)
		}
	}
}

// scan looks for a partner in the meeting area (the searching branch of
// the tick).
func (g *Mingle) scan(s MingleState) {
	poi, ok := g.m.POI(g.kind)
	if !ok {
		return
	}
	c := Vec3{X: float32(poi.Pos.X) + 0.5, Y: float32(poi.Pos.Y) + 0.5, Z: float32(poi.Pos.Z) + 0.5}
	r := poi.Radius
	area := Box{Min: Vec3{X: c.X - r, Y: c.Y - r, Z: c.Z - r}, Max: Vec3{X: c.X + r, Y: c.Y + r, Z: c.Z + r}}
	id := g.mob.ID()
	g.near = g.world.Entities(g.near[:0], id, area)
	for _, a := range g.near {
		if a.ID() == id || a.Type() != g.cfg.MinglePartnerType || a.Flag(FlagBaby) {
			continue
		}
		o, ok := g.m.Mingle(a.ID())
		if !ok || o.State != mingleSearching || o.Last == id || a.ID() == s.Last {
			continue
		}
		if n := g.mob.Nav(); n == nil || !n.MoveTo(a.Pos(), g.cfg.SpeedMultiplier) {
			continue
		}
		g.pair(a.ID())
		return
	}
}

// pair is 0x5273A50: the other waits for the mob, which walks over.
func (g *Mingle) pair(other ID) {
	id := g.mob.ID()
	o := g.state(other)
	o.State = minglePartnered
	o.Last, o.Partner = o.Partner, id
	g.m.SetMingle(other, o)
	s := g.state(id)
	s.State = mingleApproaching
	if other != NoID {
		s.Last = s.Partner
	}
	s.Partner = other
	g.m.SetMingle(id, s)
}

func (g *Mingle) stopNav() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
}

// lookAt is 0x5273920.
func (g *Mingle) lookAt(id ID) {
	if p := g.actor(id); p != nil && !p.Removed() {
		g.mob.LookAt(lookPoint(p), mingleLook, mingleLook)
	}
}
