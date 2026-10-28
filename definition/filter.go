package definition

import (
	"encoding/json"
	"fmt"
	"slices"
)

// Subject is who a filter test or a trigger looks at (BDS's FilterSubject).
type Subject uint8

// The subjects, by BDS number.
const (
	SubjectSelf Subject = iota
	SubjectOther
	SubjectPlayer
	SubjectTarget
	SubjectParent
	SubjectBaby
	SubjectBlock
	SubjectDamager
	SubjectHolder
	SubjectItem
)

// subjectNames returns the subjects' JSON names in BDS order.
func subjectNames() []string {
	return []string{"self", "other", "player", "target", "parent", "baby", "block", "damager", "holder", "item"}
}

// String returns the subject's JSON name.
func (s Subject) String() string {
	if names := subjectNames(); int(s) < len(names) {
		return names[s]
	}
	return fmt.Sprintf("subject(%d)", s)
}

func parseSubject(name string) (Subject, bool) {
	i := slices.Index(subjectNames(), name)
	if i < 0 {
		return SubjectSelf, false
	}
	return Subject(i), true
}

// Operator is a filter test's comparison (BDS's FilterOperator).
type Operator uint8

// The operators, by BDS number.
const (
	OpEquals Operator = iota
	OpNotEquals
	OpGreater
	OpLess
	OpGreaterOrEqual
	OpLessOrEqual
)

// parseOperator reads the preset strings of BDS's operator parameter.
func parseOperator(s string) (Operator, bool) {
	switch s {
	case "=", "==", "equals":
		return OpEquals, true
	case "not", "!=", "<>":
		return OpNotEquals, true
	case ">":
		return OpGreater, true
	case "<":
		return OpLess, true
	case ">=":
		return OpGreaterOrEqual, true
	case "<=":
		return OpLessOrEqual, true
	}
	return OpEquals, false
}

// Compare applies the operator to a test's value a and the JSON value b, as
// FilterTest::_testValuesWithOperator does for numbers. An unknown operator
// is false.
func Compare[T int32 | float32](op Operator, a, b T) bool {
	switch op {
	case OpEquals:
		return a == b
	case OpNotEquals:
		return a != b
	case OpGreater:
		return a > b
	case OpLess:
		return a < b
	case OpGreaterOrEqual:
		return a >= b
	case OpLessOrEqual:
		return a <= b
	}
	return false
}

// CompareBool is Compare for bools, ordering false before true.
func CompareBool(op Operator, a, b bool) bool {
	switch op {
	case OpEquals:
		return a == b
	case OpNotEquals:
		return a != b
	case OpGreater:
		return a && !b
	case OpLess:
		return !a && b
	case OpGreaterOrEqual:
		return a || !b
	case OpLessOrEqual:
		return !a || b
	}
	return false
}

// Collection is how a filter group joins its members.
type Collection uint8

// The collections, by BDS number.
const (
	// All passes when every member passes (all_of, all, AND).
	All Collection = iota
	// Any passes when a member passes (any_of, any, OR).
	Any
	// None passes when no member passes (none_of, NOT).
	None
)

// Test is one filter test as written in the JSON. Its meaning is the
// engine's: entity only parses it and joins the results.
type Test struct {
	// Name is the "test" member, such as is_family.
	Name     string
	Subject  Subject
	Operator Operator
	// Domain and Value are the raw JSON members, nil when absent; the test
	// applies its own defaults.
	Domain, Value json.RawMessage
}

// Tester runs filter tests for one event or check: it knows the actors and
// block each subject names. The engine implements it.
type Tester interface {
	// Test runs t. resolved is false when t's subject is missing, which
	// fails the test whatever the collection.
	Test(t *Test) (pass, resolved bool)
}

// Filter is a filter group (BDS's FilterGroup): its subgroups, then its
// tests. A nil Filter passes.
type Filter struct {
	Collection Collection
	Groups     []*Filter
	Tests      []Test
}

// Eval runs the filter the way FilterGroup::evaluate does: subgroups first,
// then tests, stopping at the first member that decides the result.
func (f *Filter) Eval(t Tester) bool {
	if f == nil {
		return true
	}
	switch f.Collection {
	case All:
		for _, g := range f.Groups {
			if !g.Eval(t) {
				return false
			}
		}
		for i := range f.Tests {
			if pass, ok := t.Test(&f.Tests[i]); !ok || !pass {
				return false
			}
		}
		return true
	case Any:
		for _, g := range f.Groups {
			if g.Eval(t) {
				return true
			}
		}
		for i := range f.Tests {
			if pass, ok := t.Test(&f.Tests[i]); ok && pass {
				return true
			}
		}
		return false
	case None:
		for _, g := range f.Groups {
			if g.Eval(t) {
				return false
			}
		}
		for i := range f.Tests {
			if pass, ok := t.Test(&f.Tests[i]); ok && pass {
				return false
			}
		}
	}
	return true
}

