package label

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Token represents constant definition for lexer token
type Token int

const (
	// ErrorToken represents scan error
	ErrorToken Token = iota
	// EndOfStringToken represents end of string
	EndOfStringToken
	// ClosedParToken represents close parenthesis
	ClosedParToken
	// CommaToken represents the comma
	CommaToken
	// DoesNotExistToken represents logic not
	DoesNotExistToken
	// DoubleEqualsToken represents double equals
	DoubleEqualsToken
	// EqualsToken represents equal
	EqualsToken
	// Pattern represents pattern
	PatternToken
	// GreaterThanToken represents greater than
	GreaterThanToken
	// IdentifierToken represents identifier, e.g. keys and values
	IdentifierToken
	// InToken represents in
	InToken
	// LessThanToken represents less than
	LessThanToken
	// NotEqualsToken represents not equal
	NotEqualsToken
	// NotInToken represents not in
	NotInToken
	// OpenParToken represents open parenthesis
	OpenParToken
	// SemverToken represents Semver
	SemverToken
)

// string2token contains the mapping between lexer Token and token literal
// (except IdentifierToken, EndOfStringToken and ErrorToken since it makes no sense)
var string2token = map[string]Token{
	")":     ClosedParToken,
	",":     CommaToken,
	"!":     DoesNotExistToken,
	"==":    DoubleEqualsToken,
	"=":     EqualsToken,
	">":     GreaterThanToken,
	"in":    InToken,
	"<":     LessThanToken,
	"!=":    NotEqualsToken,
	"notin": NotInToken,
	"(":     OpenParToken,
	"~=":    PatternToken,
	"@":     SemverToken,
}

// ScannedItem contains the Token and the literal produced by the lexer.
type ScannedItem struct {
	tok     Token
	literal string
}

// isWhitespace returns true if the rune is a space, tab, or newline.
func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}

// isSpecialSymbol detect if the character ch can be an operator
func isSpecialSymbol(ch byte) bool {
	switch ch {
	case '=', '!', '(', ')', ',', '>', '<', '@', '~':
		return true
	}
	return false
}

// Lexer represents the Lexer struct for labels selector.
// It contains necessary information to tokenize the input string
type Lexer struct {
	// s stores the string to be tokenized
	s string
	// pos is the position currently tokenized
	pos int
}

// read return the character currently lexed
// increment the position and check the buffer overflow
func (l *Lexer) read() (b byte) {
	b = 0
	if l.pos < len(l.s) {
		b = l.s[l.pos]
		l.pos++
	}
	return b
}

// unread 'undoes' the last read character
func (l *Lexer) unread() {
	l.pos--
}

// scanIDOrKeyword scans string to recognize literal token (for example 'in') or an identifier.
func (l *Lexer) scanIDOrKeyword() (tok Token, lit string) {
	start := l.pos
IdentifierLoop:
	for {
		switch ch := l.read(); {
		case ch == 0:
			break IdentifierLoop
		case isSpecialSymbol(ch) || isWhitespace(ch):
			l.unread()
			break IdentifierLoop
		default:
			// continue
		}
	}
	s := l.s[start:l.pos]
	if val, ok := string2token[s]; ok { // is a literal token?
		return val, s
	}
	return IdentifierToken, s // otherwise is an identifier
}

// scanSemverSymbol
func (l *Lexer) scanSemverSymbol() (Token, string) {
	ch := l.read()
	if ch != '@' {
		return ErrorToken, fmt.Sprintf("error expected: '@' found '%c'", rune(ch))
	}
	start := l.pos
SpecialSymbolLoop:
	for {
		switch ch := l.read(); {
		case ch == 0:
			break SpecialSymbolLoop
		case ch == ',':
			l.unread()
			break SpecialSymbolLoop
		default:
			// continue
		}
	}
	return IdentifierToken, l.s[start:l.pos]
}

