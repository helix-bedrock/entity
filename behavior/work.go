package behavior

// The work numbers that are pure maths.
const (
	workStampEvery = 1200 // worked ticks between the dweller's work stamps
	workLook       = 30
	workLookY      = 1.5 // the look point's height over the job site block
)

// Worker is the part of the mob work needs on top of Dweller.
type Worker interface {
	Dweller
	// RainingAt reports rain on p: the biome there has rain, the level
	// rains harder than 0.2 and the rain reaches p (0xC8D60C0).
	RainingAt(p Pos) bool
	// PlayWorkSound plays the job site's sound event (0x54FDF30).
	PlayWorkSound(sound string)
	// StampWork tells the village the dweller worked (0xBD1D0F0).
	StampWork()
}

// WorkConfig is the JSON of minecraft:behavior.work.
type WorkConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// The times are in ticks.
	SoundDelayMin       int32    `json:"sound_delay_min"`
	SoundDelayMax       int32    `json:"sound_delay_max"`
	GoalCooldown        int32    `json:"goal_cooldown"`
	ActiveTime          int32    `json:"active_time"`
	CanWorkInRain       bool     `json:"can_work_in_rain"`
	WorkInRainTolerance int32    `json:"work_in_rain_tolerance"`
	OnArrival           []string `json:"on_arrival"`
}

// DefaultWork returns the goal's BDS defaults (TypeSchema 0xE0F4FA0).
func DefaultWork() WorkConfig {
	return WorkConfig{SpeedMultiplier: 0.5, WorkInRainTolerance: -1}
}

// workStep is the slot 21 a work goal runs each tick at the job site.
type workStep interface {
	atWork()
}

// workGoal is WorkGoal, which work_composter extends.
type workGoal struct {
	moveToPOI
	w    Worker
	cfg  WorkConfig
	step workStep
	// cooldown, worked, soundTime and soundDelay are +212, +216, +220 and
	// +224; rainCheck is +232.
	rest, worked, soundTime, soundDelay, rainCheck int32
	arrived                                        bool
	sound                                          string
}

func (g *workGoal) initWork(w World, m Worker, c WorkConfig, step workStep, h moveToHooks) {
	g.initPOI(w, m, MoveToPOIConfig{SpeedMultiplier: c.SpeedMultiplier, POIType: POIJobSite}, h, g)
	g.w, g.cfg, g.step, g.sameY = m, c, step, true
	g.flags = flagsMoveLook
}

// rainedOff is the rain test of canUse and canContinue.
func (g *workGoal) rainedOff() bool {
	return !g.cfg.CanWorkInRain && g.w.RainingAt(blockOf(g.aim))
}

// canUseWork is canUse (0x54FE1C0).
func (g *workGoal) canUseWork() bool {
	if g.rest > 0 {
		g.rest--
		if g.rest != 0 {
			return false
		}
	}
	if !g.canUse() {
		return false
	}
	poi, ok := g.w.POI(POIJobSite)
	if !ok {
		return false
	}
	g.aim = vecOf(poi.Pos)
	if g.rainedOff() {
		return false
	}
	g.sound = poi.Sound
	return true
}

// CanContinueToUse implements goal.Goal (0x54FE680).
func (g *workGoal) CanContinueToUse() bool {
	if !g.keepLite() {
		return false
	}
	left := g.cfg.ActiveTime - g.worked
	if g.cfg.ActiveTime > 0 && left < 0 {
		return false
	}
	if g.cfg.CanWorkInRain {
		return true
	}
	t := g.rainCheck
	g.rainCheck++
	if t < 0 {
		return true
	}
	g.rainCheck = 0
	if g.cfg.WorkInRainTolerance >= 0 && left <= g.cfg.WorkInRainTolerance {
		return true
	}
	return !g.w.RainingAt(blockOf(g.aim))
}

// startWork is start (0x54FD380).
func (g *workGoal) startWork() {
	g.startPOI()
	g.rest, g.worked, g.soundTime = 0, 0, 0
	g.soundDelay = randomInt(g.world.Random(), g.cfg.SoundDelayMin, g.cfg.SoundDelayMax)
}

// Stop implements goal.Goal (0x54FE810).
func (g *workGoal) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.soundTime, g.rest = 0, g.cfg.GoalCooldown
	g.aim, g.sound, g.rainCheck = Vec3{}, "", 0
	g.stopPOI()
}

// Tick implements goal.Goal (0x54FE970).
func (g *workGoal) Tick() {
	if !g.mob.LookBusy() {
		g.mob.LookAt(Vec3{X: g.aim.X + 0.5, Y: g.aim.Y + workLookY, Z: g.aim.Z + 0.5}, workLook, workLook)
	}
	g.tickPOI()
	if !g.reached {
		return
	}
	g.step.atWork()
	if g.worked%workStampEvery == 0 {
		g.w.StampWork()
	}
	g.worked++
	if !g.arrived {
		g.arrived = true
		for _, e := range g.cfg.OnArrival {
			g.mob.Event(e, g.mob.ID())
		}
	}
}

