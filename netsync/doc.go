// Package netsync sends an entity's packets the way BDS 1.26.60 does:
// movement deltas, the distance throttle, rotation rounding, spawn and
// despawn packets, equipment, data, events and the order of a tick's
// packets.
//
// netsync never decides anything for the engine. The engine decides who
// sees which entity, when an entity moved, when a sound plays and who hears
// it. netsync gives it three layers to pick from:
//
//   - The BDS rules as plain functions: [RotationByte], [Significant],
//     [DeltaHeader], [Score], [VanillaView] and the rest.
//   - Packet builders that fill every field the way BDS does: [AddActor],
//     [Delta], [Absolute], [ArmourEquipment], [HandEquipment] and the rest.
//   - [Sync], an optional helper that keeps the last-sent state per entity
//     and per viewer and writes one batch per viewer per tick. Every
//     vanilla behaviour in it is an [Options] field: [VanillaOptions] turns
//     them all on, the zero Options turns them all off.
//
// netsync knows no block or item names. Items arrive as finished
// protocol.ItemInstance values from the engine.
//
// Research: BDS 1.26.60 Linux, decompiled and run under gdb with a real
// client joined. The function addresses are cited where each rule is used.
package netsync
