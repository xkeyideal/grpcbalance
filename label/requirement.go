package label

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Requirements is AND of all requirements.
type Requirements []Requirement

type Requirement struct {
	key      string
	operator Operator
	// In huge majority of cases we have at most one value here.
	// It is generally faster to operate on a single-element slice
	// than on a single-element map, so we have a slice here.
	strValues []string
}

// NewRequirement is the constructor for a Requirement.
// If any of these rules is violated, an error is returned:
// (1) The operator can only be In, NotIn, Equals, DoubleEquals, NotEquals, Exists, or DoesNotExist.
// (2) If the operator is In or NotIn, the values sets must be non-empty.
// (3) If the operator is Equals, DoubleEquals, or NotEquals, the values sets must contain one value.
// (4) If the operator is Exists or DoesNotExist, the value sets must be empty.
// (5) If the operator is Gt or Lt, the values sets must contain only one value, which will be interpreted as an integer.
// (6) The key is invalid due to its length, or sequence
//     of characters. See validateLabelKey for more details.
//
// The empty string is a valid value in the input values
func NewRequirement(key string, op Operator, vals []string) (*Requirement, error) {
	if err := validateLabelKey(key); err != nil {
		return nil, err
	}
	switch op {
	case In, NotIn:
		if len(vals) == 0 {
			return nil, fmt.Errorf("for 'in', 'notin' operators, values sets can't be empty")
		}
	case Equals, DoubleEquals, NotEquals, Pattern, Semver:
		if len(vals) != 1 {
			return nil, fmt.Errorf("exact-match compatibility requires one single value")
		}
	case Exists, DoesNotExist:
		if len(vals) != 0 {
			return nil, fmt.Errorf("values sets must be empty for exists and does not exist")
		}
	case GreaterThan, LessThan:
		if len(vals) != 1 {
			return nil, fmt.Errorf("for 'Gt', 'Lt' operators, exactly one value is required")
		}
		for i := range vals {
			if _, err := strconv.ParseInt(vals[i], 0, 64); err != nil {
				return nil, fmt.Errorf("for 'Gt', 'Lt' operators, the value must be an integer")
			}
		}
	default:
		return nil, fmt.Errorf("operator '%v' is not recognized", op)
	}

	for i := range vals {
		if err := validateSelectorValue(op, key, vals[i]); err != nil {
			return nil, err
		}
	}
	return &Requirement{key: key, operator: op, strValues: vals}, nil
}

func (r *Requirement) Equal(requirement Requirement) bool {
	if r.key != requirement.key || r.operator != requirement.operator {
		return false
	}
	if len(r.strValues) != len(requirement.strValues) {
		return false
	}
	for i, value := range r.strValues {
		if value != requirement.strValues[i] {
			return false
		}
	}
	return true
}

func (r *Requirement) hasValue(set Set) bool {
	for _, v := range r.strValues {
		if set.Has(v) {
			return true
		}
	}
	return false
}

