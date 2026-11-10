package validation

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// tagKey is the struct tag holding the rules.
const tagKey = "validate"

// level holds the rules for one level of a field: the field itself, or, after
// each dive, the elements of the level before.
type level struct {
	// required reports blank for the zero value, and nothing else: whatever
	// else an empty value breaks only says again that it is missing.
	required bool

	// omitEmpty lets the zero value past the rules of the level.
	omitEmpty bool

	rules []rule
}

// rule is one rule of a validate tag, such as max=50.
type rule struct {
	name string

	// param is what follows the "=", or "" for a rule written without one.
	param string
}

// String spells r as the tag writes it.
func (r rule) String() string {
	if r.param == "" {
		return r.name
	}
	return r.name + "=" + r.param
}

// parseTag reads the validate tag in tag into its levels: the field itself
// first, then one level per dive. It returns nil when there is no validate tag.
// It checks the syntax only; whether a rule exists and fits the field is for
// the caller.
//
// Rules are separated by commas, except that pattern takes everything after
// "pattern=" as its regular expression, which may well hold a comma, as in
// \d{1,3}. That makes pattern the last rule of a tag.
func parseTag(tag reflect.StructTag) ([]level, error) {
	s, ok, err := lookup(tag)
	if !ok || err != nil {
		return nil, err
	}

	levels := []level{{}}
	for more := s != ""; more; {
		var item string
		if strings.HasPrefix(s, "pattern=") {
			item, more = s, false
		} else {
			item, s, more = strings.Cut(s, ",")
		}
		if levels, err = add(levels, item); err != nil {
			return nil, err
		}
	}

	for _, l := range levels {
		if l.required && l.omitEmpty {
			return nil, errors.New("omitempty lets through the empty value required rejects; keep one of them")
		}
	}
	if last := levels[len(levels)-1]; len(levels) > 1 && !last.required && len(last.rules) == 0 {
		return nil, errors.New("dive must be followed by the rules for the elements")
	}
	return levels, nil
}

// ruleAfterPattern returns the part of the regular expression of a pattern= in
// tag that starts like another rule, a comma and the name of one, which the
// expression takes in all the same.
func ruleAfterPattern(tag reflect.StructTag) (string, bool) {
	s, ok, err := lookup(tag)
	if !ok || err != nil {
		return "", false
	}
	i := strings.Index(","+s, ",pattern=")
	if i < 0 {
		return "", false
	}
	re := s[i+len("pattern="):]
	for j := range len(re) {
		if re[j] != ',' {
			continue
		}
		name, _, _ := strings.Cut(re[j+1:], ",")
		name, _, _ = strings.Cut(name, "=")
		if _, ok := resolvers[name]; ok || name == "required" || name == "omitempty" || name == "dive" {
			return re[j:], true
		}
	}
	return "", false
}

// lookup returns the validate tag in tag and whether there is one.
//
// A validate key that reflect cannot read is an error, rather than a tag to
// skip: without the check, a field whose tag says `validate:"pattern=^\d+$"`,
// `validate: "required"`, or `Validate:"required"` would quietly go unchecked.
// So is a second validate key, which reflect never reads.
func lookup(tag reflect.StructTag) (string, bool, error) {
	s, ok := tag.Lookup(tagKey)
	keys := validateKeys(tag)
	switch {
	case len(keys) > 1:
		return "", true, &ruleError{
			msg:  "the struct tag has a validate key twice, and only the first is read",
			hint: "write the rules in one validate tag, separated by commas",
		}
	case ok || len(keys) == 0:
		return s, ok, nil
	}

	switch k := keys[0]; {
	case k.name != tagKey:
		return "", true, &ruleError{
			msg:  fmt.Sprintf("the struct tag key %s is not read, as keys are case-sensitive", k.name),
			hint: "write the key as validate",
		}
	case k.spaced:
		return "", true, &ruleError{
			msg:  "the validate tag has a space around its colon, so it is not read",
			hint: `write it as validate:"…", without spaces`,
		}
	case !k.quoted && strings.Contains(k.value, `\`):
		return "", true, errors.New(`the validate tag is not a valid quoted string; write a backslash as \\`)
	case !k.quoted:
		return "", true, errors.New("the validate tag is not a valid quoted string")
	}
	return "", true, &ruleError{
		msg:  "the struct tag is malformed before the validate key, so it is not read",
		hint: "go vet reports where the tag goes wrong",
	}
}

// tagKeyAt is a key of a struct tag, read more leniently than reflect reads
// one.
type tagKeyAt struct {
	name string

	// spaced says there is a space before or after the colon.
	spaced bool

	// value is the quoted value, and quoted says it unquotes.
	value  string
	quoted bool
}

// validateKeys returns the keys of tag that spell validate in any case. It
// reads tag as reflect does, but takes spaces around a colon, and goes on past
// a value that does not unquote, so that it finds the keys reflect does not.
func validateKeys(tag reflect.StructTag) []tagKeyAt {
	var keys []tagKeyAt
	s := string(tag)
	for {
		s = strings.TrimLeft(s, " ")
		i := 0
		for i < len(s) && s[i] > ' ' && s[i] != ':' && s[i] != '"' && s[i] != 0x7f {
			i++
		}
		k := tagKeyAt{name: s[:i]}
		rest := strings.TrimLeft(s[i:], " ")
		k.spaced = len(rest) != len(s[i:])
		rest, colon := strings.CutPrefix(rest, ":")
		after := strings.TrimLeft(rest, " ")
		k.spaced = k.spaced || len(after) != len(rest)
		if i == 0 || !colon || !strings.HasPrefix(after, `"`) {
			return keys
		}

		j := 1
		for j < len(after) && after[j] != '"' {
			if after[j] == '\\' {
				j++
			}
			j++
		}
		k.value = after[:min(j+1, len(after))]
		_, err := strconv.Unquote(k.value)
		k.quoted = err == nil
		if strings.EqualFold(k.name, tagKey) {
			keys = append(keys, k)
		}
		if j >= len(after) {
			return keys
		}
		s = after[j+1:]
	}
}

// add adds item, one comma-separated part of a tag, to the last of levels.
func add(levels []level, item string) ([]level, error) {
	name, param, hasParam := strings.Cut(item, "=")
	l := &levels[len(levels)-1]

	switch {
	case item == "":
		return nil, errors.New("empty rule; check for a stray comma")
	case strings.ContainsAny(name, " \t"):
		return nil, fmt.Errorf("rule %q contains a space; rules are separated by commas alone", item)
	case name == "":
		return nil, fmt.Errorf("rule %q has no name", item)
	case hasParam && param == "":
		return nil, fmt.Errorf("rule %q has no value after =", item)
	case name == "dive", name == "required", name == "omitempty":
		if hasParam {
			return nil, fmt.Errorf("%s takes no value", name)
		}
	}

	switch name {
	case "dive":
		return append(levels, level{}), nil
	case "required":
		if l.required {
			return nil, errors.New("required is given twice")
		}
		l.required = true
	case "omitempty":
		if l.omitEmpty {
			return nil, errors.New("omitempty is given twice")
		}
		if len(l.rules) > 0 {
			return nil, fmt.Errorf("omitempty must come before %s, which it lets an empty value through", l.rules[0].name)
		}
		l.omitEmpty = true
	default:
		l.rules = append(l.rules, rule{name: name, param: param})
	}
	return levels, nil
}

// HasTag reports whether tag holds a validate tag, even one that is not a valid
// quoted string, which Planner.Struct reports.
func HasTag(tag reflect.StructTag) bool {
	_, ok, _ := lookup(tag)
	return ok
}