// scanSpecialSymbol scans string starting with special symbol.
// special symbol identify non literal operators. "!=", "==", "="
func (l *Lexer) scanSpecialSymbol() (Token, string) {
	ch := l.read()
	switch ch {
	case 0:
		return EndOfStringToken, ""
	case ')':
		return ClosedParToken, ")"
	case ',':
		return CommaToken, ","
	case '(':
		return OpenParToken, "("
	case '>':
		return GreaterThanToken, ">"
	case '<':
		return LessThanToken, "<"
	case '@':
		return SemverToken, "@"
	case '~':
		next := l.read()
		if next == '=' {
			return PatternToken, "~="
		}
		// unread the lookahead byte if any
		if next != 0 {
			l.unread()
		}
		return ErrorToken, fmt.Sprintf("error expected: keyword found '%c'", rune(ch))
	case '!':
		next := l.read()
		if next == '=' {
			return NotEqualsToken, "!="
		}
		// unread the lookahead byte if any
		if next != 0 {
			l.unread()
		}
		return DoesNotExistToken, "!"
	case '=':
		next := l.read()
		if next == '=' {
			return DoubleEqualsToken, "=="
		}
		// unread the lookahead byte if any
		if next != 0 {
			l.unread()
		}
		return EqualsToken, "="
	default:
		return ErrorToken, fmt.Sprintf("error expected: keyword found '%c'", rune(ch))
	}
}

// skipWhiteSpaces consumes all blank characters
// returning the first non blank character
func (l *Lexer) skipWhiteSpaces(ch byte) byte {
	for {
		if !isWhitespace(ch) {
			return ch
		}
		ch = l.read()
	}
}

// Lex returns a pair of Token and the literal
// literal is meaningfull only for IdentifierToken token
func (l *Lexer) Lex() (tok Token, lit string) {
	switch ch := l.skipWhiteSpaces(l.read()); {
	case ch == 0:
		return EndOfStringToken, ""
	case ch == '@':
		l.unread()
		return SemverToken, string(Semver)
	case isSpecialSymbol(ch):
		l.unread()
		return l.scanSpecialSymbol()
	default:
		l.unread()
		return l.scanIDOrKeyword()
	}
}

// Parser data structure contains the labels selector parser data structure
type Parser struct {
	l            *Lexer
	scannedItems []ScannedItem
	position     int
}

// ParserContext represents context during parsing:
// some literal for example 'in' and 'notin' can be
// recognized as operator for example 'x in (a)' but
// it can be recognized as value for example 'value in (in)'
type ParserContext int

const (
	// KeyAndOperator represents key and operator
	KeyAndOperator ParserContext = iota
	// Values represents values
	Values
)

// lookahead func returns the current token and string. No increment of current position
func (p *Parser) lookahead(context ParserContext) (Token, string) {
	tok, lit := p.scannedItems[p.position].tok, p.scannedItems[p.position].literal
	if context == Values {
		switch tok {
		case InToken, NotInToken:
			tok = IdentifierToken
		}
	}
	return tok, lit
}

// consume returns current token and string. Increments the position
func (p *Parser) consume(context ParserContext) (Token, string) {
	p.position++
	tok, lit := p.scannedItems[p.position-1].tok, p.scannedItems[p.position-1].literal
	if context == Values {
		switch tok {
		case InToken, NotInToken:
			tok = IdentifierToken
		}
	}
	return tok, lit
}

// scan runs through the input string and stores the ScannedItem in an array
// Parser can now lookahead and consume the tokens
func (p *Parser) scan() {
	for {
		token, literal := p.l.Lex()
		p.scannedItems = append(p.scannedItems, ScannedItem{token, literal})
		if token == SemverToken {
			token, literal := p.l.scanSemverSymbol()
			p.scannedItems = append(p.scannedItems, ScannedItem{token, literal})
		}
		if token == EndOfStringToken {
			break
		}
	}
}

