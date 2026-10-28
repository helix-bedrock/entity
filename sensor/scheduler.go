package sensor

import "github.com/helix-bedrock/entity/definition"

// SchedulerConfig is minecraft:scheduler.
type SchedulerConfig struct {
	// MinDelay and MaxDelay are min_delay_secs and max_delay_secs. BDS
	// reads them (1.26.60 0x715A210) but the 1.21.3 tick does not use
	// them; they are kept for servers that do.
	MinDelay, MaxDelay float32
	// Events are scheduled_events: an event and the filters that pick it.
	Events []*definition.Trigger
}

// DefaultScheduler is BDS's scheduler: no events, no delays
// (SchedulerDefinition ctor, 1.21.3 0x14061CD90).
func DefaultScheduler() SchedulerConfig { return SchedulerConfig{} }

// Scheduler fires the first scheduled event whose filters pass, once each
// time the passing event changes (1.21.3 tickSchedulerComponent
// 0x140B576C0).
type Scheduler struct {
	cfg SchedulerConfig
	// current is the index of the event fired last, -1 for none.
	current int
}

// NewScheduler makes a scheduler.
func NewScheduler(c SchedulerConfig) *Scheduler { return &Scheduler{cfg: c, current: -1} }

// Config returns the scheduler's settings.
func (s *Scheduler) Config() SchedulerConfig { return s.cfg }

// SetConfig changes the settings and forgets the event fired last.
func (s *Scheduler) SetConfig(c SchedulerConfig) { s.cfg, s.current = c, -1 }

// Current is the index of the event fired last, -1 for none.
func (s *Scheduler) Current() int { return s.current }

// Tick keeps the event fired last while its filters still pass; otherwise
// it fires the first event whose filters pass.
func (s *Scheduler) Tick(env *Env) {
	ctx := env.tester(nil)
	if s.current >= 0 && s.current < len(s.cfg.Events) && s.cfg.Events[s.current].Filters.Eval(ctx) {
		return
	}
	s.current = -1
	for i, t := range s.cfg.Events {
		if t != nil && t.Filters.Eval(ctx) {
			s.current = i
			env.Events.RunTrigger(t, ctx)
			return
		}
	}
}
