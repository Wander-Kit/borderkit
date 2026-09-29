// Package borderkit answers "given my papers, what will this border ask for?"
//
// It reasons with residence permits, free movement, held visas, the Passport
// Index matrix, transit rules, passport validity regimes, health requirements
// and arrival formalities, and states the basis of every line so the traveller
// can check it. Rules live as YAML under data/ with a source and a verification
// date each; the engine is the same for every country.
package borderkit

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed data/groups.yaml data/regions/*.yaml data/countries/*.yaml
var embedded embed.FS

// Provenance says where a rule comes from and how far to trust it.
type Provenance struct {
	Source     string `yaml:"source"`
	Verified   string `yaml:"verified"`   // YYYY-MM-DD
	Confidence string `yaml:"confidence"` // high | medium | low | seed
	Note       string `yaml:"note"`
}

// Check reports whether the rule should be confirmed by the traveller.
func (p Provenance) Check() bool { return p.Confidence != "high" }

// Group is a set of countries with a reason to exist.
type Group struct {
	Provenance `yaml:",inline"`
	Members    []string `yaml:"members"`
	set        map[string]bool
}

// Has reports membership.
func (g *Group) Has(cc string) bool { return g != nil && g.set[cc] }

// PassportValidity is what the destination wants of the passport's expiry.
type PassportValidity struct {
	Provenance `yaml:",inline"`
	Rule       string `yaml:"rule"` // valid_on_day | valid_for_stay | three_months_after_departure | six_months_on_arrival | six_months_after_departure
}

// PermitRule: a document from elsewhere that opens this border.
type PermitRule struct {
	Provenance `yaml:",inline"`
	PermitFrom string   `yaml:"permit_from"` // group name or country code
	Accepts    []string `yaml:"accepts"`     // permanent | temporary | visa
	Result     string   `yaml:"result"`      // visa_free | on_arrival | e_visa
	Days       int      `yaml:"days"`
	Per        int      `yaml:"per"` // rolling window, e.g. 180
}

// Arrival is a mandatory form or registration.
type Arrival struct {
	Provenance `yaml:",inline"`
	Form       string `yaml:"form"`
	AppliesTo  string `yaml:"applies_to"` // all | visa_exempt
}

// Health lists vaccination requirements.
type Health struct {
	Provenance  `yaml:",inline"`
	YellowFever string `yaml:"yellow_fever"` // all | none | from_endemic
}

// Transit describes airside connections through the country's airports.
type Transit struct {
	Provenance     `yaml:",inline"`
	AirsideVisaFor string   `yaml:"airside_visa_for"` // group name | all_except:<group> | check | none
	Exemptions     []string `yaml:"exemptions"`       // schengen_permit | schengen_visa | free_movement | destination_visa
}

// ETA is an electronic travel authorisation for visa-exempt visitors.
type ETA struct {
	Provenance `yaml:",inline"`
	Name       string `yaml:"name"`
	AppliesTo  string `yaml:"applies_to"`
	From       string `yaml:"from"`
}

// Country is one destination's rules; nil sections mean "nothing special known".
type Country struct {
	Country          string            `yaml:"country"`
	Name             string            `yaml:"name"`
	Region           string            `yaml:"region"`
	PassportValidity *PassportValidity `yaml:"passport_validity"`
	PermitHolders    []PermitRule      `yaml:"permit_holders"`
	Arrival          *Arrival          `yaml:"arrival"`
	Health           *Health           `yaml:"health"`
	Transit          *Transit          `yaml:"transit"`
	ETA              *ETA              `yaml:"eta"`
}

// Dataset is everything loaded from data/.
type Dataset struct {
	Groups    map[string]*Group
	Regions   map[string]*Country // defaults inherited by group members
	Countries map[string]*Country // after inheritance
	raw       map[string]*Country // as written, for validation
}

// Load reads the embedded dataset.
func Load() (*Dataset, error) { return LoadFS(embedded, "data") }

// LoadFS reads a dataset from any filesystem (tests, forks, a checkout).
func LoadFS(fsys fs.FS, root string) (*Dataset, error) {
	d := &Dataset{Groups: map[string]*Group{}, Regions: map[string]*Country{}, Countries: map[string]*Country{}, raw: map[string]*Country{}}
	b, err := fs.ReadFile(fsys, path.Join(root, "groups.yaml"))
	if err != nil {
		return nil, fmt.Errorf("groups: %w", err)
	}
	var gf struct {
		Groups map[string]*Group `yaml:"groups"`
	}
	if err := yaml.Unmarshal(b, &gf); err != nil {
		return nil, fmt.Errorf("groups.yaml: %w", err)
	}
	for name, g := range gf.Groups {
		g.set = map[string]bool{}
		for _, m := range g.Members {
			g.set[m] = true
		}
		d.Groups[name] = g
	}
	regions, _ := fs.ReadDir(fsys, path.Join(root, "regions"))
	for _, e := range regions {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		c, err := readCountry(fsys, path.Join(root, "regions", e.Name()))
		if err != nil {
			return nil, err
		}
		if c.Region == "" {
			return nil, fmt.Errorf("regions/%s: missing region", e.Name())
		}
		if _, ok := d.Groups[c.Region]; !ok {
			return nil, fmt.Errorf("regions/%s: region %q is not a group", e.Name(), c.Region)
		}
		d.Regions[c.Region] = c
	}
	files, err := fs.ReadDir(fsys, path.Join(root, "countries"))
	if err != nil {
		return nil, fmt.Errorf("countries: %w", err)
	}
	for _, e := range files {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		c, err := readCountry(fsys, path.Join(root, "countries", e.Name()))
		if err != nil {
			return nil, err
		}
		if c.Country == "" || strings.TrimSuffix(e.Name(), ".yaml") != c.Country {
			return nil, fmt.Errorf("countries/%s: file name must match country code %q", e.Name(), c.Country)
		}
		d.raw[c.Country] = c
	}
	// Inheritance: every member of a region group gets the region's sections unless it sets its own.
	for name, region := range d.Regions {
		for cc := range d.Groups[name].set {
			c := d.Countries[cc]
			if c == nil {
				if r := d.raw[cc]; r != nil {
					cp := *r
					c = &cp
				} else {
					c = &Country{Country: cc}
				}
				d.Countries[cc] = c
			}
			if c.PassportValidity == nil {
				c.PassportValidity = region.PassportValidity
			}
			if len(c.PermitHolders) == 0 {
				c.PermitHolders = region.PermitHolders
			}
			if c.Transit == nil {
				c.Transit = region.Transit
			}
			if c.ETA == nil {
				c.ETA = region.ETA
			}
			if c.Region == "" {
				c.Region = name
			}
		}
	}
	for cc, r := range d.raw {
		if _, ok := d.Countries[cc]; !ok {
			cp := *r
			d.Countries[cc] = &cp
		}
	}
	return d, nil
}

func readCountry(fsys fs.FS, p string) (*Country, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, err
	}
	var c Country
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &c, nil
}

// Country returns the merged rules for a destination (never nil).
func (d *Dataset) Country(cc string) *Country {
	if c := d.Countries[cc]; c != nil {
		return c
	}
	return &Country{Country: cc}
}

// In reports whether cc belongs to the named group.
func (d *Dataset) In(group, cc string) bool { return d.Groups[group].Has(cc) }

// CountryCodes lists every country with a file or a region, sorted.
func (d *Dataset) CountryCodes() []string {
	out := make([]string, 0, len(d.Countries))
	for cc := range d.Countries {
		out = append(out, cc)
	}
	sort.Strings(out)
	return out
}

// Validate checks the dataset's shape and provenance and returns one message
// per problem: errors first ("E "), then warnings ("W ").
func (d *Dataset) Validate() []string {
	var errs, warns []string
	e := func(f string, a ...any) { errs = append(errs, "E "+fmt.Sprintf(f, a...)) }
	w := func(f string, a ...any) { warns = append(warns, "W "+fmt.Sprintf(f, a...)) }
	for name, g := range d.Groups {
		if len(g.Members) == 0 {
			e("group %s has no members", name)
		}
		if g.Source == "" {
			e("group %s has no source", name)
		}
	}
	check := func(where string, p Provenance) {
		switch p.Confidence {
		case "high", "medium", "low":
			if p.Source == "" {
				e("%s: confidence %s but no source", where, p.Confidence)
			}
			if p.Verified == "" {
				e("%s: no verified date", where)
			}
		case "seed":
			w("%s: needs an official source (confidence seed)", where)
		default:
			e("%s: confidence must be high, medium, low or seed (got %q)", where, p.Confidence)
		}
	}
	valid := map[string]bool{"valid_on_day": true, "valid_for_stay": true, "three_months_after_departure": true, "six_months_on_arrival": true, "six_months_after_departure": true}
	for cc, c := range d.raw {
		if len(cc) != 2 || strings.ToUpper(cc) != cc {
			e("%s: country code must be two upper-case letters", cc)
		}
		if c.Name == "" {
			e("%s: name is required", cc)
		}
		if c.PassportValidity != nil {
			if !valid[c.PassportValidity.Rule] {
				e("%s: passport_validity.rule %q unknown", cc, c.PassportValidity.Rule)
			}
			check(cc+" passport_validity", c.PassportValidity.Provenance)
		}
		for i, pr := range c.PermitHolders {
			if _, ok := d.Groups[pr.PermitFrom]; !ok && len(pr.PermitFrom) != 2 {
				e("%s: permit_holders[%d].permit_from %q is neither a group nor a country", cc, i, pr.PermitFrom)
			}
			if pr.Result != "visa_free" && pr.Result != "on_arrival" && pr.Result != "e_visa" {
				e("%s: permit_holders[%d].result %q unknown", cc, i, pr.Result)
			}
			check(fmt.Sprintf("%s permit_holders[%d]", cc, i), pr.Provenance)
		}
		if c.Arrival != nil {
			if c.Arrival.AppliesTo != "all" && c.Arrival.AppliesTo != "visa_exempt" {
				e("%s: arrival.applies_to must be all or visa_exempt", cc)
			}
			check(cc+" arrival", c.Arrival.Provenance)
		}
		if c.Health != nil {
			check(cc+" health", c.Health.Provenance)
		}
		if c.Transit != nil {
			a := c.Transit.AirsideVisaFor
			switch {
			case a == "check", a == "none", d.Groups[a] != nil:
			case strings.HasPrefix(a, "all_except:") && d.Groups[strings.TrimPrefix(a, "all_except:")] != nil:
			default:
				e("%s: transit.airside_visa_for %q must be a group, all_except:<group>, check or none", cc, a)
			}
			check(cc+" transit", c.Transit.Provenance)
		}
	}
	for name, r := range d.Regions {
		if r.PassportValidity != nil {
			check("region "+name+" passport_validity", r.PassportValidity.Provenance)
		}
		for i, pr := range r.PermitHolders {
			check(fmt.Sprintf("region %s permit_holders[%d]", name, i), pr.Provenance)
		}
	}
	sort.Strings(errs)
	sort.Strings(warns)
	return append(errs, warns...)
}
