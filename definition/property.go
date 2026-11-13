package definition

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// PropertyType is a property's type.
type PropertyType uint8

// The property types, by BDS number.
const (
	PropertyInt PropertyType = iota
	PropertyFloat
	PropertyBool
	PropertyEnum
)

// Value is a property value, or a Molang result: a number or a string.
// Bools are numbers, 0 for false.
type Value struct {
	Number   float32
	String   string
	IsString bool
}

// Property is one entry of the description's properties.
type Property struct {
	Name string
	Type PropertyType
	// Min and Max are an int or float property's range.
	Min, Max float32
	// Values are an enum's values.
	Values     []string
	ClientSync bool
	// Default is the literal default; DefaultExpr is set instead when the
	// default is a Molang expression, evaluated by the engine.
	Default     Value
	DefaultExpr string
}

func (p *Parser) validPropertyName(name string) bool {
	if p.ValidPropertyName != nil {
		return p.ValidPropertyName(name)
	}
	ns, n, ok := strings.Cut(name, ":")
	return ok && ns != "" && n != ""
}

// parseProperty reads one property the way 0xBC7BA50 does.
func (p *Parser) parseProperty(name string, raw json.RawMessage, warn func(error)) (Property, error) {
	prop := Property{Name: name}
	if kindOf(raw) == kindNull {
		return prop, fmt.Errorf("JSON node is missing")
	}
	if !p.validPropertyName(name) {
		return prop, fmt.Errorf("name must have valid characters and include a namespace")
	}
	var j struct {
		Type       string          `json:"type"`
		Default    json.RawMessage `json:"default"`
		Range      json.RawMessage `json:"range"`
		Values     json.RawMessage `json:"values"`
		ClientSync json.RawMessage `json:"client_sync"`
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return prop, err
	}
	switch j.Type {
	case "int":
		prop.Type = PropertyInt
	case "float":
		prop.Type = PropertyFloat
	case "bool":
		prop.Type = PropertyBool
	case "enum":
		prop.Type = PropertyEnum
	default:
		return prop, fmt.Errorf(`type value must be "int", "float", "bool", or "enum"`)
	}
	if j.Default == nil || kindOf(j.Default) == kindNull {
		return prop, fmt.Errorf("missing a 'default' value")
	}
	if j.ClientSync != nil {
		if err := json.Unmarshal(j.ClientSync, &prop.ClientSync); err != nil {
			return prop, fmt.Errorf("client_sync value must be boolean")
		}
	}
	switch prop.Type {
	case PropertyInt, PropertyFloat:
		if err := p.parseRange(&prop, j.Range); err != nil {
			return prop, err
		}
	case PropertyBool, PropertyEnum:
		if j.Range != nil {
			warn(fmt.Errorf("property %s: %s type was specified but a range field was still provided", name, j.Type))
		}
	}
	if prop.Type == PropertyEnum {
		if err := p.parseValues(&prop, j.Values); err != nil {
			return prop, err
		}
	}
	return prop, p.parseDefault(&prop, j.Default, warn)
}

func (p *Parser) parseRange(prop *Property, raw json.RawMessage) error {
	if raw == nil || kindOf(raw) == kindNull {
		return fmt.Errorf("'range' node is missing")
	}
	var r []json.RawMessage
	if kindOf(raw) != kindArray || json.Unmarshal(raw, &r) != nil {
		return fmt.Errorf("'range' node is invalid")
	}
	if len(r) != 2 {
		return fmt.Errorf("'range' array should have two elements")
	}
	var lo, hi float64
	if kindOf(r[0]) != kindNumber || kindOf(r[1]) != kindNumber || json.Unmarshal(r[0], &lo) != nil || json.Unmarshal(r[1], &hi) != nil {
		return fmt.Errorf("'range' array should contain only number elements")
	}
	if prop.Type == PropertyInt {
		lo, hi = float64(int32(lo)), float64(int32(hi))
	}
	if hi < lo {
		return fmt.Errorf("range max is less than range min")
	}
	prop.Min, prop.Max = float32(lo), float32(hi)
	return nil
}

