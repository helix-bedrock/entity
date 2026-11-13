package behavior

// The hide numbers that are pure maths.
const (
	hideRepath    = 40 // HideGoal's repath interval (slot 18)
	hideMaxTries  = 2  // failed trips after which the own POI is skipped
	hideMaxFailed = 3  // failed trips after which the hide request ends
)

// Hider is the part of the mob hide needs on top of Dweller.
type Hider interface {
	Dweller
	// Hide is the HideComponent: hiding is set while a raid sends the mob
	// in, triggered while something else (a bell) does; ok is false
	// without the component.
	Hide() (hiding, triggered, ok bool)
	// SetHide replaces the HideComponent's two flags.
	SetHide(hiding, triggered bool)
	Trader
	// Raid reports a raid on the dweller's village (+648) and whether it
	// has started (raid +4 == 0).
	Raid() (exists, running bool)
	// CanReach reports a path to p (nav vt+40).
	CanReach(p Vec3) bool
}

// HideConfig is the JSON of minecraft:behavior.hide.
type HideConfig struct {
	MoveToPOIConfig
	// Duration is in seconds.
	Duration float32 `json:"duration"`
}

// DefaultHide returns the goal's BDS defaults (TypeSchema 0xDF6BF90).
func DefaultHide() HideConfig {
	return HideConfig{MoveToPOIConfig: DefaultMoveToPOI(), Duration: 1}
}

// Hide is HideGoal: villagers run to their POI (their bed) when a raid or a
// bell sends them in.
type Hide struct {
	moveToPOI
	m      Hider
	cfg    HideConfig
	failed int16
}

// NewHide makes the goal (0x52BBA60).
func NewHide(w World, m Hider, c HideConfig) *Hide {
	g := &Hide{m: m, cfg: c}
	g.initPOI(w, m, c.MoveToPOIConfig, g, g)
	g.repath = hideRepath
	g.maxStay = int32(c.Duration * ticksPerSecond)
	return g
}

// find is unused by the POI goals.
func (g *Hide) find() bool { return false }

// lookup is the goal's POI lookup (0x5749A30): its own POI while it can
// get there, else the village's nearest free one.
func (g *Hide) lookup(kind POIKind) (POI, bool) {
	if p, ok := g.m.POI(kind); ok && g.failed < hideMaxTries && g.m.CanReach(vecOf(p.Pos)) {
		return p, true
	}
	return g.m.FreePOI(kind, blockOf(g.mob.Pos()))
}

// CanUse implements goal.Goal (0x5749130).
func (g *Hide) CanUse() bool {
	hiding, triggered, ok := g.m.Hide()
	if !ok {
		return false
	}
	// A villager trading with a player (actor data 68) does not hide.
	if g.actor(g.m.TradingPlayer()) != nil {
		if triggered {
			s := g.stay
			g.stay++
			if s >= g.maxStay {
				g.m.SetHide(false, false)
				g.stay = 0
			}
		}
		return false
	}
	if triggered {
		// 0x57494E0: a raid that has started takes over the request.
		if _, running := g.m.Raid(); !hiding && running {
			g.m.SetHide(true, triggered)
		}
		return g.canUse()
	}
	if !g.m.IsDweller() {
		return false
	}
	if _, running := g.m.Raid(); running {
		g.maxStay = maxInt32
		g.m.SetHide(true, false)
		return g.canUse()
	}
	g.m.SetHide(false, false)
	return false
}

// CanContinueToUse implements goal.Goal (0x57497C0): a hiding mob stays
// while the raid lasts.
func (g *Hide) CanContinueToUse() bool {
	if hiding, _, ok := g.m.Hide(); ok && hiding && g.m.IsDweller() {
		if exists, _ := g.m.Raid(); exists {
			return true
		}
	}
	return g.keepLite()
}

// Start implements goal.Goal (0x5749740).
func (g *Hide) Start() {
	g.hooks.moveToBlock()
	g.last = blockOf(g.mob.Pos())
	g.cooldown, g.tries = 0, 0
}

// Stop implements goal.Goal (0x5749F10).
func (g *Hide) Stop() {
	if hiding, triggered, ok := g.m.Hide(); ok {
		switch {
		case hiding:
			g.reset()
		case triggered:
			if g.stay > g.maxStay || g.failed > hideMaxFailed {
				g.reset()
			} else {
				g.failed++
			}
		case g.failed > hideMaxFailed:
			g.reset()
		}
	}
	g.moveTo.Stop()
	g.box = false
	g.stay = 0
}

func (g *Hide) reset() {
	g.m.SetHide(false, false)
	g.failed = 0
}

// Tick implements goal.Goal (0x5273610).
func (g *Hide) Tick() { g.tickPOI() }
