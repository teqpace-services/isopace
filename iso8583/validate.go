// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Copyright (C) 2026 Teqpace Services Ltd.
//
// This file is part of Isopace, a financial transaction framework.
//
// Isopace is dual-licensed:
//   - under the GNU Affero General Public License v3.0 or later (see LICENSE); or
//   - under a commercial license from Teqpace Services Ltd. (see COMMERCIAL-LICENSE.md).
//
// Authorship is recorded in the AUTHORS file.

package iso8583

import (
	"fmt"
	"regexp"
)

// Validator is one field-level rule. It returns nil when the value is
// acceptable, or a *Violation describing the failure. Validators are pure and
// composable.
type Validator interface {
	Validate(v Value, def *FieldDef) *Violation
}

// ValidatorFunc adapts a function to the Validator interface.
type ValidatorFunc func(Value, *FieldDef) *Violation

// Validate implements Validator.
func (f ValidatorFunc) Validate(v Value, d *FieldDef) *Violation { return f(v, d) }

func violate(v Value, rule, msg string) *Violation {
	return &Violation{Offset: v.off, Rule: rule, Msg: msg}
}

// Required flags an empty value (zero-length body) as a violation. Presence
// (bitmap) is enforced by Codec.Validate via RequiredRule; this guards content.
func Required() Validator {
	return ValidatorFunc(func(v Value, _ *FieldDef) *Violation {
		if len(v.Bytes()) == 0 {
			return violate(v, "required", "value is empty")
		}
		return nil
	})
}

// Digits requires every byte to be an ASCII decimal digit.
func Digits() Validator {
	return ValidatorFunc(func(v Value, _ *FieldDef) *Violation {
		b := v.Bytes()
		for i := 0; i < len(b); i++ {
			if b[i] < '0' || b[i] > '9' {
				return violate(v, "digits", "non-digit byte in value")
			}
		}
		return nil
	})
}

// Luhn verifies the trailing check digit per the Luhn algorithm (PAN, DE 2).
func Luhn() Validator {
	return ValidatorFunc(func(v Value, _ *FieldDef) *Violation {
		b := v.Bytes()
		if len(b) == 0 {
			return violate(v, "luhn", "empty value")
		}
		sum := 0
		double := false
		for i := len(b) - 1; i >= 0; i-- {
			c := b[i]
			if c < '0' || c > '9' {
				return violate(v, "luhn", "non-digit byte in value")
			}
			d := int(c - '0')
			if double {
				d *= 2
				if d > 9 {
					d -= 9
				}
			}
			sum += d
			double = !double
		}
		if sum%10 != 0 {
			return violate(v, "luhn", "luhn check failed")
		}
		return nil
	})
}

// LenBetween requires the value byte length to be within [min, max].
func LenBetween(min, max int) Validator {
	return ValidatorFunc(func(v Value, _ *FieldDef) *Violation {
		n := len(v.Bytes())
		if n < min || n > max {
			return violate(v, "len", fmt.Sprintf("length %d outside [%d,%d]", n, min, max))
		}
		return nil
	})
}

// MaxLength requires the value byte length to be at most n.
func MaxLength(n int) Validator {
	return ValidatorFunc(func(v Value, _ *FieldDef) *Violation {
		if l := len(v.Bytes()); l > n {
			return violate(v, "len", fmt.Sprintf("length %d exceeds max %d", l, n))
		}
		return nil
	})
}

// OneOf requires the decoded string to equal one of the allowed values.
func OneOf(allowed ...string) Validator {
	set := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		set[a] = struct{}{}
	}
	return ValidatorFunc(func(v Value, _ *FieldDef) *Violation {
		s, err := v.String()
		if err != nil {
			return violate(v, "oneof", err.Error())
		}
		if _, ok := set[s]; !ok {
			return violate(v, "oneof", fmt.Sprintf("value %q not in allowed set", s))
		}
		return nil
	})
}

// Regexp requires the decoded string to match the pattern. The pattern is
// compiled once at construction; an invalid pattern panics (it is a constant).
func Regexp(pattern string) Validator {
	re := regexp.MustCompile(pattern)
	return ValidatorFunc(func(v Value, _ *FieldDef) *Violation {
		s, err := v.String()
		if err != nil {
			return violate(v, "regexp", err.Error())
		}
		if !re.MatchString(s) {
			return violate(v, "regexp", fmt.Sprintf("value %q does not match /%s/", s, pattern))
		}
		return nil
	})
}

// CurrencyAmount checks that an amount field decodes to a well-formed Decimal.
func CurrencyAmount() Validator {
	return ValidatorFunc(func(v Value, _ *FieldDef) *Violation {
		if _, err := v.Decimal(); err != nil {
			return violate(v, "currency", err.Error())
		}
		return nil
	})
}

// MTIClass requires the value (an MTI) to begin with one of the allowed class
// prefixes.
func MTIClass(allowed ...string) Validator {
	return ValidatorFunc(func(v Value, _ *FieldDef) *Violation {
		s, err := v.String()
		if err != nil {
			return violate(v, "mti", err.Error())
		}
		if !mtiInClasses(s, allowed) {
			return violate(v, "mti", fmt.Sprintf("MTI %q not in allowed classes", s))
		}
		return nil
	})
}

// All combines validators with AND, returning the first violation.
func All(vs ...Validator) Validator {
	return ValidatorFunc(func(v Value, d *FieldDef) *Violation {
		for _, rule := range vs {
			if viol := rule.Validate(v, d); viol != nil {
				return viol
			}
		}
		return nil
	})
}
