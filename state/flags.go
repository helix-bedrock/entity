package state

import "github.com/helix-bedrock/entity/behavior"

// FlagComponents maps each flag component to the actor flag it raises
// while the mob has it. The numbers are BDS's actor flags as the protocol
// repo lists them; which flag each component sets was not read from BDS's
// component code (is_charged is taken as the powered flag a charged
// creeper shows).
func FlagComponents() map[string]int {
	return map[string]int{
		"minecraft:is_baby":            behavior.FlagBaby,
		"minecraft:is_tamed":           behavior.FlagTamed,
		"minecraft:is_saddled":         FlagSaddled,
		"minecraft:is_chested":         FlagChested,
		"minecraft:is_sheared":         FlagSheared,
		"minecraft:is_stunned":         behavior.FlagStunned,
		"minecraft:is_pregnant":        FlagPregnant,
		"minecraft:is_illager_captain": FlagCaptain,
		"minecraft:is_shaking":         FlagShaking,
		"minecraft:is_ignited":         behavior.FlagIgnited,
		"minecraft:is_charged":         FlagPowered,
		"minecraft:is_stackable":       FlagStackable,
	}
}

// SetFlags raises the flag of each flag component the mob gained and
// lowers the flag of each one it lost, as adding and removing a flag
// component does. Flags of components it kept are left alone, so a flag
// another component changed (ageable's baby flag) stays.
func SetFlags(m behavior.Mob, flags map[string]int, had, has func(name string) bool) {
	for name, f := range flags {
		if h := has(name); h != had(name) {
			m.SetFlag(f, h)
		}
	}
}
