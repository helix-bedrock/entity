package behavior

// chargeAim is the actor location charge_attack flies at (map?: BDS
// passes 6 to getAttachPos).
const chargeAim = 6

// ChargeAttackConfig is the JSON of minecraft:behavior.charge_attack.
type ChargeAttackConfig struct {
	MinDistance     float32 `json:"min_distance"`
	MaxDistance     float32 `json:"max_distance"`
	SpeedMultiplier float32 `json:"speed_multiplier"`
	SuccessRate     float32 `json:"success_rate"`
	SoundEvent      int     `json:"sound_event"`
}

// DefaultChargeAttack is the goal's BDS defaults.
var DefaultChargeAttack = ChargeAttackConfig{MinDistance: 2, MaxDistance: 3, SpeedMultiplier: 1, SuccessRate: 0.1428, SoundEvent: 134}

// ChargeAttack is ChargeAttackGoal: a flying mob dashes at its target with
// the move control and hits it when their boxes touch.
type ChargeAttack struct {
	base
	m   Attacker
	cfg ChargeAttackConfig
}

// NewChargeAttack makes the goal.
func NewChargeAttack(w World, m Attacker, c ChargeAttackConfig) *ChargeAttack {
	return &ChargeAttack{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x5769460).
func (g *ChargeAttack) CanUse() bool {
	wanted, ok := g.m.MoveWanted()
	if !ok {
		return false
	}
	t := g.actor(g.m.Target())
	if t == nil || wanted || g.cfg.SuccessRate <= g.world.Random().NextFloat() {
		return false
	}
	return dist2(g.m.Pos(), t.Pos()) > g.cfg.MinDistance*g.cfg.MinDistance
}

// CanContinueToUse implements goal.Goal (0x5769640).
func (g *ChargeAttack) CanContinueToUse() bool {
	wanted, ok := g.m.MoveWanted()
	if !ok || !wanted || !g.m.Flag(FlagCharging) {
		return false
	}
	t := g.actor(g.m.Target())
	return t != nil && !t.Removed()
}

// Start implements goal.Goal (0x57697B0).
func (g *ChargeAttack) Start() {
	t := g.actor(g.m.Target())
	if t == nil {
		return
	}
	g.m.SetMoveWanted(t.AttachPos(chargeAim), g.cfg.SpeedMultiplier)
	g.m.SetFlag(FlagCharging, true)
	g.m.PlaySoundAt(g.cfg.SoundEvent, g.m.Pos())
}

// Stop implements goal.Goal (0x5769A60).
func (g *ChargeAttack) Stop() { g.m.SetFlag(FlagCharging, false) }

// Tick implements goal.Goal (0x5769A80).
func (g *ChargeAttack) Tick() {
	t := g.actor(g.m.Target())
	if t == nil {
		return
	}
	if !overlaps(g.m.Box(), t.Box()) {
		if dist2(g.m.Pos(), t.Pos()) < g.cfg.MaxDistance*g.cfg.MaxDistance {
			g.m.SetMoveWanted(t.AttachPos(chargeAim), g.cfg.SpeedMultiplier)
		}
		return
	}
	g.m.Attack(t)
	g.m.SetFlag(FlagCharging, false)
}
