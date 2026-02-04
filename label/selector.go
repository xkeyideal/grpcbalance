package label

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

type ReadOnlySelectors []ReadOnlySelector

func (s ReadOnlySelectors) Match(labels Labels) bool {
	if len(s) == 0 {
		return true
	}
	for _, selector := range s {
		if selector.Matches(labels) {
			return true
		}
	}
	return false
}

func (s ReadOnlySelectors) Equal(_s ISelectors) bool {
	s2, ok := _s.(Selectors)
	if !ok {
		return false
	}
	if len(s) != len(s2) {
		return false
	}
	for i, selector := range s {
		if !selector.Equal(s2[i]) {
			return false
		}
	}
	return true
}

type ReadOnlySelector interface {
	Equal(selector Selector) bool
	// Matches returns true if this selector matches the given Set of labels.
	Matches(Labels) bool
	// Empty returns true if this selector does not restrict the selection space.
	Empty() bool

	// String returns a human readable string that represents this selector.
	String() string
	Bytes() []byte

	// Requirements converts this interface into Requirements to expose
	// more detailed selection information.
	// If there are querying parameters, it will return converted requirements and selectable=true.
	// If this selector doesn't want to select anything, it will return selectable=false.
	Requirements() (requirements Requirements)

	// DeepCopySelector Make a deep copy of the selector.
	DeepCopySelector() Selector

	// RequiresExactMatch allows a caller to introspect whether a given selector
	// requires a single specific labels to be Set, and if so returns the value it
	// requires.
	RequiresExactMatch(label string) (value string, found bool)
}
type Selector interface {
	ReadOnlySelector

	// Add adds requirements to the Selector
	Add(r ...Requirement) Selector
}

// Everything returns a selector that matches all labels.
func Everything() Selector {
	return internalSelector{}
}

type nothingSelector struct{}

func (n nothingSelector) Equal(n1 Selector) bool {
	_, ok := n1.(*nothingSelector)
	return ok
}
func (n nothingSelector) Matches(_ Labels) bool         { return false }
func (n nothingSelector) Empty() bool                   { return false }
func (n nothingSelector) String() string                { return "" }
func (n nothingSelector) Bytes() []byte                 { return nil }
func (n nothingSelector) Add(_ ...Requirement) Selector { return n }
func (n nothingSelector) Requirements() Requirements    { return nil }
func (n nothingSelector) DeepCopySelector() Selector    { return n }
func (n nothingSelector) RequiresExactMatch(label string) (value string, found bool) {
	return "", false
}

// Nothing returns a selector that matches no labels
func Nothing() Selector {
	return nothingSelector{}
}

// NewSelector returns a nil selector
func NewSelector() Selector {
	return internalSelector(nil)
}

type internalSelector []Requirement

func (s internalSelector) Equal(s2 Selector) bool {
	r1 := s.Requirements()
	r2 := s2.Requirements()
	if len(r1) != len(r2) {
		return false
	}
	for i, rr := range r1 {
		if !rr.Equal(r2[i]) {
			return false
		}
	}
	return true
}
func (s internalSelector) DeepCopy() internalSelector {
	if s == nil {
		return nil
	}
	result := make([]Requirement, len(s))
	for i := range s {
		s[i].DeepCopyInto(&result[i])
	}
	return result
}

func (s internalSelector) DeepCopySelector() Selector {
	return s.DeepCopy()
}

// Empty returns true if the internalSelector doesn't restrict selection space
func (s internalSelector) Empty() bool {
	if s == nil {
		return true
	}
	return len(s) == 0
}

// Add adds requirements to the selector. It copies the current selector returning a new one
func (s internalSelector) Add(reqs ...Requirement) Selector {
	ret := make(internalSelector, 0, len(s)+len(reqs))
	ret = append(ret, s...)
	ret = append(ret, reqs...)
	sort.Sort(ByKey(ret))
	return ret
}

// Matches for a internalSelector returns true if all
// its Requirements match the input Labels. If any
// Requirement does not match, false is returned.
func (s internalSelector) Matches(l Labels) bool {
	if len(s) == 0 {
		return true
	}
	for ix := range s {
		if matches := s[ix].Matches(l); !matches {
			return false
		}
	}
	return true
}

func (s internalSelector) Requirements() Requirements { return Requirements(s) }

// String returns a comma-separated string of all
// the internalSelector Requirements' human-readable strings.
func (s internalSelector) String() string {
	if len(s) == 0 {
		return ""
	}
	reqs := make([]string, len(s))
	for ix := range s {
		reqs[ix] = s[ix].String()
	}
	return strings.Join(reqs, ",")
}

// Bytes returns a comma-separated bytes of all
// the internalSelector Requirements' human-readable strings.
func (s internalSelector) Bytes() []byte {
	if len(s) == 0 {
		return nil
	}
	reqs := make([][]byte, len(s))
	for ix := range s {
		reqs[ix] = s[ix].Bytes()
	}
	return bytes.Join(reqs, []byte{','})
}

// RequiresExactMatch introspect whether a given selector requires a single specific field
// to be Set, and if so returns the value it requires.
func (s internalSelector) RequiresExactMatch(label string) (value string, found bool) {
	for ix := range s {
		if s[ix].key == label {
			switch s[ix].operator {
			case Equals, DoubleEquals, In:
				if len(s[ix].strValues) == 1 {
					return s[ix].strValues[0], true
				}
			}
			return "", false
		}
	}
	return "", false
}

// ByKey sorts requirements by key to obtain deterministic parser
type ByKey []Requirement

func (a ByKey) Len() int { return len(a) }

func (a ByKey) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

func (a ByKey) Less(i, j int) bool { return a[i].key < a[j].key }

type ISelectors interface {
	Match(labels Labels) bool
	Equal(selectors ISelectors) bool
}

func EqISelectors(is1, is2 ISelectors) bool {
	if is1 == nil || is2 == nil {
		return is1 == is2
	}
	return is1.Equal(is2)
}

type Selectors []Selector

func (s Selectors) Equal(_s ISelectors) bool {
	s2, ok := _s.(Selectors)
	if !ok {
		return false
	}
	if len(s) != len(s2) {
		return false
	}
	for i, selector := range s {
		if !selector.Equal(s2[i]) {
			return false
		}
	}
	return true
}

func (s Selectors) Match(labels Labels) bool {
	if len(s) == 0 {
		return true
	}
	for _, selector := range []Selector(s) {
		if selector.Matches(labels) {
			return true
		}
	}
	return false
}

func (s Selectors) MatchRawLabels(rawLabels []string) (bool, error) {
	labels, err := ParseLabels(rawLabels)
	if err != nil {
		return false, err
	}
	return s.Match(labels), nil
}
func (s Selectors) Values() []Selector {
	return s
}

func (s Selectors) String() string {
	v, _ := json.Marshal(s)
	return string(v)
}
func ParseSelectors(rawSelectors []string) (Selectors, error) {

	if len(rawSelectors) == 0 {
		return []Selector{Everything()}, nil
	}

	selectors := make([]Selector, len(rawSelectors))
	var err error
	for i, selector := range rawSelectors {
		selectors[i], err = ParseSelector(selector)
		if err != nil {
			return nil, err
		}
	}
	return selectors, nil
}