// parse runs the left recursive descending algorithm
// on input string. It returns a list of Requirement objects.
func (p *Parser) parse() (internalSelector, error) {
	p.scan() // init scannedItems

	// Pre-size requirements based on number of comma-separated requirements.
	requirements := make(internalSelector, 0, 1+strings.Count(p.l.s, ","))
	for {
		tok, lit := p.lookahead(Values)
		switch tok {
		case IdentifierToken, DoesNotExistToken:
			r, err := p.parseRequirement()
			if err != nil {
				return nil, fmt.Errorf("unable to parse requirement: %v", err)
			}
			requirements = append(requirements, *r)
			t, l := p.consume(Values)
			switch t {
			case EndOfStringToken:
				return requirements, nil
			case CommaToken:
				t2, l2 := p.lookahead(Values)
				if t2 != IdentifierToken && t2 != DoesNotExistToken {
					return nil, fmt.Errorf("found '%s', expected: identifier after ','", l2)
				}
			default:
				return nil, fmt.Errorf("found '%s', expected: ',' or 'end of string'", l)
			}
		case EndOfStringToken:
			return requirements, nil
		default:
			return nil, fmt.Errorf("found '%s', expected: !, identifier, or 'end of string'", lit)
		}
	}
}

func (p *Parser) parseRequirement() (*Requirement, error) {
	key, operator, err := p.parseKeyAndInferOperator()
	if err != nil {
		return nil, err
	}
	if operator == Exists || operator == DoesNotExist { // operator found lookahead Set checked
		return NewRequirement(key, operator, []string{})
	}
	operator, err = p.parseOperator()
	if err != nil {
		return nil, err
	}
	var values String
	switch operator {
	case In, NotIn:
		values, err = p.parseValues()
	case Equals, Pattern, DoubleEquals, NotEquals, GreaterThan, LessThan:
		values, err = p.parseExactValue()
	case Semver:
		values, err = p.parseSemverValue()
	}
	if err != nil {
		return nil, err
	}
	return NewRequirement(key, operator, values.List())

}

// parseKeyAndInferOperator parse literals.
// in case of no operator '!, in, notin, ==, =, !=, @' are found
// the 'exists' operator is inferred
func (p *Parser) parseKeyAndInferOperator() (string, Operator, error) {
	var operator Operator
	tok, literal := p.consume(Values)
	if tok == DoesNotExistToken {
		operator = DoesNotExist
		tok, literal = p.consume(Values)
	}
	if tok != IdentifierToken {
		err := fmt.Errorf("found '%s', expected: identifier", literal)
		return "", "", err
	}
	if err := validateLabelKey(literal); err != nil {
		return "", "", err
	}
	if t, _ := p.lookahead(Values); t == EndOfStringToken || t == CommaToken {
		if operator != DoesNotExist {
			operator = Exists
		}
	}
	return literal, operator, nil
}

// parseOperator return operator and eventually matchType
// matchType can be exact
func (p *Parser) parseOperator() (op Operator, err error) {
	tok, lit := p.consume(KeyAndOperator)
	switch tok {
	// DoesNotExistToken shouldn't be here because it's a unary operator, not a binary operator
	case InToken:
		op = In
	case EqualsToken:
		op = Equals
	case DoubleEqualsToken:
		op = DoubleEquals
	case GreaterThanToken:
		op = GreaterThan
	case LessThanToken:
		op = LessThan
	case NotInToken:
		op = NotIn
	case NotEqualsToken:
		op = NotEquals
	case PatternToken:
		op = Pattern
	case SemverToken:
		op = Semver
	default:
		return "", fmt.Errorf("found '%s', expected: '=', '!=', '==', 'in', notin'", lit)
	}
	return op, nil
}

// parseValues parses the values for Set based matching (x,y,z)
func (p *Parser) parseValues() (String, error) {
	tok, lit := p.consume(Values)
	if tok != OpenParToken {
		return nil, fmt.Errorf("found '%s' expected: '('", lit)
	}
	tok, lit = p.lookahead(Values)
	switch tok {
	case IdentifierToken, CommaToken:
		s, err := p.parseIdentifiersList() // handles general cases
		if err != nil {
			return s, err
		}
		if tok, _ = p.consume(Values); tok != ClosedParToken {
			return nil, fmt.Errorf("found '%s', expected: ')'", lit)
		}
		return s, nil
	case ClosedParToken: // handles "()"
		p.consume(Values)
		return NewString(""), nil
	default:
		return nil, fmt.Errorf("found '%s', expected: ',', ')' or identifier", lit)
	}
}

