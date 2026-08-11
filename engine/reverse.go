package engine

import (
	"unicode"

	bamboo "github.com/BambooEngine/bamboo-core"
)

// toneKeys maps a Vietnamese tone to the Telex key that produces it.
var toneKeys = map[bamboo.Tone]rune{
	bamboo.ToneGrave: 'f',
	bamboo.ToneAcute: 's',
	bamboo.ToneHook:  'r',
	bamboo.ToneTilde: 'x',
	bamboo.ToneDot:   'j',
}

// markPairs maps a toneless marked letter to the Telex key pair that types it.
var markPairs = map[rune]string{
	'â': "aa",
	'ă': "aw",
	'ê': "ee",
	'ô': "oo",
	'ơ': "ow",
	'ư': "uw",
	'đ': "dd",
}

// IsWordRune reports whether r can be part of an adoptable word: ASCII
// letters or Vietnamese letters (toned/marked vowels and đ).
func IsWordRune(r rune) bool {
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
		return true
	}
	return bamboo.IsVietnameseRune(unicode.ToLower(r))
}

// rawFromWord derives Telex keystrokes that would produce word: marks are
// typed inline (aa, ow, dd, ...) and the single tone key at the end of the
// word. Returns "" when the word cannot be a plain Vietnamese/ASCII word
// (digits, punctuation, two different tones, non-Vietnamese diacritics).
// The result is a candidate only — callers must replay-verify it.
func rawFromWord(word string) string {
	var keys []rune
	tone := bamboo.ToneNone
	for _, r := range word {
		lower := unicode.ToLower(r)
		upper := r != lower
		if t := bamboo.FindToneFromChar(lower); t != bamboo.ToneNone {
			if tone != bamboo.ToneNone && tone != t {
				return ""
			}
			tone = t
			lower = bamboo.AddToneToChar(lower, uint8(bamboo.ToneNone))
		}
		if pair, ok := markPairs[lower]; ok {
			pr := []rune(pair)
			if upper {
				keys = append(keys, unicode.ToUpper(pr[0]), unicode.ToUpper(pr[1]))
			} else {
				keys = append(keys, pr[0], pr[1])
			}
			continue
		}
		if lower < 'a' || lower > 'z' {
			return ""
		}
		// Append the tone-stripped letter, not the original rune: for "cá"
		// the key typed was 'a', the tone arrives via the key appended below.
		if upper {
			keys = append(keys, unicode.ToUpper(lower))
		} else {
			keys = append(keys, lower)
		}
	}
	if tone != bamboo.ToneNone {
		keys = append(keys, toneKeys[tone])
	}
	return string(keys)
}
