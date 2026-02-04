package label

import (
	"fmt"
	"regexp"
)

// LabelValueMaxLength is a labels's max length
const LabelValueMaxLength int = 63

// IsValidLabelValue tests whether the value passed is a valid labels value.  If
// the value is not valid, a list of error strings is returned.  Otherwise an
// empty list (or nil) is returned.
func IsValidLabelValue(value string) []string {
	var errs []string
	if len(value) > LabelValueMaxLength {
		errs = append(errs, MaxLenError(LabelValueMaxLength))
	}
	if !labelValueRegexp.MatchString(value) {
		errs = append(errs, RegexError(labelValueErrMsg, labelValueFmt, "MyValue", "my_value", "12345"))
	}
	return errs
}

// IsValidSelectorValue tests whether the value passed is a valid labels value.  If
// the value is not valid, a list of error strings is returned.  Otherwise an
// empty list (or nil) is returned.
func IsValidSelectorValue(value string) []string {
	var errs []string
	if len(value) > LabelValueMaxLength {
		errs = append(errs, MaxLenError(LabelValueMaxLength))
	}
	if !selectorValueRegexp.MatchString(value) {
		errs = append(errs, RegexError(selectorValueErrMsg, selectorValueFmt, "MyValue", "my_value", "12345"))
	}
	return errs
}

// IsValidSelectorWildcardValue tests whether the value passed is a valid labels value.  If
// the value is not valid, a list of error strings is returned.  Otherwise an
// empty list (or nil) is returned.
func IsValidSelectorWildcardValue(value string) []string {
	var errs []string
	if len(value) > LabelValueMaxLength {
		errs = append(errs, MaxLenError(LabelValueMaxLength))
	}
	if !selectorWildcardRegexp.MatchString(value) {
		errs = append(errs, RegexError(selectorWildcardValueErrMsg, selectorWildcardFmt, "MyValue-*", "*app*", "123?5"))
	}
	return errs
}

// MaxLenError returns a string explanation of a "string too long" validation
// failure.
func MaxLenError(length int) string {
	return fmt.Sprintf("must be no more than %d characters", length)
}

const labelValueFmt string = "((" + qnameCharFmt + qValueExtCharFmt + "*)?" + qnameCharFmt + ")?"
const labelValueErrMsg string = "a valid labels must be an empty string or consist of alphanumeric characters, '-', '+', ':', '_' or '.', and must start and end with an alphanumeric character"

var labelValueRegexp = regexp.MustCompile("^" + labelValueFmt + "$")

const charCJK = "\\p{Han}\\p{Hiragana}\\p{Katakana}\\p{Hangul}"

const qValueExtCharFmt string = "[-A-Za-z0-9_.:+" + charCJK + "]"

const qnameCharFmt string = "[A-Za-z0-9%" + charCJK + "]"
const qnameExtCharFmt string = "[-A-Za-z0-9_.%" + charCJK + "]"
const qualifiedNameFmt string = "(" + qnameCharFmt + qnameExtCharFmt + "*)?" + qnameCharFmt
const qualifiedNameErrMsg string = "must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character"
const qualifiedNameMaxLength int = 63

const qualifiedSelectorCharFmt = "[A-Za-z0-9%" + charCJK + "]"
const qualifiedSelectorExtCharFmt = "[-A-Za-z0-9_:+.%" + charCJK + "]"
const qualifiedSelectorNameFmt = "(" + qualifiedSelectorCharFmt + qualifiedSelectorExtCharFmt + "*)?" + qualifiedSelectorCharFmt
const selectorValueFmt = "(" + qualifiedSelectorNameFmt + ")?"
const selectorWildcardFmt = "([-A-Za-z0-9_:+.?*%" + charCJK + "]+)?"
const selectorWildcardValueErrMsg string = "a valid wildcard selector must be an empty string or consist of alphanumeric characters, '-', '+', ':', '_', '?', '*' or '.'"
const selectorValueErrMsg string = "a valid selector must be an empty string or consist of alphanumeric characters, '-', '+', ':', '_' or '.'"

var qualifiedNameRegexp = regexp.MustCompile("^" + qualifiedNameFmt + "$")
var selectorValueRegexp = regexp.MustCompile("^" + selectorValueFmt + "$")
var selectorWildcardRegexp = regexp.MustCompile("^" + selectorWildcardFmt + "$")

// IsQualifiedName tests whether the value passed is what Kubernetes calls a
// "qualified name".  This is a format used in various places throughout the
// system.  If the value is not valid, a list of error strings is returned.
// Otherwise an empty list (or nil) is returned.
func IsQualifiedName(name string) []string {
	var errs []string
	if len(name) == 0 {
		errs = append(errs, "name part "+EmptyError())
	} else if len(name) > qualifiedNameMaxLength {
		errs = append(errs, "name part "+MaxLenError(qualifiedNameMaxLength))
	}
	if !qualifiedNameRegexp.MatchString(name) {
		errs = append(errs, "name part "+RegexError(qualifiedNameErrMsg, qualifiedNameFmt, "MyName", "my.name", "123-abc"))
	}
	return errs
}

// EmptyError returns a string explanation of a "must not be empty" validation
// failure.
func EmptyError() string {
	return "must be non-empty"
}

// RegexError returns a string explanation of a regex validation failure.
func RegexError(msg string, fmt string, examples ...string) string {
	if len(examples) == 0 {
		return msg + " (regex used for validation is '" + fmt + "')"
	}
	msg += " (e.g. "
	for i := range examples {
		if i > 0 {
			msg += " or "
		}
		msg += "'" + examples[i] + "', "
	}
	msg += "regex used for validation is '" + fmt + "')"
	return msg
}
