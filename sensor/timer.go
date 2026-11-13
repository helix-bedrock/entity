package sensor

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// TimeChoice is one entry of random_time_choices.
type TimeChoice struct {
	Weight int32
	// Value is in seconds.
	Value float32
}

// TimerConfig is minecraft:timer.
type TimerConfig struct {
	// Looping restarts the timer each time it fires; otherwise it fires
	// once.
	Looping bool
	// RandomInterval draws a new time each restart; otherwise the time
	// drawn when the timer was made is kept.
	RandomInterval bool
	// Time is the range a time is drawn from, in seconds.
	Time behavior.Range[float32]
	// Choices, when not empty, are drawn from by weight instead of Time.
	Choices []TimeChoice
	// Event is time_down_event.
	Event *definition.Trigger
}

// DefaultTimer is BDS's timer: looping, random interval, time 0
// (TimerDefinition ctor, 1.21.3 0x14061D3E0; the 1.26.60 schema
// 0x745E5E0 reads the same members).
func DefaultTimer() TimerConfig { return TimerConfig{Looping: true, RandomInterval: true} }

// Timer fires an event after a time (TimerComponent; 1.21.3 tick
// 0x140B1B970, getRandomTime 0x140637A20, restartTimer 0x14063E510,
// initialize 0x140639690).
type Timer struct {
	cfg TimerConfig
	// min and max are Time in ticks.
	min, max int32
	// time is the current interval in ticks; stamp is the tick it ends.
	time  int32
	stamp uint64
	fired bool
}

// NewTimer starts a timer for env.Self.
func NewTimer(c TimerConfig, env *Env) *Timer {
	t := &Timer{}
	t.SetConfig(c)
	// BDS draws one time, then draws again when the interval is random.
	t.time = t.randomTime(env.World)
	t.restart(env.World)
	return t
}

// Config returns the timer's settings.
func (t *Timer) Config() TimerConfig { return t.cfg }

// SetConfig changes the settings; the running interval is kept.
func (t *Timer) SetConfig(c TimerConfig) {
	t.cfg = c
	t.min, t.max = ticks(c.Time.Min), ticks(c.Time.Max)
}

// Remaining is the number of ticks until the timer fires, 0 when due.
func (t *Timer) Remaining(now uint64) uint64 {
	if now >= t.stamp {
		return 0
	}
	return t.stamp - now
}

func (t *Timer) randomTime(w behavior.World) int32 {
	if len(t.cfg.Choices) == 0 {
		return t.min + w.Random().NextInt(max(t.max-t.min, 1))
	}
	var total int32
	for _, c := range t.cfg.Choices {
		total += c.Weight
	}
	var r int32
	if total > 0 {
		r = w.IRandom().NextInt(total)
	}
	for _, c := range t.cfg.Choices {
		r -= c.Weight
		if r < 0 {
			return ticks(c.Value)
		}
	}
	// BDS reads through a null entry here (all weights 0); 0 keeps going.
	return 0
}

func (t *Timer) restart(w behavior.World) {
	if t.cfg.RandomInterval {
		t.time = t.randomTime(w)
	}
	t.stamp = uint64(max(int64(w.CurrentTick())+int64(t.time), 0))
}

// Tick fires time_down_event once the time is up, with self's target as
// the target subject. A looping timer starts again; a dead actor's timer
// waits.
func (t *Timer) Tick(env *Env) {
	if !env.Self.Alive() || t.fired && !t.cfg.Looping {
		return
	}
	if t.stamp > env.World.CurrentTick() {
		return
	}
	t.fired = true
	t.restart(env.World)
	env.fire(t.cfg.Event, env.tester(nil))
}
