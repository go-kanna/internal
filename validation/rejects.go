package validation

import (
	"fmt"
	"go/constant"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"
)

// The codes the rules that Rejects tells report, as go-kanna/validator spells
// them.
var (
	compareCodes = map[token.Token]string{
		token.EQL: "equal_to", token.NEQ: "other_than",
		token.LSS: "less_than", token.LEQ: "less_than_or_equal_to",
		token.GTR: "greater_than", token.GEQ: "greater_than_or_equal_to",
	}
	lengthCodes = map[token.Token]string{token.GEQ: "too_short", token.LEQ: "too_long", token.EQL: "wrong_length"}
)

// Rejects reports whether the checks of v, a field's, reject value, a constant
// the field could hold, such as its default, by a rule that needs nothing but
// the value to tell: required, a length, a comparison, oneof, or a pattern. It
// returns what the rule reports, as go-kanna/validator spells it, such as
// "greater_than_or_equal_to (count=1)", or "(count=1s)" for a time.Duration,
// which value holds in nanoseconds. A rule that runs code of its own, or
// looks at another field, is left to the run, and so are all the rules of a
// value whose required or omitempty asks the IsZero method of its type.
func Rejects(v *Value, value constant.Value) (string, bool) {
	if v == nil || v.pointee != nil {
		return "", false
	}
	if v.zero == zeroMethod && (v.required || v.omitEmpty) {
		return "", false
	}
	if zero := isZero(value); v.required && zero {
		return "blank", true
	} else if v.omitEmpty && zero {
		return "", false
	}
	for _, c := range v.checks {
		if reported, ok := rejects(c, value); ok {
			return reported, true
		}
	}
	return "", false
}

func rejects(c check, value constant.Value) (string, bool) {
	switch c := c.(type) {
	case lengthCheck:
		if value.Kind() != constant.String {
			return "", false
		}
		s := constant.StringVal(value)
		n := len(s)
		if c.runes {
			n = utf8.RuneCountInString(s)
		}
		if !constant.Compare(constant.MakeInt64(int64(n)), c.op, constant.MakeInt64(int64(c.bound))) {
			return fmt.Sprintf("%s (count=%d)", lengthCodes[c.op], c.bound), true
		}
	case compareCheck:
		bound := numberOf(c.bound)
		if !isNumber(value) || bound.Kind() == constant.Unknown {
			return "", false
		}
		if !constant.Compare(value, c.op, bound) {
			count := c.bound
			if c.duration {
				n, _ := constant.Int64Val(bound)
				count = time.Duration(n).String()
			}
			return fmt.Sprintf("%s (count=%s)", compareCodes[c.op], count), true
		}
	case oneOfCheck:
		if !slices.ContainsFunc(c.values, func(lit string) bool { return same(value, lit) }) {
			return "inclusion", true
		}
	case patternCheck:
		re, err := regexp.Compile(c.expr)
		if err != nil || value.Kind() != constant.String {
			return "", false
		}
		if !re.MatchString(constant.StringVal(value)) {
			return "invalid", true
		}
	case confirmCheck, formatCheck, ruleCheck:
		// Another field, or code of its own, decides.
	}
	return "", false
}

// same reports whether value is lit, a Go literal oneof lists.
func same(value constant.Value, lit string) bool {
	if value.Kind() == constant.String {
		s, err := strconv.Unquote(lit)
		return err == nil && s == constant.StringVal(value)
	}
	n := numberOf(lit)
	return isNumber(value) && n.Kind() != constant.Unknown && constant.Compare(value, token.EQL, n)
}

func isNumber(v constant.Value) bool {
	switch v.Kind() {
	case constant.Int, constant.Float:
		return true
	case constant.Unknown, constant.Bool, constant.String, constant.Complex:
	}
	return false
}

// isZero reports whether v is the zero value of its kind.
func isZero(v constant.Value) bool {
	switch v.Kind() {
	case constant.String:
		return constant.StringVal(v) == ""
	case constant.Bool:
		return !constant.BoolVal(v)
	case constant.Int, constant.Float:
		return constant.Sign(v) == 0
	case constant.Unknown, constant.Complex:
	}
	return false
}
