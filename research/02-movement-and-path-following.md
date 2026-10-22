# Movement control, path shortcuts and the stuck check (BDS 1.26.52)

Clean-room notes in our own words, read in the 1.26.52 Linux build. They
contain no decompiled code, addresses or symbol dumps. This note follows
[01](01-entity-ai-overview.md), section 5.

**Status as of 2026-10-22:** first pass. Nothing has been checked in game yet.

## 1. When each entity ticks

The navigation, goal and move control systems all skip an entity that:
- has the "removed" flag set;
- fails the virtual check that is probably "AI disabled";
- has health 0 or less.

The navigation system then calls the tick of the entity's follower: walk,
climb, fly and so on. (high)

## 2. Path shortcutting (string pulling)

After the "reached node" skip (01, 5.3), and when the current node is a
walkable or door node (two of the node types), the follower tries to **skip
ahead in a straight line**. (high for the shape; medium for the meaning of the
node types)

**Probe size:**
- **Width:** the mob's width rounded up, but at least 2 when it is less than
  3. That value is then multiplied by 1.5, using integer maths.
- **Height:** the mob's height rounded up, plus 1.

**Search:** walk backwards from the furthest candidate node towards the
current one. Each node gets the same centre offset as path following
(half the mob's width rounded, plus 0.5 on door nodes). The first node the mob
can reach in a straight line with that probe size becomes the new current
node.

The result is that mobs walk diagonally across open ground instead of
following the A* grid step by step.

## 3. The stuck check

Each navigation component keeps:
- a tick counter that goes up every navigation tick;
- the tick of the last check;
- the mob's position at the last check;
- a **check interval** in ticks;
- a **minimum move distance**.

(high for the logic; the default values are open)

When more than the check interval has passed since the last check:

1. **Moved too little.** If the mob is less than the minimum distance (as a
   squared distance) from where it was at the last check, and the shortcut
   step did not just succeed, the follower's **stuck handler** runs. In the
   base class that drops the path; the goal then usually repaths.
2. **Record.** The check tick and position are updated.

TODO: the default interval and distance, and whether a JSON component or a goal
sets them.

## 4. Move control: walking (generic)

Each tick, with a wanted position and a speed modifier set by navigation:

1. **No wanted position.** The forward input is set to 0 and nothing else
   happens.
2. **Step adjustment.** The block at the wanted position is read. If its
   collision box top is above its bottom by less than 1 (a slab, carpet or
   similar), the vertical gap to aim for is adjusted by that height. Mobs
   therefore treat a half step as a walkable step, not a jump.
3. **Ground movement** (the mob is walking: the actor flag that means "on
   ground or walking", and not the swim or fly flag):
   - **Close enough:** if the squared distance to the target is **less than
     0.1**, the wanted position is cleared, which means arrived.
   - **Speed:** otherwise the speed is the mob's `movement` attribute × the
     speed modifier.
   - **Turning:** the body yaw turns towards the target. The difference is
     wrapped to −180..180 and clamped to ± the move control's **max turn**
     (from `max_turn` in the movement JSON).
   - **Moving:** the mob moves forward at that speed.
   - **Jumping:** if the target is more than **0.009** blocks higher and the
     horizontal squared distance is less than **max(mob width, 1)**, the jump
     control is told to jump.
4. **Not on the ground** (swimming, flying, or the swim and fly flags):
   - **Arrival:** the mob counts as arrived when the distance is less than
     **the average of the bounding box's three sizes**. The wanted position is
     cleared, and the horizontal velocity is damped (the vertical velocity is
     halved).
   - **Otherwise:** the velocity is pushed towards the target, scaled by the
     speed, and the vertical part gets **0.05 ×** the normalised vertical
     direction.
   - **Facing:** the yaw turns, clamped by the max turn, towards either:
     - the mob's look-at target, if it has one;
     - otherwise the direction of travel.

(high for the numbers 0.1, 0.009, 0.05 and the max(width, 1) check; medium for
which actor flag is which)

## Open questions

- The default stuck interval and distance.
- Which actor flags are numbers 43, 49 and 57 in the 1.26.52 flag list. They
  read as swimming, walking and something that skips the generic step logic.
- The other move controls: hop, glide, dolphin, amphibious, slime, sway, skip
  and vex.
- The order of the AI systems within the level tick.

## BDS test list

- **Slab steps.** A zombie walking up a slab staircase should not jump; up a
  full block it should jump.
- **Turning.** Lure a cow with wheat while you walk in a circle. It should
  turn smoothly at its `max_turn` rate (compare the speed against BDS).
- **Diagonal paths.** In an open field, a villager walking to a far point
  should take a straight diagonal line, not a staircase.
- **Stuck.** Trap a zombie's path with a block placed mid-walk. It should give
  up and repath after a short time; time how long in BDS.
- **Flying arrival.** A bee or parrot reaching its target should slow down and
  stop near it, not overshoot and circle.
