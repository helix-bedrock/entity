package behavior

import "github.com/helix-bedrock/entity/goal"

// EventSender is the mob send_event runs on.
type EventSender interface {
	Mob
	// SendEvent fires an entity event on another entity.
	SendEvent(to ID, event string)
	// SetSpellColor sets the mob's spell particle colour actor data, as
	// ARGB.
	SetSpellColor(argb int32)
}

// SendEventStage is one entry of an event choice's sequence.
type SendEventStage struct {
	// BaseDelay is in ticks, counted from the stage before.
	BaseDelay  int32
	Event      string
	SoundEvent int
}

// SendEventChoice is one entry of event_choices.
type SendEventChoice struct {
	MinActivationRange, MaxActivationRange float32
	// CooldownTime is in ticks.
	CooldownTime int32
	Weight       float32
	LookAtTarget bool
	DoCasting    bool
	// ParticleColor is ARGB.
	ParticleColor   int32
	StartSoundEvent int
	// Filters pick the entity the events go to.
	Filters Filter
	// CastDuration is in ticks; the loader sets the sum of the stage
	// delays (TotalDelay) when the JSON has none or it is 0 or less.
	CastDuration int32
	Sequence     []SendEventStage
}

// DefaultSendEventChoice returns a choice's BDS defaults (the 1.26.60
// conversion 0xBEC3C00; the same numbers as the older parser in
// 0xBF93840).
func DefaultSendEventChoice() SendEventChoice {
	return SendEventChoice{MinActivationRange: 1, MaxActivationRange: 32, LookAtTarget: true, DoCasting: true, ParticleColor: -1, StartSoundEvent: NoSound}
}

// TotalDelay is the sum of the stages' base delays, the default
// cast_duration.
func (c SendEventChoice) TotalDelay() int32 {
	var t int32
	for _, s := range c.Sequence {
		t += s.BaseDelay
	}
	return t
}

// SendEventConfig is the JSON of minecraft:behavior.send_event.
type SendEventConfig struct {
	EventChoices []SendEventChoice
	// LookYaw is the look control's yaw limit; the pitch limit is the
	// mob's MaxHeadXRot.
	LookYaw float32
	// CastFlags are the control flags the goal is made and stopped with,
	// IdleFlags the ones once a cast ends.
	CastFlags, IdleFlags goal.Flags
}

// DefaultSendEvent returns the goal's BDS defaults (create 0x54F5050, look
// limit from tick 0xBFA0B30).
func DefaultSendEvent() SendEventConfig {
	return SendEventConfig{LookYaw: 10, CastFlags: flagsMoveLook | flagCast, IdleFlags: flagCast}
}

// SendEvent is SendEventGoal (canUse 0xBF9FF50, canContinue 0xBFA06C0,
// start 0xBFA0770, stop 0xBFA0AE0, tick 0xBFA0B30): a weighted pick of the
// choices that have an entity in range passing their filters, then each
// stage's event fired on that entity after its delay.
type SendEvent struct {
	base
	m   EventSender
	cfg SendEventConfig
	// to is the entity the events go to, choice the picked choice (-1 for
	// none).
	to          ID
	choice      int
	stage       int32
	delay, cast int32
	cooldownEnd uint64
	picks       []int
	found       []ID
	buf         []Actor
}

// NewSendEvent makes the goal.
func NewSendEvent(w World, m EventSender, c SendEventConfig) *SendEvent {
	return &SendEvent{base: base{mob: m, world: w, flags: c.CastFlags}, m: m, cfg: c, to: NoID, choice: -1}
}

// Config returns the goal's settings.
func (g *SendEvent) Config() SendEventConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *SendEvent) SetConfig(c SendEventConfig) { g.cfg = c }

func (g *SendEvent) picked() *SendEventChoice {
	if g.choice < 0 || g.choice >= len(g.cfg.EventChoices) {
		return nil
	}
	return &g.cfg.EventChoices[g.choice]
}

