# entity

The AI and pathfinding engine for Helix entities: entity definitions loaded
from Mojang's JSON files, components, behaviours (AI goals), sensors and
targeting, navigation, pathfinding, movement and spawn rules. It is a one-to-one
copy of how the Bedrock Dedicated Server (BDS) runs mobs.

**Status as of 2026-10-23:** first code. `goal` runs a mob's goals the way
the BDS goal selector does; `path` is the BDS 1.26.52 pathfinder for walking
and swimming mobs (A* search, node classification and costs). Flying mobs,
navigation, movement, the goals themselves and entity definitions are not
built yet. Research notes go in [`research/`](research/).

Part of [Helix](https://github.com/helix-bedrock), an open-source Minecraft: Bedrock
Edition server engine in Go.

## License

MIT. See [LICENSE](LICENSE).
