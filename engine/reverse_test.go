package engine

import "testing"

// Adopting a committed word only knows its on-screen form; the derived raw
// keys must replay to exactly that form or the adoption is refused.
func TestResumeWordRoundTrip(t *testing.T) {
	words := []string{
		"việt", "người", "được", "đường", "quốc", "hưởng", "tư",
		"cá", "nhà", "gì", "quá", "cả", "tôi", "chào", "học",
		"a", "Á", "hello",
		"Việt", "Đường",
	}
	for _, w := range words {
		tx := New()
		if !tx.ResumeWord(w) {
			t.Errorf("ResumeWord(%q) refused, want accepted", w)
			continue
		}
		if got := tx.Current(); got != w {
			t.Errorf("ResumeWord(%q): Current() = %q", w, got)
		}
	}
}

func TestResumeWordRefusesNonWords(t *testing.T) {
	words := []string{"", "café2", "naïve", "xin chào", "a1b", "đường."}
	for _, w := range words {
		tx := New()
		if tx.ResumeWord(w) {
			t.Errorf("ResumeWord(%q) accepted, want refused", w)
			continue
		}
		if !tx.Empty() {
			t.Errorf("ResumeWord(%q): engine not empty after refusal", w)
		}
	}
}

// After adoption the word must behave like a live composing word: tone keys
// re-tone it, z strips the tone, w adds a horn mark.
func TestResumeWordThenEdit(t *testing.T) {
	cases := []struct {
		word string
		key  rune
		want string
	}{
		{"viet", 'j', "viẹt"}, // no hat in the adopted word, so none in the result
		{"việt", 'z', "viêt"}, // z strips the tone, keeps the mark
		{"chao", 'f', "chào"},
		{"cá", 'f', "cà"}, // replacing the tone on a bare vowel
		{"tu", 'w', "tư"},
		{"đuong", 'w', "đương"},
	}
	for _, c := range cases {
		tx := New()
		if !tx.ResumeWord(c.word) {
			t.Errorf("ResumeWord(%q) refused", c.word)
			continue
		}
		tx.ProcessChar(c.key)
		if got := tx.Current(); got != c.want {
			t.Errorf("ResumeWord(%q)+%q: Current() = %q, want %q", c.word, c.key, got, c.want)
		}
	}
}

// Adoption policy (AdoptEdit, the guard the frontend ships): the edit is
// applied only when the key transforms the word (not a plain append, not a
// no-op) and the result still spells like a Vietnamese syllable. Anything
// else must be refused so committed text is never rewritten into a mangled
// word.
func TestAdoptionPolicy(t *testing.T) {
	edited := func(word string, key rune) (string, bool) {
		return New().AdoptEdit(word, key)
	}

	accepts := []struct {
		word string
		key  rune
		want string
	}{
		{"viet", 'j', "viẹt"},
		{"chao", 'f', "chào"},
		{"tu", 'w', "tư"},
		{"nguoi", 'f', "nguòi"}, // half-fixed Vietnamese still allowed
	}
	for _, c := range accepts {
		got, ok := edited(c.word, c.key)
		if !ok || got != c.want {
			t.Errorf("%q+%q: got %q ok=%v, want %q accepted", c.word, c.key, got, ok, c.want)
		}
	}

	refusals := []struct {
		word string
		key  rune
	}{
		{"hello", 's'}, // would become "helló" — not Vietnamese
		{"hello", 'w'}, // would become "hellơ"
		{"nam", 'z'},   // no tone to remove — no-op
		{"xyz", 's'},   // consonant cluster, plain append
	}
	for _, c := range refusals {
		if got, ok := edited(c.word, c.key); ok {
			t.Errorf("%q+%q: accepted as %q, want refused", c.word, c.key, got)
		}
	}
}
