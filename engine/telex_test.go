package engine

import "testing"

// applyAll simulates the frontend: feed a sequence of letters and track the
// committed string by applying each (delete, insert) diff, mimicking the
// application's text buffer.
func applyAll(t *testing.T, seq string) string {
	t.Helper()
	tx := New()
	var buf []rune
	for _, r := range seq {
		del, ins := tx.ProcessChar(r)
		if del > 0 {
			buf = buf[:len(buf)-del] // each Backspace removes one rune
		}
		buf = append(buf, []rune(ins)...)
	}
	return string(buf)
}

func TestTelexBasic(t *testing.T) {
	cases := map[string]string{
		"tieesng": "tiếng",
		"vieejt":  "việt",
		"ddafi":   "đài",
		"chuaarn": "chuẩn",
		"mootj":   "một",
		"VIEETJ":  "VIỆT",
		"aw":      "ă",
		"xin":     "xin",
		"chaof":   "chào",
	}
	for in, want := range cases {
		if got := applyAll(t, in); got != want {
			t.Errorf("%q => %q, want %q", in, got, want)
		}
	}
}

func TestBackspace(t *testing.T) {
	tx := New()
	for _, r := range "vieejt" {
		tx.ProcessChar(r)
	}
	if tx.Empty() {
		t.Fatal("word should not be empty")
	}
	// backspace once should remove the last char of "việt" -> "việ"
	del, ins, handled := tx.Backspace()
	if !handled {
		t.Fatal("backspace should be handled")
	}
	_ = del
	_ = ins
	if got := tx.eng.GetProcessedString(1); got != "việ" {
		t.Errorf("after backspace got %q want %q", got, "việ")
	}
}

func TestResetEndsWord(t *testing.T) {
	tx := New()
	tx.ProcessChar('a')
	tx.Reset()
	if !tx.Empty() {
		t.Error("Reset should clear the word")
	}
	if _, _, handled := tx.Backspace(); handled {
		t.Error("backspace after reset should not be handled")
	}
}
