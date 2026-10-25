package spawn

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// filter is a biome filter, tested once per biome when the spawner is built.
type filter struct {
	// kind is "" (no filter: every biome), "all_of", "any_of", "none_of",
	// "has_biome_tag" or "is_snow_covered".
	kind   string
	negate bool
	value  string
	sub    []filter
}

func parseFilter(raw json.RawMessage) (filter, error) {
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return filter{}, err
		}
		f := filter{kind: "all_of"}
		for _, it := range items {
			s, err := parseFilter(it)
			if err != nil {
				return filter{}, err
			}
			f.sub = append(f.sub, s)
		}
		return f, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return filter{}, err
	}
	for _, k := range []string{"all_of", "any_of", "none_of"} {
		if v, ok := obj[k]; ok {
			f, err := parseFilter(v)
			if err != nil {
				return filter{}, err
			}
			f.kind = k
			return f, nil
		}
	}
	var t struct {
		Test     string `json:"test"`
		Operator string `json:"operator"`
		Subject  string `json:"subject"`
		Value    any    `json:"value"`
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return filter{}, err
	}
	f := filter{kind: t.Test}
	switch t.Operator {
	case "", "==", "equals":
	case "!=", "not", "<>":
		f.negate = true
	default:
		return filter{}, fmt.Errorf("operator %q", t.Operator)
	}
	switch v := t.Value.(type) {
	case string:
		f.value = v
	case bool:
		// is_snow_covered takes a bool value, default true.
		if !v {
			f.negate = !f.negate
		}
	}
	switch f.kind {
	case "has_biome_tag", "is_snow_covered":
	default:
		return filter{}, fmt.Errorf("test %q", f.kind)
	}
	return f, nil
}

// BiomeInfo is what biome filters read about a biome.
type BiomeInfo struct {
	Tags []string
	// SnowCovered is the biome's snow cover (its minimum snow
	// accumulation is above 0), which is_snow_covered tests.
	SnowCovered bool
}

func (f *filter) test(b *BiomeInfo) bool {
	var ok bool
	switch f.kind {
	case "":
		return true
	case "all_of":
		ok = true
		for i := range f.sub {
			if !f.sub[i].test(b) {
				ok = false
				break
			}
		}
		return ok
	case "any_of":
		for i := range f.sub {
			if f.sub[i].test(b) {
				return true
			}
		}
		return false
	case "none_of":
		for i := range f.sub {
			if f.sub[i].test(b) {
				return false
			}
		}
		return true
	case "has_biome_tag":
		ok = slices.Contains(b.Tags, f.value)
	case "is_snow_covered":
		ok = b.SnowCovered
	}
	return ok != f.negate
}

type rawBiome struct {
	Biome struct {
		Description struct {
			Identifier string `json:"identifier"`
		} `json:"description"`
		Components struct {
			Climate *struct {
				Snow []float32 `json:"snow_accumulation"`
			} `json:"minecraft:climate"`
			Tags *struct {
				Tags []string `json:"tags"`
			} `json:"minecraft:tags"`
		} `json:"components"`
	} `json:"minecraft:biome"`
}

// LoadBiomes reads every *.biome.json under dir in fsys (helix-data's
// vanilla/biomes). The result is keyed by name without "minecraft:".
func LoadBiomes(fsys fs.FS, dir string) (map[string]BiomeInfo, error) {
	files, err := fs.Glob(fsys, path.Join(dir, "*.biome.json"))
	if err != nil {
		return nil, fmt.Errorf("spawn: list biomes: %w", err)
	}
	out := make(map[string]BiomeInfo, len(files))
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, fmt.Errorf("spawn: read %s: %w", f, err)
		}
		var raw rawBiome
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("spawn: decode %s: %w", f, err)
		}
		name := strings.TrimPrefix(raw.Biome.Description.Identifier, "minecraft:")
		if name == "" {
			name = strings.TrimSuffix(path.Base(f), ".biome.json")
		}
		var b BiomeInfo
		if t := raw.Biome.Components.Tags; t != nil {
			b.Tags = t.Tags
		}
		if c := raw.Biome.Components.Climate; c != nil && len(c.Snow) > 0 {
			b.SnowCovered = c.Snow[0] > 0
		}
		out[name] = b
	}
	return out, nil
}
