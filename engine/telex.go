package engine

import (
	bamboo "github.com/BambooEngine/bamboo-core"
)

// Telex wraps the Bamboo engine for a single in-progress word and tracks the
// text already committed to the application so changes can be applied as a
// minimal (delete, insert) diff. No preedit is used.
type Telex struct {
	eng       bamboo.IEngine
	displayed string
}

func New() *Telex {
	im := bamboo.ParseInputMethod(bamboo.InputMethodDefinitions, "Telex")
	return &Telex{eng: bamboo.NewEngine(im, bamboo.EstdFlags)}
}

// Reset ends the current word. The committed text in the application is left
// untouched.
func (t *Telex) Reset() {
	t.eng.Reset()
	t.displayed = ""
}

// Empty reports whether there is no word currently being composed.
func (t *Telex) Empty() bool {
	return t.displayed == ""
}

// Current returns the full composed word as it should currently be displayed.
// Used by preedit mode, which shows the whole word rather than a (delete,
// insert) diff.
func (t *Telex) Current() string {
	return t.displayed
}

// RawKeys returns the original keystrokes typed for the current word, case
// preserved (e.g. "vieejt" while the screen shows "việt"). Empty when no word
// is being composed. Used to snapshot a word at word-end so it can be
// restored later.
func (t *Telex) RawKeys() string {
	return t.eng.GetProcessedString(bamboo.EnglishMode)
}

// RestoreWord replays raw keystrokes so a previously ended word becomes the
// composing word again. onScreen is the word as the application still shows
// it. Replaying is not always faithful: an in-word Backspace can leave a
// tone-cancel key pair (e.g. "cass" + Backspace shows "ca" but raw "cas"
// replays to "cá"), so if the replay does not reproduce onScreen exactly the
// restore is refused and the engine is left empty.
func (t *Telex) RestoreWord(raw, onScreen string) bool {
	t.eng.Reset()
	t.eng.ProcessString(raw, bamboo.VietnameseMode)
	if t.eng.GetProcessedString(bamboo.VietnameseMode) != onScreen {
		t.eng.Reset()
		t.displayed = ""
		return false
	}
	t.displayed = onScreen
	return true
}

// ResumeWord makes a previously committed word the composing word again when
// only its on-screen form is known (the word near the cursor was committed
// long ago, so its raw keystrokes are gone). The raw keys are derived by
// reversing the Telex transform and then replay-verified via RestoreWord, so
// a word that does not round-trip exactly is refused and the engine stays
// empty.
func (t *Telex) ResumeWord(onScreen string) bool {
	raw := rawFromWord(onScreen)
	if raw == "" {
		return false
	}
	return t.RestoreWord(raw, onScreen)
}

// AdoptEdit resumes a committed word (known only in its on-screen form) and
// applies one Telex key to it, returning the edited word. The edit is
// accepted only when the key actually transforms the word — not a no-op, not
// a plain append — and the result still spells like a Vietnamese syllable,
// so "viet"+j yields "viẹt" but "hello"+s refuses (bamboo would mangle it to
// "helló"). On refusal the engine is reset and the caller should treat the
// key as plain typing. On success the word stays composing so further keys
// keep editing it.
func (t *Telex) AdoptEdit(word string, key rune) (string, bool) {
	if !t.ResumeWord(word) {
		return "", false
	}
	t.ProcessChar(key)
	newWord := t.Current()
	if newWord == "" || newWord == word || newWord == word+string(key) || !t.SpellingValid() {
		t.Reset()
		return "", false
	}
	return newWord, true
}

// SpellingValid reports whether the composing word is (a prefix of) a
// plausible Vietnamese syllable per bamboo's spelling rules. English-looking
// words fail it even after a tone key mangles them ("hello"+s → "helló" is
// invalid), while half-fixed Vietnamese ("viẹt", "nguòi") passes. Used to
// gate cursor-word adoption so non-Vietnamese text is never rewritten.
func (t *Telex) SpellingValid() bool {
	return t.eng.IsValid(false)
}

// ProcessChar feeds one letter into the engine and returns the diff needed to
// turn the currently displayed word into the new one: the number of trailing
// characters (runes) to delete via Backspace and the string to insert
// afterwards.
func (t *Telex) ProcessChar(r rune) (deleteRunes int, insert string) {
	t.eng.ProcessKey(r, bamboo.VietnameseMode)
	return t.diff(t.eng.GetProcessedString(bamboo.VietnameseMode))
}

// Backspace removes the last character of the composing word. The bool result
// reports whether a word was being composed; if false the caller should let
// the application handle the backspace itself.
func (t *Telex) Backspace() (deleteRunes int, insert string, handled bool) {
	if t.displayed == "" {
		return 0, "", false
	}
	t.eng.RemoveLastChar(true)
	next := t.eng.GetProcessedString(bamboo.VietnameseMode)
	d, ins := t.diff(next)
	if next == "" {
		t.eng.Reset()
	}
	return d, ins, true
}

// diff compares the currently displayed word with next and returns how many
// trailing characters to delete and what to insert. Deletion is counted in
// runes because each Backspace removes one character.
func (t *Telex) diff(next string) (int, string) {
	ar := []rune(t.displayed)
	br := []rune(next)
	n := 0
	for n < len(ar) && n < len(br) && ar[n] == br[n] {
		n++
	}
	deleteRunes := len(ar) - n
	insert := string(br[n:])
	t.displayed = next
	return deleteRunes, insert
}
