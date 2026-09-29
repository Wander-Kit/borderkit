// Package visa answers "does this passport need a visa there?" from the
// Passport Index dataset (github.com/ilyankou/passport-index-dataset, MIT),
// embedded as a passport × destination matrix of ISO 3166-1 alpha-2 codes.
// Rules are passport-based: residence permits and dual nationality can change
// the answer, so callers should say so next to the badge.
package visa

import (
	_ "embed"
	"encoding/csv"
	"strconv"
	"strings"
	"sync"
)

//go:embed passport-index-matrix-iso2.csv
var matrixCSV string

// Status is the normalized entry requirement.
type Status string

const (
	Unknown     Status = ""
	Same        Status = "home"          // destination is the passport's own country
	Free        Status = "visa_free"     // Days may say for how long (0 = not stated)
	OnArrival   Status = "on_arrival"    // visa issued at the border
	ETA         Status = "eta"           // electronic travel authorisation before departure
	EVisa       Status = "e_visa"        // visa applied for online
	Required    Status = "visa_required" // embassy visa
	NoAdmission Status = "no_admission"
)

// Rule is the entry requirement for one passport at one destination.
type Rule struct {
	Status Status
	Days   int // visa-free stay length when the dataset states one
}

// Easy is true when nothing needs arranging before flying (free, on arrival, or home).
func (r Rule) Easy() bool { return r.Status == Free || r.Status == OnArrival || r.Status == Same }

// Label is the short human form.
func (r Rule) Label() string {
	switch r.Status {
	case Same:
		return "home country"
	case Free:
		if r.Days > 0 {
			return "visa-free, " + strconv.Itoa(r.Days) + " days"
		}
		return "visa-free"
	case OnArrival:
		return "visa on arrival"
	case ETA:
		return "eTA needed"
	case EVisa:
		return "e-visa needed"
	case Required:
		return "visa required"
	case NoAdmission:
		return "no admission"
	}
	return ""
}

var (
	once   sync.Once
	matrix map[string]map[string]Rule
)

func load() {
	matrix = map[string]map[string]Rule{}
	rows, err := csv.NewReader(strings.NewReader(matrixCSV)).ReadAll()
	if err != nil || len(rows) < 2 {
		return
	}
	hdr := rows[0]
	for _, r := range rows[1:] {
		if len(r) != len(hdr) {
			continue
		}
		row := make(map[string]Rule, len(hdr)-1)
		for i := 1; i < len(hdr); i++ {
			row[strings.ToUpper(hdr[i])] = parse(r[i])
		}
		matrix[strings.ToUpper(r[0])] = row
	}
}

func parse(v string) Rule {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "-1":
		return Rule{Status: Same}
	case "visa free":
		return Rule{Status: Free}
	case "visa on arrival":
		return Rule{Status: OnArrival}
	case "eta":
		return Rule{Status: ETA}
	case "e-visa":
		return Rule{Status: EVisa}
	case "visa required":
		return Rule{Status: Required}
	case "no admission":
		return Rule{Status: NoAdmission}
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return Rule{Status: Free, Days: n}
	}
	return Rule{}
}

// Lookup returns the rule for a passport (ISO alpha-2) at a destination country.
// Unknown codes give Status Unknown.
func Lookup(passport, destination string) Rule {
	once.Do(load)
	row, ok := matrix[strings.ToUpper(strings.TrimSpace(passport))]
	if !ok {
		return Rule{}
	}
	return row[strings.ToUpper(strings.TrimSpace(destination))]
}

// Passports lists the passport codes in the dataset, sorted.
func Passports() []string {
	once.Do(load)
	out := make([]string, 0, len(matrix))
	for k := range matrix {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