// ParseFilter reads a filter group as found in a component, such as a
// goal's "filters" member. The group read before an error is returned with
// it.
func (p *Parser) ParseFilter(raw json.RawMessage) (*Filter, error) { return p.parseFilter(raw) }

// parseFilter reads a "filters" member into a new All group. BDS stops at
// the first member that fails to parse and keeps what it read before it, so
// the group is returned with the error.
func (p *Parser) parseFilter(raw json.RawMessage) (*Filter, error) {
	f := &Filter{Collection: All}
	err := p.parseObject(f, raw)
	return f, err
}

// parseObject is FilterGroup::_parseObject.
func (p *Parser) parseObject(f *Filter, raw json.RawMessage) error {
	switch kindOf(raw) {
	case kindArray:
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return fmt.Errorf("filter: %w", err)
		}
		for _, it := range items {
			if err := p.parseObject(f, it); err != nil {
				return err
			}
		}
		return nil
	case kindObject:
	case kindNull:
		return fmt.Errorf("filter: null member")
	default:
		// Strings, numbers and bools add nothing and do not fail.
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fmt.Errorf("filter: %w", err)
	}
	if _, ok := obj["test"]; ok {
		return p.parseTest(f, obj)
	}
	if len(obj) > 1 && f.Collection == Any {
		// Several members inside an any group are joined with all.
		g := &Filter{Collection: All}
		err := p.parseObject(g, raw)
		if err == nil {
			f.Groups = append(f.Groups, g)
		}
		return err
	}
	for _, k := range sortedKeys(obj) {
		if err := p.parseMember(f, k, obj[k]); err != nil {
			return err
		}
	}
	return nil
}

// parseMember is FilterGroup::_parseMember.
func (p *Parser) parseMember(f *Filter, key string, raw json.RawMessage) error {
	if kindOf(raw) == kindNull {
		return fmt.Errorf("filter: member %q is null", key)
	}
	var c Collection
	switch key {
	case "all_of", "all", "AND":
		c = All
	case "any_of", "any", "OR":
		c = Any
	case "none_of", "NOT":
		c = None
	default:
		// BDS hands other members to ActorFilterGroup's legacy filters,
		// which entity does not have.
		return fmt.Errorf("filter: unknown member %q", key)
	}
	if f.Collection == c {
		return p.parseObject(f, raw)
	}
	g := &Filter{Collection: c}
	if err := p.parseObject(g, raw); err != nil {
		return err
	}
	f.Groups = append(f.Groups, g)
	return nil
}

// parseTest is FilterGroup::_parseTest with the inputs every test shares.
func (p *Parser) parseTest(f *Filter, obj map[string]json.RawMessage) error {
	var name string
	if json.Unmarshal(obj["test"], &name) != nil {
		return fmt.Errorf("filter: member 'test' is not a string")
	}
	if p.KnownTest != nil && !p.KnownTest(name) {
		return fmt.Errorf("filter: test not found: %s", name)
	}
	t := Test{Name: name, Domain: obj["domain"], Value: obj["value"]}
	if raw, ok := obj["subject"]; ok {
		var known bool
		var s string
		var n int
		switch {
		case json.Unmarshal(raw, &s) == nil:
			t.Subject, known = parseSubject(s)
		case json.Unmarshal(raw, &n) == nil:
			// Files written back out of BDS (such as helix-data's) hold the
			// enum number.
			t.Subject, known = Subject(n), n >= 0 && n < len(subjectNames())
		default:
			return fmt.Errorf("filter: test %s: subject is not a string", name)
		}
		if !known {
			return fmt.Errorf("filter: test %s: unknown subject %s", name, raw)
		}
	}
	if raw, ok := obj["operator"]; ok {
		var known bool
		var s string
		var n int
		switch {
		case json.Unmarshal(raw, &s) == nil:
			t.Operator, known = parseOperator(s)
		case json.Unmarshal(raw, &n) == nil:
			t.Operator, known = Operator(n), n >= 0 && n <= int(OpLessOrEqual)
		default:
			return fmt.Errorf("filter: test %s: operator is not a string", name)
		}
		if !known {
			return fmt.Errorf("filter: test %s: unknown operator %s", name, raw)
		}
	}
	f.Tests = append(f.Tests, t)
	return nil
}
