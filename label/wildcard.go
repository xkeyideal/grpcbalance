package label

import "unicode/utf8"

// Match -  finds whether the text matches/satisfies the pattern string.
// supports  '*' and '?' wildcards in the pattern string.
// unlike path.Match(), considers a path as a flat name space while matching the pattern.
// The difference is illustrated in the example here https://play.golang.org/p/Ega9qgD4Qz .
func Match(pattern, name string) (matched bool) {
	if pattern == "" {
		return name == pattern
	}
	if pattern == "*" {
		return true
	}
	if isASCII(pattern) && isASCII(name) {
		return deepMatchByte([]byte(name), []byte(pattern))
	}
	// Does extended wildcard '*' and '?' match.
	return deepMatchRune([]rune(name), []rune(pattern), false)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func deepMatchByte(str, pattern []byte) bool {
	si, pi := 0, 0
	starPi := -1
	starSi := 0

	for si < len(str) {
		if pi < len(pattern) {
			switch pattern[pi] {
			case '*':
				starPi = pi
				pi++
				starSi = si
				continue
			case '?':
				si++
				pi++
				continue
			default:
				if pattern[pi] == str[si] {
					si++
					pi++
					continue
				}
			}
		}

		if starPi != -1 {
			pi = starPi + 1
			starSi++
			si = starSi
			continue
		}
		return false
	}

	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

func deepMatchRune(str, pattern []rune, simple bool) bool {
	_ = simple // kept for backward compatibility; current matching treats '?' as single-rune.

	si, pi := 0, 0
	starPi := -1
	starSi := 0

	for si < len(str) {
		if pi < len(pattern) {
			switch pattern[pi] {
			case '*':
				starPi = pi
				pi++
				starSi = si
				continue
			case '?':
				si++
				pi++
				continue
			default:
				if pattern[pi] == str[si] {
					si++
					pi++
					continue
				}
			}
		}

		// Mismatch: if we saw a '*', backtrack and let it consume one more rune.
		if starPi != -1 {
			pi = starPi + 1
			starSi++
			si = starSi
			continue
		}
		return false
	}

	// Consume trailing '*' in pattern.
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}