// parseIdentifiersList parses a (possibly empty) list of
// of comma separated (possibly empty) identifiers
func (p *Parser) parseIdentifiersList() (String, error) {
	s := NewString()
	for {
		tok, lit := p.consume(Values)
		switch tok {
		case IdentifierToken:
			s.Insert(lit)
			tok2, lit2 := p.lookahead(Values)
			switch tok2 {
			case CommaToken:
				continue
			case ClosedParToken:
				return s, nil
			default:
				return nil, fmt.Errorf("found '%s', expected: ',' or ')'", lit2)
			}
		case CommaToken: // handled here since we can have "(,"
			if s.Len() == 0 {
				s.Insert("") // to handle (,
			}
			tok2, _ := p.lookahead(Values)
			if tok2 == ClosedParToken {
				s.Insert("") // to handle ,)  Double "" removed by StringSet
				return s, nil
			}
			if tok2 == CommaToken {
				p.consume(Values)
				s.Insert("") // to handle ,, Double "" removed by StringSet
			}
		default: // it can be operator
			return s, fmt.Errorf("found '%s', expected: ',', or identifier", lit)
		}
	}
}

func (p *Parser) parseSemverValue() (String, error) {
	s := NewString()
	tok, lit := p.consume(KeyAndOperator)
	if tok == IdentifierToken {
		s.Insert(lit)
		return s, nil
	}
	return nil, fmt.Errorf("found '%s' %d, expected: SemverValue", lit, tok)
}

// parseExactValue parses the only value for exact match style
func (p *Parser) parseExactValue() (String, error) {
	s := NewString()
	tok, _ := p.lookahead(Values)
	if tok == EndOfStringToken || tok == CommaToken {
		s.Insert("")
		return s, nil
	}
	tok, lit := p.consume(Values)
	if tok == IdentifierToken {
		s.Insert(lit)
		return s, nil
	}
	return nil, fmt.Errorf("found '%s' %d, expected: identifier", lit, tok)
}

// ParseSelector takes a string representing a selector and returns a selector
// object, or an error.
// The input will cause an error if it does not follow this form:
//
// <selector-syntax>         ::= <requirement> | <requirement> "," <selector-syntax>
// <requirement>             ::= [!] KEY [ <Set-based-restriction> | <exact-match-restriction> ]
// <Set-based-restriction>   ::= "" | <inclusion-exclusion> <value-Set>
// <inclusion-exclusion>     ::= <inclusion> | <exclusion>
// <exclusion>               ::= "notin"
// <inclusion>               ::= "in"
// <value-Set>               ::= "(" <values> ")"
// <values>                  ::= VALUE | VALUE "," <values>
// <exact-match-restriction> ::= ["~="|"=="|"!="|"@"] VALUE
//
// KEY is a sequence of one or more characters following [ DNS_SUBDOMAIN "/" ] DNS_LABEL. Max length is 63 characters.
// VALUE is a sequence of zero or more characters "([A-Za-z0-9_-\.])". Max length is 63 characters.
// Delimiter is white space: (' ', '\t')
// Example of valid syntax:
//
//	"x in (foo,,baz),y,z notin ()"
//
// Note:
//
//	(1) Inclusion - " in " - denotes that the KEY exists and is equal to any of the
//	    VALUEs in its requirement
//	(2) Exclusion - " notin " - denotes that the KEY is not equal to any
//	    of the VALUEs in its requirement or does not exist
//	(3) The empty string is a valid VALUE
//	(4) A requirement with just a KEY - as in "y" above - denotes that
//	    the KEY exists and can be any VALUE.
//	(5) A requirement with just !KEY requires that the KEY not exist.
func ParseSelector(selector string) (Selector, error) {
	parsedSelector, err := parse(selector)
	if err == nil {
		return parsedSelector, nil
	}
	return nil, err
}

