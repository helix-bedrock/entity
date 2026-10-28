// Package definition loads entity definitions (Mojang's minecraft:entity
// files) and runs them the way BDS 1.26.60 does: the stack of component
// groups an actor has added and removed, its events (filters, randomize,
// sequence, first_valid, add, remove, triggers and responses), filter
// groups, and entity properties.
//
// A Definition is the parsed file and is shared by every actor of that type;
// an Instance is one actor's state and is used only from that actor's tick.
// Components stay raw JSON here: other packages (behavior, nav, move) give
// them their meaning, and the engine answers filter tests, runs responses
// other than set_property and evaluates Molang, through Host and Tester.
//
// Read from BDS 1.26.60 (event parse 0xB64EAA0, evaluate 0xB31A220, execute
// 0xB30C5F0, triggers 0xB30FB40, properties 0xBC7BA50, set_property
// 0xD56D800 and 0xBC75200) with names and the group stack and filter code
// from 1.21.3 (ActorDefinitionDiffList, FilterGroup).
package definition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// Config holds the limits BDS checks while loading.
type Config struct {
	// MinEnumValues and MaxEnumValues bound an enum property's values.
	MinEnumValues, MaxEnumValues int
	// MaxEnumValueLength is the longest enum value name.
	MaxEnumValueLength int
}

// DefaultConfig returns BDS 1.26.60's limits (0xBC7BA50, 0xBC7E170,
// 0xBC7DBB0).
func DefaultConfig() Config {
	return Config{MinEnumValues: 2, MaxEnumValues: 16, MaxEnumValueLength: 32}
}

// Parser reads entity files. The zero value accepts every test and response
// name; set the hooks to the engine's lists to drop the same entries BDS
// drops.
type Parser struct {
	Config Config
	// KnownTest reports whether a filter test exists. A filter stops
	// parsing at an unknown test, as BDS's does.
	KnownTest func(name string) bool
	// CheckResponse checks one event response against its schema. An error
	// turns the whole event node off, as BDS's response check (0xB64FA90)
	// does.
	CheckResponse func(r Response) error
	// ValidPropertyName reports whether a property name is allowed. Nil
	// accepts names with a namespace: a ':' with text on both sides.
	ValidPropertyName func(name string) bool
}

// NewParser returns a Parser with DefaultConfig and no hooks.
func NewParser() *Parser {
	return &Parser{Config: DefaultConfig()}
}

// Group is a set of components: component name to its raw JSON.
type Group map[string]json.RawMessage

// Description is the file's description object.
type Description struct {
	Identifier        string
	RuntimeIdentifier string
	SpawnCategory     string
	IsSpawnable       bool
	IsSummonable      bool
	IsExperimental    bool
	// Properties are sorted by name, the order BDS reads them in.
	Properties []Property
}

// Definition is one parsed entity file. It is read-only once parsed and may
// be shared by every actor of the type, on any thread.
type Definition struct {
	FormatVersion string
	Description   Description
	// Components are the components every actor of the type starts with.
	Components Group
	// Groups are the component groups events add and remove.
	Groups map[string]Group
	Events map[string]*Event
}

// Parse reads one entity file. Content problems BDS logs and skips are
// returned as warnings; err is set only when the file is not an entity file.
func (p *Parser) Parse(data []byte) (def *Definition, warnings []error, err error) {
	var file struct {
		FormatVersion string `json:"format_version"`
		Entity        *struct {
			Description     json.RawMessage            `json:"description"`
			ComponentGroups map[string]json.RawMessage `json:"component_groups"`
			Components      json.RawMessage            `json:"components"`
			Events          map[string]json.RawMessage `json:"events"`
		} `json:"minecraft:entity"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, nil, fmt.Errorf("definition: decode: %w", err)
	}
	if file.Entity == nil {
		return nil, nil, fmt.Errorf("definition: no minecraft:entity object")
	}
	e := file.Entity
	def = &Definition{FormatVersion: file.FormatVersion, Groups: map[string]Group{}, Events: map[string]*Event{}}
	warn := func(err error) { warnings = append(warnings, err) }
	if err := p.parseDescription(&def.Description, e.Description, warn); err != nil {
		return nil, warnings, err
	}
	if def.Components, err = parseGroup(e.Components); err != nil {
		warn(fmt.Errorf("definition: components: %w", err))
	}
	for _, name := range sortedKeys(e.ComponentGroups) {
		g, err := parseGroup(e.ComponentGroups[name])
		if err != nil {
			warn(fmt.Errorf("definition: component group %s: %w", name, err))
			continue
		}
		def.Groups[name] = g
	}
	for _, name := range sortedKeys(e.Events) {
		def.Events[name] = p.parseEvent(e.Events[name], func(err error) {
			warn(fmt.Errorf("definition: event %s: %w", name, err))
		})
	}
	return def, warnings, nil
}

func parseGroup(raw json.RawMessage) (Group, error) {
	g := Group{}
	if len(raw) == 0 || kindOf(raw) == kindNull {
		return g, nil
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		return Group{}, err
	}
	return g, nil
}

func (p *Parser) parseDescription(d *Description, raw json.RawMessage, warn func(error)) error {
	var j struct {
		Identifier        string                     `json:"identifier"`
		RuntimeIdentifier string                     `json:"runtime_identifier"`
		SpawnCategory     string                     `json:"spawn_category"`
		IsSpawnable       bool                       `json:"is_spawnable"`
		IsSummonable      bool                       `json:"is_summonable"`
		IsExperimental    bool                       `json:"is_experimental"`
		Properties        map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return fmt.Errorf("definition: description: %w", err)
	}
	if j.Identifier == "" {
		return fmt.Errorf("definition: description has no identifier")
	}
	*d = Description{
		Identifier: j.Identifier, RuntimeIdentifier: j.RuntimeIdentifier, SpawnCategory: j.SpawnCategory,
		IsSpawnable: j.IsSpawnable, IsSummonable: j.IsSummonable, IsExperimental: j.IsExperimental,
	}
	for _, name := range sortedKeys(j.Properties) {
		prop, err := p.parseProperty(name, j.Properties[name], warn)
		if err != nil {
			warn(fmt.Errorf("definition: property %s: %w", name, err))
			continue
		}
		d.Properties = append(d.Properties, prop)
	}
	return nil
}

// jsonKind is the type of a raw JSON value.
type jsonKind uint8

const (
	kindNull jsonKind = iota
	kindObject
	kindArray
	kindString
	kindBool
	kindNumber
)

func kindOf(raw json.RawMessage) jsonKind {
	b := bytes.TrimSpace(raw)
	if len(b) == 0 {
		return kindNull
	}
	switch b[0] {
	case '{':
		return kindObject
	case '[':
		return kindArray
	case '"':
		return kindString
	case 't', 'f':
		return kindBool
	case 'n':
		return kindNull
	}
	return kindNumber
}

// sortedKeys returns m's keys in byte order, the order BDS's JSON objects
// keep their members in.
func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
