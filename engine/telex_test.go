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

// feedTracked feeds letters into tx, applying each diff to buf the way the
// frontend applies it to the application's text.
func feedTracked(tx *Telex, buf []rune, seq string) []rune {
	for _, r := range seq {
		del, ins := tx.ProcessChar(r)
		buf = buf[:len(buf)-del]
		buf = append(buf, []rune(ins)...)
	}
	return buf
}

func TestRawKeys(t *testing.T) {
	tx := New()
	feedTracked(tx, nil, "tieengs")
	if got := tx.RawKeys(); got != "tieengs" {
		t.Errorf("RawKeys() = %q, want %q", got, "tieengs")
	}
	tx.Reset()
	if got := tx.RawKeys(); got != "" {
		t.Errorf("RawKeys() after Reset = %q, want empty", got)
	}
}

func TestRestoreWordContinuesTone(t *testing.T) {
	cases := []struct {
		typed, more, want string
	}{
		{"vi", "f", "vì"},
		{"tieeng", "s", "tiếng"},
		{"Vieet", "j", "Việt"},
	}
	for _, c := range cases {
		tx := New()
		buf := feedTracked(tx, nil, c.typed)
		raw, word := tx.RawKeys(), tx.Current()
		tx.Reset() // word ended by Space; app buffer keeps the word
		if !tx.Empty() {
			t.Fatal("Reset should clear the word")
		}
		if !tx.RestoreWord(raw, word) {
			t.Fatalf("RestoreWord(%q, %q) should succeed", raw, word)
		}
		buf = feedTracked(tx, buf, c.more)
		if got := string(buf); got != c.want {
			t.Errorf("%q + restore + %q => %q, want %q", c.typed, c.more, got, c.want)
		}
	}
}

func TestRestoreWordRefusesUnfaithfulReplay(t *testing.T) {
	// "cass" cancels the tone (screen "cas"); an in-word Backspace leaves
	// "ca" on screen, but the raw keys "cas" replay to "cá". The restore
	// must be refused rather than silently re-adding the diacritic.
	tx := New()
	feedTracked(tx, nil, "cass")
	if _, _, handled := tx.Backspace(); !handled {
		t.Fatal("in-word backspace should be handled")
	}
	raw, word := tx.RawKeys(), tx.Current()
	tx.Reset()
	if tx.RestoreWord(raw, word) {
		t.Fatalf("RestoreWord(%q, %q) should be refused: replay differs from screen", raw, word)
	}
	if !tx.Empty() {
		t.Error("refused restore should leave the engine empty")
	}
}

func TestRestoreWordThenBackspace(t *testing.T) {
	tx := New()
	feedTracked(tx, nil, "vieejt")
	raw, word := tx.RawKeys(), tx.Current()
	tx.Reset()
	if !tx.RestoreWord(raw, word) {
		t.Fatalf("RestoreWord(%q, %q) should succeed", raw, word)
	}
	if _, _, handled := tx.Backspace(); !handled {
		t.Fatal("backspace after restore should be handled")
	}
	if got := tx.Current(); got != "việ" {
		t.Errorf("after restore+backspace got %q, want %q", got, "việ")
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