// Matches returns true if the Requirement matches the input Labels.
// There is a match in the following cases:
// (1) The operator is Exists and Labels has the Requirement's key.
// (2) The operator is In, Labels has the Requirement's key and Labels'
//     value for that key is in Requirement's value
// (3) The operator is NotIn, Labels has the Requirement's key and
//     Labels' value for that key is not in Requirement's value
// (4) The operator is DoesNotExist or NotIn and Labels does not have the
//     Requirement's key.
// (5) The operator is GreaterThanOperator or LessThanOperator, and Labels has
//     the Requirement's key and the corresponding value satisfies mathematical inequality.
func (r *Requirement) Matches(ls Labels) bool {
	switch r.operator {
	case In, Equals, DoubleEquals:
		if !ls.Has(r.key) {
			return false
		}
		return r.hasValue(ls.Get(r.key))
	case NotIn, NotEquals:
		if !ls.Has(r.key) {
			return true
		}
		return !r.hasValue(ls.Get(r.key))
	case Exists:
		return ls.Has(r.key)
	case DoesNotExist:
		return !ls.Has(r.key)
	case GreaterThan, LessThan:
		if !ls.Has(r.key) {
			return false
		}
		set := ls.Get(r.key)
		for value := range set {

			lsValue, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				//klog.V(10).Infof("ParseInt failed for value %+v in labels %+v, %+v", ls.Get(r.key), ls, err)
				return false
			}

			// There should be only one strValue in r.strValues, and can be converted to an integer.
			if len(r.strValues) != 1 {
				//klog.V(10).Infof("Invalid values count %+v of requirement %#v, for 'Gt', 'Lt' operators, exactly one value is required", len(r.strValues), r)
				return false
			}

			var rValue int64
			for i := range r.strValues {
				rValue, err = strconv.ParseInt(r.strValues[i], 0, 64)
				if err != nil {
					//klog.V(10).Infof("ParseInt failed for value %+v in requirement %#v, for 'Gt', 'Lt' operators, the value must be an integer", r.strValues[i], r)
					return false
				}
			}
			if (r.operator == GreaterThan && lsValue > rValue) || (r.operator == LessThan && lsValue < rValue) {
				return true
			}
		}
		return false
	case Semver:
		if !ls.Has(r.key) {
			return false
		}
		versions := ls.Get(r.key)
		dep := r.strValues[0]
		for version := range versions {
			res, err := Satisfy(dep, version)
			if err != nil {
				continue
			}
			if res {
				return true
			}
		}
		return false
	case Pattern:
		values := ls.Get(r.Key())
		for value := range values {
			if Match(r.strValues[0], value) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// Key returns requirement key
func (r *Requirement) Key() string {
	return r.key
}

// Operator returns requirement operator
func (r *Requirement) Operator() Operator {
	return r.operator
}

// Values returns requirement values
func (r *Requirement) Values() String {
	ret := String{}
	for i := range r.strValues {
		ret.Insert(r.strValues[i])
	}
	return ret
}

// safeSort sort input strings without modification
func safeSort(in []string) []string {
	if sort.StringsAreSorted(in) {
		return in
	}
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

// String returns a human-readable string that represents this
// Requirement. If called on an invalid Requirement, an error is
// returned. See NewRequirement for creating a valid Requirement.
func (r *Requirement) String() string {
	return r.buffered().String()
}

func (r *Requirement) buffered() *bytes.Buffer {
	var buffer = &bytes.Buffer{}
	if r.operator == DoesNotExist {
		buffer.WriteString("!")
	}
	buffer.WriteString(r.key)

	switch r.operator {
	case Equals:
		buffer.WriteString("=")
	case DoubleEquals:
		buffer.WriteString("==")
	case NotEquals:
		buffer.WriteString("!=")
	case In:
		buffer.WriteString(" in ")
	case NotIn:
		buffer.WriteString(" notin ")
	case GreaterThan:
		buffer.WriteString(">")
	case LessThan:
		buffer.WriteString("<")
	case Pattern:
		buffer.WriteString("~=")
	case Semver:
		buffer.WriteString("@")
	case Exists, DoesNotExist:
		return buffer
	}

	switch r.operator {
	case In, NotIn:
		buffer.WriteString("(")
	}
	if len(r.strValues) == 1 {
		buffer.WriteString(r.strValues[0])
	} else { // only > 1 since == 0 prohibited by NewRequirement
		// normalizes value order on output, without mutating the in-memory selector representation
		// also avoids normalization when it is not required, and ensures we do not mutate shared data
		buffer.WriteString(strings.Join(safeSort(r.strValues), ","))
	}

	switch r.operator {
	case In, NotIn:
		buffer.WriteString(")")
	}
	return buffer
}

func (r *Requirement) Bytes() []byte {
	return r.buffered().Bytes()
}
