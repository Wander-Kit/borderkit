package borderkit

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wander-Kit/borderkit/visa"
)

// Traveller is what we know about the person's papers.
type Traveller struct {
	Nationality     string     // passport country, ISO alpha-2
	PassportExpires *time.Time // nil = unknown
	Residence       string     // country of residence; "" = unknown
	ResidenceStatus string     // "" | citizen | permanent | temporary
	PermitExpires   *time.Time // temporary permits
	Visas           []Paper    // visas held; Country = valid for ("*" = unknown)
	Insurance       []Paper
	Vaccinations    []Paper // Name e.g. "yellow fever"
}

// Paper is a document the traveller holds.
type Paper struct {
	Name    string
	Country string
	Expires *time.Time
}

// Leg is one journey: from a country to a country, possibly via others. Callers
// with airports resolve them to countries first.
type Leg struct {
	From, To string   // ISO alpha-2
	Via      []string // countries of airside connections
	Stops    int      // connections when Via is unknown
	Depart   time.Time
	Return   time.Time // zero = one-way
}

// Requirement is one line the border may ask for.
type Requirement struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	Status string `json:"status"` // ok | todo | warn | info
	Detail string `json:"detail"`
	Basis  string `json:"basis"`  // law, dataset or document behind the line
	Source string `json:"source"` // URL when the rule has one
	Check  bool   `json:"check"`  // confirm with an official source before flying
}

// Names maps country codes to display names; callers may replace it.
var Names = func(cc string) string { return cc }

// HasPermit reports whether the traveller holds a residence permit valid on the date.
func (t Traveller) HasPermit(at time.Time) bool {
	if t.ResidenceStatus == "" || t.ResidenceStatus == "citizen" || t.Residence == "" {
		return false
	}
	if t.ResidenceStatus == "temporary" && t.PermitExpires != nil && t.PermitExpires.Before(at) {
		return false
	}
	return true
}

func (t Traveller) visaFor(country string, until time.Time) *Paper {
	for i := range t.Visas {
		v := &t.Visas[i]
		if v.Country != "*" && !strings.EqualFold(v.Country, country) {
			continue
		}
		if v.Expires == nil || !v.Expires.Before(until) {
			return v
		}
	}
	return nil
}

// permitOpens reports whether a permit rule on the destination is satisfied.
func (d *Dataset) permitOpens(t Traveller, pr PermitRule, until time.Time) bool {
	from := pr.PermitFrom
	inScope := func(cc string) bool { return cc != "" && (cc == from || d.In(from, cc)) }
	for _, a := range pr.Accepts {
		switch a {
		case "permanent", "temporary":
			if t.ResidenceStatus == a && t.HasPermit(until) && inScope(t.Residence) {
				return true
			}
		case "visa":
			for _, v := range t.Visas {
				if inScope(v.Country) && (v.Expires == nil || !v.Expires.Before(until)) {
					return true
				}
			}
		}
	}
	return false
}