func (p *Parser) parseValues(prop *Property, raw json.RawMessage) error {
	var items []json.RawMessage
	if raw == nil || kindOf(raw) != kindArray || json.Unmarshal(raw, &items) != nil {
		return fmt.Errorf("missing a 'values' array")
	}
	for _, it := range items {
		var s string
		if kindOf(it) != kindString || json.Unmarshal(it, &s) != nil {
			return fmt.Errorf("this 'enum' property should have string type elements in the 'values' array")
		}
		if len(s) > p.Config.MaxEnumValueLength {
			return fmt.Errorf("value %q has too long of a name (maximum is %d)", s, p.Config.MaxEnumValueLength)
		}
		if !validEnumValue(s) {
			return fmt.Errorf("value %q must start with an alphabetical character and otherwise have alphanumeric or underscore characters", s)
		}
		if slices.Contains(prop.Values, s) {
			return fmt.Errorf("duplicate value %q in 'values' array", s)
		}
		prop.Values = append(prop.Values, s)
	}
	if n := len(prop.Values); n < p.Config.MinEnumValues || n > p.Config.MaxEnumValues {
		return fmt.Errorf("string enum must have more than 1 and no more than %d elements", p.Config.MaxEnumValues)
	}
	return nil
}

func validEnumValue(s string) bool {
	if s == "" || !isLetter(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c != '_' && !isLetter(c) && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// parseDefault reads the default: a literal of the property's type, or a
// string BDS keeps as a Molang expression. A literal number outside an int
// or float range is clamped with a warning.
func (p *Parser) parseDefault(prop *Property, raw json.RawMessage, warn func(error)) error {
	k := kindOf(raw)
	if k == kindString {
		var s string
		_ = json.Unmarshal(raw, &s)
		if prop.Type == PropertyEnum && slices.Contains(prop.Values, s) {
			prop.Default = Value{String: s, IsString: true}
			return nil
		}
		prop.DefaultExpr = s
		return nil
	}
	switch prop.Type {
	case PropertyBool:
		var b bool
		if k != kindBool || json.Unmarshal(raw, &b) != nil {
			return fmt.Errorf("'default' value does not match the specified type 'bool'")
		}
		prop.Default = Value{Number: boolNumber(b)}
	case PropertyInt, PropertyFloat:
		var f float64
		if k != kindNumber || json.Unmarshal(raw, &f) != nil {
			return fmt.Errorf("'default' value does not match the specified type")
		}
		v := float32(f)
		if prop.Type == PropertyInt {
			v = float32(int32(f))
		}
		if v < prop.Min || v > prop.Max {
			warn(fmt.Errorf("property %s: default %v is not between %v and %v, clamping to nearest value", prop.Name, v, prop.Min, prop.Max))
			v = min(max(v, prop.Min), prop.Max)
		}
		prop.Default = Value{Number: v}
	case PropertyEnum:
		return fmt.Errorf("default value not in 'values' array and is not a valid Molang expression")
	}
	return nil
}

func boolNumber(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// convert turns a set_property value into the property's type the way
// 0xBC75200 does: ints truncate, bools are non-zero, enums need one of the
// values exactly. ok is false when BDS logs an error and sets nothing.
func (prop *Property) convert(v Value) (Value, bool) {
	switch prop.Type {
	case PropertyInt:
		if v.IsString {
			return Value{}, false
		}
		return Value{Number: float32(int32(v.Number))}, true
	case PropertyFloat:
		if v.IsString {
			return Value{}, false
		}
		return Value{Number: v.Number}, true
	case PropertyBool:
		if v.IsString {
			return Value{}, false
		}
		return Value{Number: boolNumber(v.Number != 0)}, true
	case PropertyEnum:
		if !v.IsString || !slices.Contains(prop.Values, v.String) {
			return Value{}, false
		}
		return v, true
	}
	return Value{}, false
}
