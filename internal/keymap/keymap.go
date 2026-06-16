package keymap

// Linux evdev key codes (from <linux/input-event-codes.h>).
const (
	KeyBackspace  = 14
	KeyTab        = 15
	KeyEnter      = 28
	KeyLeftCtrl   = 29
	KeyLeftShift  = 42
	KeyRightShift = 54
	KeyLeftAlt    = 56
	KeySpace      = 57
	KeyCapsLock   = 58
	KeyRightCtrl  = 97
	KeyRightAlt   = 100
	KeyLeftMeta   = 125
	KeyRightMeta  = 126
)

// letterCode maps an evdev key code to its US-layout lowercase letter.
// Only alphabetic keys are relevant for Telex word composition.
var letterCode = map[uint32]rune{
	16: 'q', 17: 'w', 18: 'e', 19: 'r', 20: 't', 21: 'y', 22: 'u', 23: 'i', 24: 'o', 25: 'p',
	30: 'a', 31: 's', 32: 'd', 33: 'f', 34: 'g', 35: 'h', 36: 'j', 37: 'k', 38: 'l',
	44: 'z', 45: 'x', 46: 'c', 47: 'v', 48: 'b', 49: 'n', 50: 'm',
}

// Letter returns the lowercase rune produced by code on a US layout, and
// whether the code corresponds to an alphabetic key.
func Letter(code uint32) (rune, bool) {
	r, ok := letterCode[code]
	return r, ok
}

// IsModifier reports whether code is a modifier key (ctrl/shift/alt/meta/caps).
func IsModifier(code uint32) bool {
	switch code {
	case KeyLeftCtrl, KeyRightCtrl, KeyLeftShift, KeyRightShift,
		KeyLeftAlt, KeyRightAlt, KeyLeftMeta, KeyRightMeta, KeyCapsLock:
		return true
	}
	return false
}