// CanUse implements goal.Goal.
func (g *SendEvent) CanUse() bool {
	g.picks, g.found = g.picks[:0], g.found[:0]
	weight := int32(0)
	for i := range g.cfg.EventChoices {
		c := &g.cfg.EventChoices[i]
		if a := g.first(c); a != nil {
			g.picks = append(g.picks, i)
			g.found = append(g.found, a.ID())
			weight = int32(float32(weight) + c.Weight)
		}
	}
	g.choice = -1
	if len(g.picks) > 0 && weight > 0 {
		r := g.world.Random().NextInt(weight)
		for k, i := range g.picks {
			w := g.cfg.EventChoices[i].Weight
			if w > float32(r) {
				g.choice = i
				g.to = g.found[k]
				break
			}
			r = int32(float32(r) - w)
		}
	}
	c := g.picked()
	return c != nil && len(c.Sequence) > 0 && g.cooldownEnd <= g.world.CurrentTick()
}

// first is the first entity in the choice's range around the mob's box
// that passes its filters.
func (g *SendEvent) first(c *SendEventChoice) Actor {
	b, r := g.m.Box(), c.MaxActivationRange
	lo := Vec3{X: b.Min.X - r, Y: b.Min.Y - r, Z: b.Min.Z - r}
	hi := Vec3{X: b.Max.X + r, Y: b.Max.Y + r, Z: b.Max.Z + r}
	g.buf = g.world.Entities(g.buf[:0], g.m.ID(), Box{
		Min: Vec3{X: min(lo.X, hi.X), Y: min(lo.Y, hi.Y), Z: min(lo.Z, hi.Z)},
		Max: Vec3{X: max(lo.X, hi.X), Y: max(lo.Y, hi.Y), Z: max(lo.Z, hi.Z)},
	})
	defer clear(g.buf)
	for _, a := range g.buf {
		d := dist2(g.m.Pos(), a.Pos())
		if c.MaxActivationRange*c.MaxActivationRange >= d && d >= c.MinActivationRange*c.MinActivationRange &&
			(c.Filters == nil || c.Filters(g.m, a)) {
			return a
		}
	}
	return nil
}

// CanContinueToUse implements goal.Goal: while casting, then until every
// stage has fired, as long as the entity is there.
func (g *SendEvent) CanContinueToUse() bool {
	c := g.picked()
	if g.actor(g.to) == nil || c == nil {
		return false
	}
	return g.cast > 0 || int(g.stage) < len(c.Sequence)
}

// Start implements goal.Goal.
func (g *SendEvent) Start() {
	g.stage, g.delay = 0, -1
	c := g.picked()
	if c == nil {
		return
	}
	if c.StartSoundEvent != NoSound {
		g.m.PlaySoundAt(c.StartSoundEvent, g.m.Pos())
	}
	g.cast = c.CastDuration
	g.m.SetSpellColor(c.ParticleColor)
	if c.DoCasting {
		g.m.SetFlag(FlagCasting, g.cast > 0)
	}
	if len(c.Sequence) > 0 && c.Sequence[0].SoundEvent != NoSound {
		g.m.PlaySoundAt(c.Sequence[0].SoundEvent, g.m.Pos())
	}
}

// Stop implements goal.Goal.
func (g *SendEvent) Stop() {
	g.flags = g.cfg.CastFlags
	g.m.SetFlag(FlagCasting, false)
	if c := g.picked(); c != nil {
		g.cooldownEnd = g.world.CurrentTick() + uint64(c.CooldownTime)
	}
}

// Tick implements goal.Goal.
func (g *SendEvent) Tick() {
	c := g.picked()
	t := g.actor(g.to)
	if c == nil || t == nil || t.Removed() {
		return
	}
	if c.LookAtTarget {
		g.m.LookAt(t.Eyes(), g.cfg.LookYaw, g.m.MaxHeadXRot())
	}
	if g.cast--; g.cast == 0 {
		g.flags = g.cfg.IdleFlags
		g.m.SetFlag(FlagCasting, false)
	}
	if int(g.stage) >= len(c.Sequence) {
		return
	}
	g.delay++
	for s := &c.Sequence[g.stage]; g.delay >= s.BaseDelay; {
		g.m.SendEvent(t.ID(), s.Event)
		g.delay -= s.BaseDelay
		g.stage++
		if int(g.stage) >= len(c.Sequence) {
			return
		}
		s = &c.Sequence[g.stage]
		if s.SoundEvent != NoSound {
			g.m.PlaySoundAt(s.SoundEvent, g.m.Pos())
		}
	}
}
