package pbx

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// SpeechWords lowercases text, drops accents and punctuation, and splits it into words.
func SpeechWords(s string) []string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	if out, _, err := transform.String(t, s); err == nil {
		s = out
	}
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// MatchIntent returns the intent (from 1) whose keywords occur most in what the caller said, or 0.
// A keyword word matches a spoken word that starts with it ("factu" matches "factuur" and
// "facturen"); a keyword of several words matches them in a row. Ties go to the first intent.
func MatchIntent(said string, intents []SpeechIntent) int {
	words := SpeechWords(said)
	best, bestScore := 0, 0
	for i, in := range intents {
		score := 0
		for _, k := range in.Keywords {
			if kw := SpeechWords(k); len(kw) > 0 && containsPhrase(words, kw) {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = i+1, score
		}
	}
	return best
}

func containsPhrase(words, phrase []string) bool {
	for i := 0; i+len(phrase) <= len(words); i++ {
		ok := true
		for j, p := range phrase {
			if !strings.HasPrefix(words[i+j], p) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