// Assess lists what the leg needs, most important first.
func (d *Dataset) Assess(t Traveller, leg Leg, now time.Time) []Requirement {
	dest, origin := strings.ToUpper(leg.To), strings.ToUpper(leg.From)
	c := d.Country(dest)
	end := leg.Return
	if end.IsZero() {
		end = leg.Depart.AddDate(0, 0, 14) // assume a fortnight for validity rules
	}
	free := d.Groups["free_movement"]
	schengen := d.Groups["schengen"]
	var out []Requirement
	add := func(key, title, status, detail string, p Provenance, basis string) {
		out = append(out, Requirement{Key: key, Title: title, Status: status, Detail: detail, Basis: basis, Source: p.Source, Check: p.Source != "" && p.Check() || p.Confidence == "seed"})
	}
	none := Provenance{Confidence: "high"}
	name := Names

	// 1. Entry regime.
	entry := ""
	switch {
	case dest == "":
		add("entry", "Entry", "info", "Destination country unknown; check the rule yourself.", none, "")
	case t.Nationality == "":
		add("entry", "Entry", "todo", "Set your passport country to see the entry rule.", none, "")
	case dest == t.Nationality:
		entry = "home"
		add("entry", "Entry", "ok", fmt.Sprintf("%s is your passport country; enter with your passport (or national ID card).", name(dest)), none, "citizenship")
	case free.Has(t.Nationality) && free.Has(dest):
		entry = "free_movement"
		add("entry", "Entry", "ok", fmt.Sprintf("Free movement: an EU/EEA passport or ID card is enough in %s, no time limit for visits.", name(dest)), free.Provenance, "Directive 2004/38/EC")
	case dest == t.Residence && t.HasPermit(end):
		entry = "resident"
		add("entry", "Entry", "ok", fmt.Sprintf("You live in %s; enter with your residence permit card and passport.", name(dest)), none, "residence permit")
	case t.visaFor(dest, end) != nil:
		entry = "visa_held"
		v := t.visaFor(dest, end)
		exp := ""
		if v.Expires != nil {
			exp = " valid until " + v.Expires.Format("Jan 2, 2006")
		}
		add("entry", "Entry", "ok", fmt.Sprintf("You hold a visa for %s%s (%s).", name(dest), exp, v.Name), none, "your visa document")
	default:
		r := visa.Lookup(t.Nationality, dest)
		var opened *PermitRule
		for i := range c.PermitHolders {
			if d.permitOpens(t, c.PermitHolders[i], end) {
				opened = &c.PermitHolders[i]
				break
			}
		}
		switch {
		case r.Easy():
			entry = "free"
			add("entry", "Entry", "ok", fmt.Sprintf("%s for %s passports.", capital(r.Label()), name(t.Nationality)), Provenance{Source: "https://github.com/ilyankou/passport-index-dataset", Confidence: "medium"}, "Passport Index")
		case opened != nil && schengen.Has(dest) && d.In("schengen", t.Residence):
			entry = "schengen_permit"
			add("entry", "Entry", "ok", fmt.Sprintf("Your %s residence permit lets you move freely in the Schengen area for up to %d days in any %d. Carry the permit card and your passport; no visa, no ETIAS.", name(t.Residence), opened.Days, opened.Per), opened.Provenance, "Schengen Borders Code art. 6(1)(b); Convention art. 21")
		case opened != nil:
			entry = "permit_free"
			stay := ""
			if opened.Days > 0 {
				stay = fmt.Sprintf(" (about %d days)", opened.Days)
			}
			add("entry", "Entry", "ok", fmt.Sprintf("%s admits holders of a valid %s residence permit or visa without a visa%s — otherwise %s for %s passports. Confirm with the embassy before flying.", name(dest), scopeName(opened.PermitFrom), stay, r.Label(), name(t.Nationality)), opened.Provenance, "national rule for permit holders")
		case r.Status == visa.ETA || r.Status == visa.EVisa:
			entry = "online"
			add("entry", "Entry", "todo", fmt.Sprintf("%s for %s passports — apply online before departure.", capital(r.Label()), name(t.Nationality)), Provenance{Source: "https://github.com/ilyankou/passport-index-dataset", Confidence: "medium"}, "Passport Index")
		case r.Status == visa.Required:
			entry = "visa"
			add("entry", "Entry", "warn", fmt.Sprintf("Visa required for %s passports at an embassy or consulate — allow several weeks.", name(t.Nationality)), Provenance{Source: "https://github.com/ilyankou/passport-index-dataset", Confidence: "medium"}, "Passport Index")
		case r.Status == visa.NoAdmission:
			entry = "no"
			add("entry", "Entry", "warn", fmt.Sprintf("%s passports are not admitted to %s.", name(t.Nationality), name(dest)), Provenance{Source: "https://github.com/ilyankou/passport-index-dataset", Confidence: "medium"}, "Passport Index")
		default:
			add("entry", "Entry", "info", fmt.Sprintf("No entry rule on file for %s passports to %s; check with the embassy.", name(t.Nationality), name(dest)), none, "")
		}
	}
	// Electronic travel authorisation for visa-exempt visitors (ETIAS, UK ETA, …).
	if entry == "free" && c.ETA != nil && !free.Has(t.Nationality) {
		add("eta", c.ETA.Name, "info", strings.TrimSpace(c.ETA.Note), c.ETA.Provenance, c.ETA.Name)
	}

	// 2. Passport validity.
	rule := "valid_for_stay"
	var pv Provenance
	if c.PassportValidity != nil {
		rule, pv = c.PassportValidity.Rule, c.PassportValidity.Provenance
	}
	switch {
	case t.PassportExpires == nil:
		add("passport", "Passport validity", "todo", "Add your passport expiry date.", none, "")
	case t.PassportExpires.Before(end):
		add("passport", "Passport validity", "warn", fmt.Sprintf("Your passport expires %s, before the trip ends %s.", t.PassportExpires.Format("Jan 2, 2006"), end.Format("Jan 2")), none, "")
	case entry == "home" || entry == "free_movement":
		add("passport", "Passport validity", "ok", fmt.Sprintf("Valid until %s; it only needs to be valid on the day.", t.PassportExpires.Format("Jan 2, 2006")), none, "citizenship / free movement")
	case entry == "schengen_permit" || entry == "resident":
		add("passport", "Passport validity", "ok", fmt.Sprintf("Valid until %s.", t.PassportExpires.Format("Jan 2, 2006")), none, "")
	case rule == "three_months_after_departure" && t.PassportExpires.Before(end.AddDate(0, 3, 0)):
		add("passport", "Passport validity", "warn", fmt.Sprintf("%s requires three months' validity beyond your departure date; yours expires %s.", name(dest), t.PassportExpires.Format("Jan 2, 2006")), pv, "passport validity rule")
	case rule == "six_months_on_arrival" && t.PassportExpires.Before(leg.Depart.AddDate(0, 6, 0)):
		add("passport", "Passport validity", "warn", fmt.Sprintf("%s requires six months' validity on arrival; yours expires %s.", name(dest), t.PassportExpires.Format("Jan 2, 2006")), pv, "passport validity rule")
	case rule == "six_months_after_departure" && t.PassportExpires.Before(end.AddDate(0, 6, 0)):
		add("passport", "Passport validity", "warn", fmt.Sprintf("%s requires six months' validity beyond your departure; yours expires %s.", name(dest), t.PassportExpires.Format("Jan 2, 2006")), pv, "passport validity rule")
	case rule == "valid_for_stay" && t.PassportExpires.Before(end.AddDate(0, 6, 0)):
		add("passport", "Passport validity", "info", fmt.Sprintf("Valid until %s, under six months after you return; %s's exact rule is not on file, so check it.", t.PassportExpires.Format("Jan 2, 2006"), name(dest)), none, "")
	default:
		add("passport", "Passport validity", "ok", fmt.Sprintf("Valid until %s.", t.PassportExpires.Format("Jan 2, 2006")), pv, "")
	}

	// 3. Residence permit validity when the trip depends on it.
	if entry == "schengen_permit" || entry == "permit_free" || entry == "resident" {
		switch {
		case t.ResidenceStatus == "temporary" && t.PermitExpires == nil:
			add("permit", "Residence permit", "todo", "Add your permit's expiry date so re-entry can be checked.", none, "")
		case t.ResidenceStatus == "temporary" && t.PermitExpires.Before(end.AddDate(0, 0, 1)):
			add("permit", "Residence permit", "warn", fmt.Sprintf("Your permit expires %s; you may not be let back in after the trip without a renewal or a D visa.", t.PermitExpires.Format("Jan 2, 2006")), none, "")
		case t.ResidenceStatus == "temporary":
			add("permit", "Residence permit", "ok", fmt.Sprintf("Valid until %s; carry the card.", t.PermitExpires.Format("Jan 2, 2006")), none, "")
		default:
			add("permit", "Residence permit", "ok", "Permanent permit; carry the card with your passport.", none, "")
		}
	}

	// 4. Transit.
	for _, vc := range leg.Via {
		vc = strings.ToUpper(vc)
		if vc == origin || vc == dest {
			continue
		}
		tc := d.Country(vc)
		tr := tc.Transit
		if tr == nil {
			add("transit:"+vc, "Transit via "+name(vc), "info", fmt.Sprintf("Airside connection in %s; check whether your passport needs a transit visa there.", name(vc)), none, "")
			continue
		}
		exempt := false
		for _, ex := range tr.Exemptions {
			switch ex {
			case "free_movement":
				exempt = exempt || free.Has(t.Nationality)
			case "schengen_permit":
				exempt = exempt || (t.HasPermit(end) && schengen.Has(t.Residence))
			case "schengen_visa":
				for _, v := range t.Visas {
					exempt = exempt || schengen.Has(v.Country)
				}
			case "destination_visa":
				exempt = exempt || t.visaFor(dest, end) != nil
			}
		}
		if exempt {
			continue
		}
		a := tr.AirsideVisaFor
		switch {
		case a == "none":
		case a == "check":
			add("transit:"+vc, "Transit via "+name(vc), "info", strings.TrimSpace(tr.Note), tr.Provenance, "transit rule")
		case strings.HasPrefix(a, "all_except:"):
			g := strings.TrimPrefix(a, "all_except:")
			if d.In(g, t.Nationality) {
				add("transit:"+vc, "Transit via "+name(vc), "todo", fmt.Sprintf("Visa-exempt passport, but %s screens everyone on connection: arrange the electronic authorisation before departure.", name(vc)), tr.Provenance, "transit rule")
			} else {
				add("transit:"+vc, "Transit via "+name(vc), "warn", strings.TrimSpace(tr.Note), tr.Provenance, "transit rule")
			}
		case d.In(a, t.Nationality):
			add("transit:"+vc, "Transit via "+name(vc), "warn", fmt.Sprintf("%s passports need an airport transit visa even for an airside connection in %s, unless exempt. %s", name(t.Nationality), name(vc), strings.TrimSpace(tr.Note)), tr.Provenance, "airport transit visa list")
		default:
			add("transit:"+vc, "Transit via "+name(vc), "info", "Airside connection, no visa needed; if you change terminals or collect bags you enter the country, so the entry rule applies.", tr.Provenance, "transit rule")
		}
	}
	if len(leg.Via) == 0 && leg.Stops > 0 && !free.Has(t.Nationality) {
		add("transit", "Connection", "info", fmt.Sprintf("Your flight connects %d time%s. If the connection is in the US, Canada, the UK or the Schengen area, transit rules for %s passports may apply; check the airport once the ticket is issued.", leg.Stops, plural(leg.Stops), name(t.Nationality)), none, "")
	}

	// 5. Insurance.
	var cover *Paper
	for i := range t.Insurance {
		p := &t.Insurance[i]
		if p.Expires == nil || !p.Expires.Before(end) {
			cover = p
			break
		}
	}
	switch {
	case cover != nil:
		add("insurance", "Travel insurance", "ok", cover.Name, none, "your document")
	case (entry == "visa" || entry == "online") && schengen.Has(dest):
		add("insurance", "Travel insurance", "todo", "A Schengen visa needs medical insurance covering at least €30,000 for the whole stay.", Provenance{Source: "https://eur-lex.europa.eu/eli/reg/2009/810/oj", Confidence: "high"}, "Visa Code art. 15")
	case entry == "visa" || entry == "online":
		add("insurance", "Travel insurance", "todo", "Most visa applications ask for medical cover.", none, "")
	case len(t.Insurance) > 0:
		add("insurance", "Travel insurance", "warn", "Your insurance expires before the trip ends.", none, "")
	default:
		add("insurance", "Travel insurance", "todo", "No insurance on file for this trip.", none, "")
	}

	// 6. Health.
	if c.Health != nil && c.Health.YellowFever == "all" {
		have := false
		for _, v := range t.Vaccinations {
			if strings.Contains(strings.ToLower(v.Name), "yellow") {
				have = true
			}
		}
		if have {
			add("yellow_fever", "Yellow fever certificate", "ok", "Certificate on file.", c.Health.Provenance, "WHO / national rule")
		} else {
			add("yellow_fever", "Yellow fever certificate", "warn", fmt.Sprintf("%s requires proof of yellow fever vaccination from every arriving traveller; get the shot at least 10 days before departure.", name(dest)), c.Health.Provenance, "WHO country requirements")
		}
	}

	// 7. Arrival formalities and what the officer may ask for.
	if c.Arrival != nil && entry != "home" && entry != "resident" && !(c.Arrival.AppliesTo == "visa_exempt" && entry != "free") {
		add("arrival_card", "Arrival formalities", "todo", capital(c.Arrival.Form)+".", c.Arrival.Provenance, "national rule")
	}
	if entry == "free" || entry == "permit_free" || entry == "online" || entry == "visa" {
		add("proof", "At the border", "info", "Have your return or onward ticket, accommodation details and means of support ready to show.", none, "")
	}

	rank := map[string]int{"warn": 0, "todo": 1, "ok": 2, "info": 3}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Status] < rank[out[j].Status] })
	return out
}

func scopeName(s string) string {
	switch s {
	case "schengen":
		return "Schengen"
	case "eu":
		return "EU"
	}
	return Names(s)
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
