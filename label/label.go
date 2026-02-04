package label

import (
	"fmt"
	"strings"
)

// Labels allows you to present labels independently from their storage.
type Labels interface {
	// Has returns whether the provided labels exists.
	Has(label string) (exists bool)

	// Get returns the value for the provided labels.
	Get(label string) (value Set)
	Clone() Labels
	Merge(labels Labels) Labels
	Keys() []string
	ToList() []string
	String() string
	Stringify(sep string) string
	Equal(labels Labels) bool
}

type noop struct{}

var Noop = noop{}

type Set map[string]noop

func (s Set) Has(key string) bool {
	_, ok := s[key]
	return ok
}
func (s Set) Add(key string) {
	s[key] = noop{}
}
func (s Set) AddSet(set Set) {
	for key := range set {
		s[key] = noop{}
	}
}
func (s Set) Delete(key string) {
	delete(s, key)
}
func (s Set) Clean() {
	for key := range s {
		delete(s, key)
	}
}
func (s Set) ToSlice() []string {
	arr := make([]string, 0, s.Size())
	for key := range s {
		arr = append(arr, key)
	}
	return arr
}

func (s Set) MustOne() (string, error) {
	size := s.Size()
	if size == 0 {
		return "", nil
	}

	if size == 1 {
		for key := range s {
			return key, nil
		}
	}
	return "", fmt.Errorf("multi value len(%d)", size)
}

func (s Set) Size() int {
	return len(map[string]noop(s))
}
func (s Set) Equal(set Set) bool {
	if s.Size() != set.Size() {
		return false
	}
	for key := range s {
		if !set.Has(key) {
			return false
		}
	}
	return true
}

func (s Set) clone() Set {
	s2 := make(Set, s.Size())
	for k := range s {
		s2[k] = noop{}
	}
	return s2
}

// labels is a map of labels:value. It implements Labels.
type labels map[string]Set

func NewLabels() Labels {
	return make(labels)
}

func NewLabelsFromMap(mm map[string]Set) Labels {
	return labels(mm)
}

// Has returns whether the provided labels exists in the map.
func (ls labels) Has(label string) bool {
	_, exists := ls[label]
	return exists
}

// Get returns the value in the map for the provided labels.
func (ls labels) Get(label string) Set {
	return ls[label]
}

func (ls labels) Clone() Labels {
	return ls.clone()
}

func (ls labels) clone() labels {
	n := make(labels, len(ls))
	for k, v := range ls {
		n[k] = v.clone()
	}
	return n
}

func Merge(lbs ...Labels) Labels {
	all := make(labels)
	for _, lb := range lbs {
		all.Merge(lb)
	}
	return all
}

func (ls labels) Merge(labels2 Labels) Labels {
	// if has same type, use inner merge
	if v, ok := labels2.(labels); ok {
		return ls.merge(v)
	}
	for _, key := range labels2.Keys() {
		vs := labels2.Get(key)
		if set, ok := ls[key]; ok {
			set.AddSet(vs)
		} else {
			ls[key] = vs.clone()
		}
	}
	return ls
}

func (ls labels) Stringify(sep string) string {
	return strings.Join(ls.ToList(), sep)
}
func (ls labels) String() string {
	return ls.Stringify("\r\n")
}
func (ls labels) ToList() []string {
	keys := make([]string, 0, len(ls))
	for k, vs := range ls {
		for v := range vs {
			keys = append(keys, k+"="+v)
		}
	}
	return keys
}

func (ls labels) Keys() []string {
	keys := make([]string, len(ls))
	i := 0
	for k := range ls {
		keys[i] = k
		i += 1
	}
	return keys
}

// merge combines given labels.
func (ls labels) merge(labels labels) labels {
	for k, v := range labels {
		s, ok := ls[k]
		if !ok {
			s = Set{}
			ls[k] = s
		}
		s.AddSet(v)
	}
	return ls
}

func (ls labels) Equal(lb Labels) bool {
	if lb == nil {
		return len(ls) == 0
	}
	if lb1, ok := lb.(labels); ok {
		return Equal(ls, lb1)
	}
	// Backward compatibility for callers holding a *labels.
	if lb1, ok := lb.(*labels); ok && lb1 != nil {
		return Equal(ls, *lb1)
	}

	keys := lb.Keys()
	if len(ls) != len(keys) {
		return false
	}
	for _, key := range keys {
		if !lb.Get(key).Equal(ls[key]) {
			return false
		}
	}
	return true
}

// Equals returns true if the given maps are equal
func Equal(labels1, labels2 labels) bool {
	if len(labels1) != len(labels2) {
		return false
	}

	for k, v := range labels1 {
		value, ok := labels2[k]
		if !ok {
			return false
		}
		if !value.Equal(v) {
			return false
		}
	}
	return true
}

// ParseLabels converts labels []string to labels map
// and validates keys and values
func ParseLabels(rawLabels []string) (Labels, error) {
	labelsMap := make(labels, len(rawLabels))
	return parseLabels(labelsMap, rawLabels)
}

const sep = "\r\n"

// ParseRawLabels converts labels []byte to labels map
// and validates keys and values
func ParseRawLabels(rawLabels string) (Labels, error) {
	n := strings.Count(rawLabels, sep)
	labelsMap := make(labels, n+1)
	return parseLabels(labelsMap, strings.Split(rawLabels, sep))
}

func IsValid(label string) bool {
	// SplitN with 3 preserves the previous behavior of rejecting multiple '='.
	l := strings.SplitN(label, "=", 3)
	if len(l) != 2 {
		return false
	}
	key := strings.TrimSpace(l[0])
	if err := validateLabelKey(key); err != nil {
		return false
	}
	value := strings.TrimSpace(l[1])
	if err := validateLabelValue(key, value); err != nil {
		return false
	}
	return true
}

func parseLabels(labelsMap labels, labels []string) (labels, error) {
	for _, label := range labels {
		l := strings.SplitN(label, "=", 3)
		if len(l) != 2 {
			return labelsMap, fmt.Errorf("invalid label: %q", label)
		}
		key := strings.TrimSpace(l[0])
		if err := validateLabelKey(key); err != nil {
			return labelsMap, err
		}
		value := strings.TrimSpace(l[1])
		if err := validateLabelValue(key, value); err != nil {
			return labelsMap, err
		}
		set, ok := labelsMap[key]
		if !ok {
			set = make(Set, 1)
			labelsMap[key] = set
		}
		set.Add(value)
	}
	return labelsMap, nil
}
