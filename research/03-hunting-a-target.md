# How a mob finds and walks to the nearest attackable entity (BDS 1.26.52)

Clean-room notes in our own words, read in the 1.26.52 Linux build. They
contain no decompiled code, addresses or symbol dumps.

**Status as of 2026-10-22:** first pass. Nothing has been checked in game yet.
Where a value comes from a JSON field, the field name is given. Fields
marked "(field?)" are our guess at which documented field the stored value
is; check them against the vanilla JSON.

Two goals work together:

1. `minecraft:behavior.nearest_attackable_target` is a **target goal**. It
   only chooses who to attack and stores that as the mob's target.
2. `minecraft:behavior.melee_attack` is a **normal goal**. It reads the
   target, walks to it with navigation and hits it when it is in reach.

Target goals run before normal goals in the same tick (see
[01](01-entity-ai-overview.md), section 4). A target chosen this tick can
therefore start the melee goal in the same tick.

The pipeline:

```
nearest_attackable_target         melee_attack                navigation / move control
  every scan_interval ticks:        each tick:                   (notes 01 and 02)
  candidates sorted nearest  --->   look at target
  first one that passes all         count down the repath timer
  checks becomes the target         on zero: moveTo(target) ---> A* path, follow, walk
                                    in reach + visible: attack
```

## Part 1: choosing the target (nearest_attackable_target)

### 1.1 When it searches (the goal's "can use")

1. If the mob has a certain component flag set, the goal never starts. This
   is probably "is tamed" or "sitting" or similar; it is open.
2. **Random interval** (field?: `attack_interval` or `random_interval`). If
   it is above 0, the goal searches only when a random integer in
   [0, interval) comes out as 0. On average that is once per interval ticks.
