package visa

import "testing"

func TestLookup(t *testing.T) {
	cases := []struct {
		passport, dest string
		status         Status
		easy           bool
	}{
		{"FI", "LV", Free, true},
		{"FI", "FI", Same, true},
		{"FI", "US", ETA, false},
		{"BD", "LV", Required, false},
		{"BD", "BD", Same, true},
		{"XX", "LV", Unknown, false},
		{"fi", "lv", Free, true}, // case-insensitive
	}
	for _, c := range cases {
		r := Lookup(c.passport, c.dest)
		if r.Status != c.status || r.Easy() != c.easy {
			t.Errorf("%s→%s: got %+v easy=%v, want %s easy=%v", c.passport, c.dest, r, r.Easy(), c.status, c.easy)
		}
	}
	if r := Lookup("FI", "TH"); r.Status != Free || r.Days == 0 {
		t.Errorf("FI→TH should be visa-free with a stated stay length, got %+v", r)
	}
	if got := Lookup("FI", "TH").Label(); got == "" || got == "visa-free" {
		t.Errorf("label should include the days: %q", got)
	}
	if n := len(Passports()); n < 150 {
		t.Errorf("dataset looks truncated: %d passports", n)
	}
}