// Work is WorkGoal: a villager walks to its job site and works there.
type Work struct{ workGoal }

// NewWork makes the goal (0x54FD090).
func NewWork(w World, m Worker, c WorkConfig) *Work {
	g := &Work{}
	g.initWork(w, m, c, g, g)
	return g
}

// find is unused by the POI goals.
func (g *Work) find() bool { return false }

// CanUse implements goal.Goal.
func (g *Work) CanUse() bool { return g.canUseWork() }

// Start implements goal.Goal.
func (g *Work) Start() { g.startWork() }

// atWork is slot 21 (0x54FECC0): the work sound every sound delay.
func (g *Work) atWork() {
	if g.soundTime > g.soundDelay {
		g.soundDelay = randomInt(g.world.Random(), g.cfg.SoundDelayMin, g.cfg.SoundDelayMax)
		g.soundTime = 0
		g.w.PlayWorkSound(g.sound)
	}
	g.soundTime++
}

// Composter is the part of the mob work_composter needs on top of Worker.
// The items are the engine's.
type Composter interface {
	Worker
	// EmptyComposter takes the bone meal out of a full composter at p;
	// false when there was none.
	EmptyComposter(p Pos) bool
	// FillComposter puts up to max of one compostable item the mob holds
	// at least minCount of into the composter at p while its fill level is
	// 6 or less; false when it put none.
	FillComposter(p Pos, max, minCount int32) bool
}

// WorkComposterConfig is the JSON of minecraft:behavior.work_composter.
type WorkComposterConfig struct {
	WorkConfig
	// The use times are in ticks.
	UseBlockMin         int32 `json:"use_block_min"`
	UseBlockMax         int32 `json:"use_block_max"`
	ItemsPerUseMax      int32 `json:"items_per_use_max"`
	BlockInteractionMax int32 `json:"block_interaction_max"`
	CanFillComposter    bool  `json:"can_fill_composter"`
	CanEmptyComposter   bool  `json:"can_empty_composter"`
	MinItemCount        int32 `json:"min_item_count"`
}

// DefaultWorkComposter returns the goal's BDS defaults (TypeSchema
// 0xE0F3EC0).
func DefaultWorkComposter() WorkComposterConfig {
	return WorkComposterConfig{WorkConfig: DefaultWork(), CanFillComposter: true, CanEmptyComposter: true, MinItemCount: 10}
}

// WorkComposter is WorkComposterGoal: a farmer works at its composter,
// emptying and filling it.
type WorkComposter struct {
	workGoal
	c       Composter
	ccfg    WorkComposterConfig
	uses    int32
	nextUse uint64
}

// NewWorkComposter makes the goal.
func NewWorkComposter(w World, m Composter, c WorkComposterConfig) *WorkComposter {
	g := &WorkComposter{c: m, ccfg: c}
	g.initWork(w, m, c.WorkConfig, g, g)
	return g
}

// find is unused by the POI goals.
func (g *WorkComposter) find() bool { return false }

// CanUse implements goal.Goal.
func (g *WorkComposter) CanUse() bool { return g.canUseWork() }

// Start implements goal.Goal (0x54FD200).
func (g *WorkComposter) Start() {
	g.startWork()
	g.uses, g.nextUse = 0, 0
}

// atWork is slot 21 (0x54FD4F0).
func (g *WorkComposter) atWork() {
	now := g.world.CurrentTick()
	if g.nextUse == 0 {
		g.nextUse = now + g.useTime()
	}
	if g.uses >= g.ccfg.BlockInteractionMax || now < g.nextUse {
		return
	}
	at := blockOf(g.aim)
	used := g.ccfg.CanEmptyComposter && g.c.EmptyComposter(at)
	if g.ccfg.CanFillComposter && g.ccfg.ItemsPerUseMax != 0 {
		n := g.ccfg.ItemsPerUseMax
		if n == -1 {
			n = maxInt32
		}
		if g.c.FillComposter(at, n, g.ccfg.MinItemCount) {
			used = true
		}
	}
	if !used {
		g.w.PlayWorkSound(g.sound)
	}
	g.uses++
	g.nextUse = now + g.useTime()
}

// useTime is the wait to the next use.
func (g *WorkComposter) useTime() uint64 {
	return uint64(randomInt(g.world.Random(), g.ccfg.UseBlockMin, g.ccfg.UseBlockMax))
}

const maxInt32 = 1<<31 - 1