3. **Otherwise, the scan interval** (`scan_interval`). The goal searches only
   on ticks where (the mob's runtime ID + the level tick) is divisible by the
   scan interval. Mobs are therefore spread across ticks instead of all
   scanning on the same one.
4. **Acquisition probability** (field?, a value below 1). If it is below 1, a
   random float must come out under it, or the search is skipped.

(high for the logic)

### 1.2 The search

1. **Radius.** The search radius is the largest `max_dist` over all the
   `entity_types` entries.
2. **Box.** The query box is the mob's bounding box grown by the radius on X
   and Z. On Y it is grown by `within_radius` if that is set (0 or more),
   otherwise also by the radius. (medium: the Y/radius pairing needs
   confirming)
3. **Candidates.** The engine collects up to 256 actors in the box and **sorts
   them by squared distance, nearest first**. (high)
4. **First match wins.** The candidates are walked in order, and the first one
   that passes every check below becomes the target. Because the list is
   sorted, that is the nearest valid one. (high)

### 1.3 Checks per candidate, in order

**A. Basic validity.** Any failure rejects the candidate. (high)
- It is not null and not the mob itself.
- Neither the mob nor the candidate is being removed.
- The candidate has no "cannot be targeted" bit.
- The candidate is alive.
- The candidate is in a dimension.
- **Owner:** if the candidate is the mob's owner, it is rejected unless
  `attack_owner` is true.
- **"Can attack":** the general rule must pass (families, peaceful rules and
  so on). TODO: read it.
- **Teams:** if the mob is on a team and the candidate is on the same team,
  it is rejected.
- **Players:** a player who is not being force-allowed is rejected if they
  have the invulnerable ability (creative mode), or if the level says players
  cannot be targeted.

**B. Matching an `entity_types` entry.** For each entry in the list:
- **Distance:** the entry is skipped if the candidate is further than that
  entry's `max_dist`.
- **Sight:** if the entry has `must_see`, the mob must see the candidate,
  using its sensing component's line of sight. That is skipped when the
  candidate is already the current target and has been unseen for less than
  `must_see_forget_duration`.
- **Filters:** the entry's `filters` are evaluated, with the candidate as
  "other".
- **Choosing an entry:** among the entries that match, the one with the
  largest `max_dist` is used. An entry flag can override `must_see` and its
  forget time for this target.

If nothing matches, the candidate is rejected. (high for the order; medium for
the override flag)

**C. Reachability** (`must_reach`). The result is cached:
- **Re-check timer:** the check is redone only when a counter runs out. The
  counter is reset to **10 + random(0..4)** each time a check is done.
- **The check:** the mob builds a path to the candidate. It passes when the
  path's last node is within **squared horizontal distance less than 3** of
  the candidate's block.
- The cached answer is "reachable" or "unreachable".

(high)

**D. Health.** The candidate must have a health component. (medium)

**E. Cooldown.** Each `entity_types` entry can have a cooldown. A target that
was picked recently is skipped until the level tick reaches the time stored
for it. (medium)

**F. Distance with visibility multipliers.** (high)
- The effective range starts as the matched entry's `max_dist`, with a
  multiplier of 1.
- **Sneaking:** the multiplier is `target_sneak_visibility_multiplier`.
- **Invisibility:** the multiplier is multiplied by:
  - max(0.1, the fraction of the target's armour slots that are filled);
  - × `target_invisible_multiplier`.

  This is skipped if the mob has a certain component; it is open which one.
- **Mob heads:** if the target is a player wearing this mob's head (a zombie
  head for a zombie and so on), the range is **halved**.
- **Range check:** the candidate is rejected if
  distance² > (range × multiplier)².
- **Height check:** it is also rejected if `target_search_height` is 0 or more
  and |dy| > `target_search_height` × multiplier.

**G. Persistence.** If `set_persistent` is true, the mob is made persistent so
that it no longer despawns.

The winner becomes the mob's target. The goal also remembers the pick time for
the cooldown.

### 1.4 Keeping the target (the goal's "can continue")

The base target goal keeps the target while it stays valid. Its checks include
being alive, the same team rules and the forget timers.

TODO: read the base target goal's "can continue" for the exact conditions:
- `persist_time`;
- `reselect_targets`, which picks a closer target while hunting;
- the must_see forget timer.

## Part 2: walking to it and hitting it (melee_attack)

### 2.1 Starting (the goal's "can use")

- **Blocked:** never starts while the same component flag as in 1.1 is set.
- **Target:** needs a navigation component and a live target.
- **`attack_types`:** if it is set, the target's actor type must match it.
- **New target:** it remembers the target, and registers to hear when the
  target is removed.
- **Path:** if the mob is not riding, it asks navigation for a path to the
  target. When `require_complete_path` is set, the path must reach the target,
  not stop part way.
- **Riding:** when the mob is riding, it can start without a path. (medium)

### 2.2 Start

- If the mob is riding and the vehicle should do the moving, the mount at the
  bottom of the stack does the navigation. The stack is followed down for at
  most 1023 levels.
- The prepared path is given to navigation with `speed_multiplier`.
- The repath timer is set to the starting delay.

(high)

### 2.3 Each tick: the core chase loop

1. **Attack cooldown.** The attack cooldown counts down by 1.
2. **Look.** The mob looks at the target, unless the look control is already
   busy. It aims at the target's head height for players and similar, and at
   the middle of the bounding box for others, with the goal's max look
   rotation (x and y).
3. **Repath timer.** The repath timer counts down by 1.
4. **Seeing the target.** Without `track_target`, the rest of this step only
   runs while the mob can see the target (sensing line of sight). With
   `track_target`, it always runs, so the mob knows where the target is even
   through walls.
5. **Remember position.** The target's current position (x, feet y, z) is
   remembered.
6. **Wait.** While the repath timer is above 0, nothing more happens this
   tick; the mob keeps following its current path.
7. **Repath decision**, when the timer reaches 0:
   - **Path still points at the target:** if the current path's last node is
     in the same block as the target, the path is reused, except for a **5%
     random chance** of repathing anyway.
   - **Path is stale but the mob is already in attack range:** no repath.
8. **New timer.** The timer is set to
   `min_path_time` + random(0 .. `max_path_time` − `min_path_time`). Then:
   - if distance² is more than `path_outer_boundary`², it adds
     `outer_boundary_time_increase`;
   - otherwise, if distance² is more than `path_inner_boundary`², it adds
     `inner_boundary_time_increase`.

   Far targets are therefore repathed less often. (high for the shape; the
   field names are our mapping of the stored values)
9. **Move.** It calls navigation "move to entity" with `speed_multiplier`. If
   that fails (no path), the timer also gets `path_fail_time_increase` added.
10. **Attack check.** See 2.4.

### 2.4 Reach and the attack

**Reach.** The squared reach is
((`reach_multiplier` × w) − w + w2)² + the target's width, where:
- w is max(0.5, the mob's width);
- w2 is the same w, except when the mob is riding: then it is
  max(min(the vehicle's width, about 1.39), w) + 0.1.

The goal's in-range test compares this with the distance. (high for the
formula; the 1.39 constant is read from the build)

**Attack**, when the target is in reach and visible:
1. **Angle.** The target must also be inside the mob's attack angle, using
   the goal's horizontal angle setting.
2. **Cooldown.** The attack cooldown is reset (`cooldown_time`).
3. **Swing.** The mob swings its arm.
4. **Speed.** The movement speed modifier may be refreshed.
5. **Hit.** The engine's "attack target" call runs.
6. **On a hit:**
   - **Fire:** if the mob is on fire and a random float is under a
     per-difficulty chance, the target is set on fire for 2 × that value
     seconds. This is zombie behaviour, enabled per goal.
   - **Triggers:** the goal's `on_attack` event fires; if the target died,
     `on_kill` fires.
   - **One attack only:** if `attack_once` is set, the goal stops after
     hitting.

(high for the order; medium for which JSON flag turns on the fire part)

### 2.5 Continuing (the goal's "can continue")

The goal stops when any of these happens:
- the target changes or dies;
- the difficulty becomes peaceful, for targets of a hostile kind;
- `random_stop_interval` (when above 0) comes up with a random integer of 0;
- without `track_target`, the path is finished or gone. With `track_target`,
  the goal keeps going even with no path.

When `attack_once` is set and the hit happened, the goal stops at once.

(high)

## How this maps to Helix

- **Reuse.** Both goals sit on shared pieces:
  - the actor query, sorted by distance;
  - sensing (line of sight, with a per-tick cache);
  - filters (01, section 2.5);
  - navigation "move to entity";
  - the look control.
- **Order to build:** filters, then the sorted actor query, then sensing, then
  the target goal, then melee.
- **Repeatable random draws.** Every random draw above goes through the
  level's random source, as in BDS: the interval check, the 5% repath, the
  10 + random(0..4) reach timer and the path time. For the same results with
  1 core or many, each mob needs its own random stream, or the order of the
  draws must be fixed.

## Open questions

- Which component flag disables both goals.
- The base target goal's "can continue": `persist_time`, `reselect_targets`
  and the forget timer.
- The exact JSON field for each stored value, and the defaults. Check against
  the vanilla zombie, husk and spider files.
- The "can attack" rule used in check A.
- Sensing: its line-of-sight ray and its per-tick cache.

## BDS test list

- **Nearest wins.** Two villagers at 10 and 20 blocks from a zombie: it should
  pick the one at 10.
- **Sneaking.** A zombie at the edge of its follow range should lose you when
  you sneak (range × the sneak multiplier).
- **Invisibility.** Fully armoured and invisible, then with no armour and
  invisible: compare the distance at which you are noticed.
- **Mob head.** Wearing a zombie head, a zombie should notice you only at
  about half the distance.
- **Unreachable target.** A villager on a pillar the zombie cannot path to:
  with `must_reach`, the zombie should ignore it.
- **Repath rate.** A far target means less frequent path changes. Watch a
  zombie chasing a player running in circles at 5 blocks and at 30 blocks.
- **Through walls.** `track_target` mobs keep following through walls; other
  mobs stop when they lose sight.
- **Fire.** A burning zombie hitting a player sets them on fire on hard
  difficulty more often than on easy.
- **Creative.** Players in creative are never targeted.