// parse parses the string representation of the selector and returns the internalSelector struct.
// The callers of this method can then decide how to return the internalSelector struct to their
// callers. This function has two callers now, one returns a Selector interface and the other
// returns a list of requirements.
func parse(selector string) (internalSelector, error) {
	// Pre-size scannedItems to reduce slice growth allocations. Token count is
	// typically O(len(selector)), and this keeps the growth amortized.
	capItems := len(selector)/2 + 8
	if capItems < 16 {
		capItems = 16
	}
	p := &Parser{l: &Lexer{s: selector, pos: 0}, scannedItems: make([]ScannedItem, 0, capItems)}
	items, err := p.parse()
	if err != nil {
		return nil, err
	}
	sort.Sort(ByKey(items)) // sort to grant determistic parsing
	return internalSelector(items), err
}

func validateLabelKey(k string) error {
	if errs := IsQualifiedName(k); len(errs) != 0 {
		return fmt.Errorf("invalid labels key %q: %s", k, strings.Join(errs, "; "))
	}
	return nil
}

func validateLabelValue(k, v string) error {
	if errs := IsValidLabelValue(v); len(errs) != 0 {
		return fmt.Errorf("invalid labels value: %q: at key: %q: %s", v, k, strings.Join(errs, "; "))
	}
	return nil
}

// TODO 按类型 validate
func validateSelectorValue(op Operator, k, v string) error {
	switch op {
	case DoesNotExist, Equals, DoubleEquals, In, NotEquals, NotIn, Exists:
		if errs := IsValidLabelValue(v); len(errs) != 0 {
			return fmt.Errorf("invalid selector normal value: %q: at key: %q: %s", v, k, strings.Join(errs, "; "))
		}
	case GreaterThan, LessThan:
		_, err := strconv.ParseInt(v, 0, 64)
		if err != nil {
			return fmt.Errorf("invalid selector number value: %q: at key: %q: %s;%s", v, k, "only allow int64", err)
		}
	case Pattern:
		if errs := IsValidSelectorWildcardValue(v); len(errs) != 0 {
			return fmt.Errorf("invalid selector ext value: %q: at key: %q: %s", v, k, strings.Join(errs, "; "))
		}
	case Semver:
		if !IsValidConstraint(v) {
			return fmt.Errorf("invalid selector semver value: %q: at key: %q", v, k)
		}
	}
	return nil
}

// SelectorFromSet returns a Selector which will match exactly the given labels. A
// nil and empty Sets are considered equivalent to Everything().
// It does not perform any validation, which means the server will reject
// the request if the labels contains invalid values.
func SelectorFromSet(ls labels) Selector {
	return SelectorFromValidatedSet(ls)
}

// ValidatedSelectorFromSet returns a Selector which will match exactly the given labels. A
// nil and empty Sets are considered equivalent to Everything().
// The labels is validated client-side, which allows to catch errors early.
func ValidatedSelectorFromSet(ls labels) (Selector, error) {
	if ls == nil || len(ls) == 0 {
		return internalSelector{}, nil
	}
	requirements := make([]Requirement, 0, len(ls))
	for label, value := range ls {
		r, err := NewRequirement(label, Equals, value.ToSlice())
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, *r)
	}
	// sort to have deterministic string representation
	sort.Sort(ByKey(requirements))
	return internalSelector(requirements), nil
}

// SelectorFromValidatedSet returns a Selector which will match exactly the given labels.
// A nil and empty Sets are considered equivalent to Everything().
// It assumes that labels is already validated and doesn't do any
func SelectorFromValidatedSet(ls labels) Selector {
	if ls == nil || len(ls) == 0 {
		return internalSelector{}
	}
	requirements := make([]Requirement, 0, len(ls))
	for label, value := range ls {
		requirements = append(requirements, Requirement{key: label, operator: Equals, strValues: value.ToSlice()})
	}
	// sort to have deterministic string representation
	sort.Sort(ByKey(requirements))
	return internalSelector(requirements)
}

// ParseToRequirements takes a string representing a selector and returns a list of
// requirements. This function is suitable for those callers that perform additional
// processing on selector requirements.
// See the documentation for ParseSelector() function for more details.
// TODO: Consider exporting the internalSelector type instead.
func ParseToRequirements(selector string) ([]Requirement, error) {
	return parse(selector)
}
