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
