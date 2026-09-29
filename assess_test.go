package borderkit

import (
	"strings"
	"testing"
	"time"
)

func date(s string) *time.Time { t, _ := time.Parse("2006-01-02", s); return &t }

func find(rs []Requirement, key string) *Requirement {
	for i := range rs {
		if rs[i].Key == key {
			return &rs[i]
		}
	}
	return nil
}

func load(t *testing.T) *Dataset {
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDatasetValidates(t *testing.T) {
	d := load(t)
	for _, m := range d.Validate() {
		if strings.HasPrefix(m, "E ") {
			t.Error(m)
		}
	}
	if !d.In("schengen", "FI") || d.In("schengen", "IE") {
		t.Error("Schengen membership")
	}
	if fi := d.Country("FI"); fi.PassportValidity == nil || fi.PassportValidity.Rule != "three_months_after_departure" || len(fi.PermitHolders) == 0 {
		t.Errorf("Finland inherits Schengen defaults: %+v", fi)
	}
}

func TestBangladeshiWithFinnishPR(t *testing.T) {
	d := load(t)
	tr := Traveller{Nationality: "BD", PassportExpires: date("2030-01-01"), Residence: "FI", ResidenceStatus: "permanent"}
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	dep, ret := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)

	rs := d.Assess(tr, Leg{From: "FI", To: "PT", Depart: dep, Return: ret}, now)
	e := find(rs, "entry")
	if e == nil || e.Status != "ok" || !strings.Contains(e.Detail, "residence permit") || e.Source == "" {
		t.Errorf("Schengen with permit: %+v", e)
	}
	if find(rs, "eta") != nil {
		t.Error("permit holders do not need ETIAS")
	}
	rs = d.Assess(tr, Leg{From: "FI", To: "GB", Depart: dep, Return: ret}, now)
	if e := find(rs, "entry"); e == nil || e.Status != "warn" {
		t.Errorf("BD → GB: %+v", e)
	}
	if find(rs, "arrival_card") != nil {
		t.Error("UK ETA is for visa-exempt visitors only")
	}
	rs = d.Assess(tr, Leg{From: "FI", To: "RS", Depart: dep, Return: ret}, now)
	if e := find(rs, "entry"); e == nil || e.Status != "ok" || !e.Check {
		t.Errorf("BD + FI permit → RS (curated, check): %+v", e)
	}
	tr2 := tr
	tr2.Visas = []Paper{{Name: "UK Standard Visitor visa", Country: "GB", Expires: date("2027-05-01")}}
	if e := find(d.Assess(tr2, Leg{From: "FI", To: "GB", Depart: dep, Return: ret}, now), "entry"); e == nil || e.Status != "ok" {
		t.Errorf("held visa: %+v", e)
	}
	tr2.Visas[0].Expires = date("2026-11-14")
	if e := find(d.Assess(tr2, Leg{From: "FI", To: "GB", Depart: dep, Return: ret}, now), "entry"); e == nil || e.Status != "warn" {
		t.Errorf("visa expiring mid-trip must not count: %+v", e)
	}
}

func TestTransit(t *testing.T) {
	d := load(t)
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	dep := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	bd := Traveller{Nationality: "BD", PassportExpires: date("2030-01-01")}
	if r := find(d.Assess(bd, Leg{From: "BD", To: "US", Via: []string{"DE"}, Depart: dep}, now), "transit:DE"); r == nil || r.Status != "warn" {
		t.Errorf("Annex IV airport transit visa: %+v", r)
	}
	pr := bd
	pr.Residence, pr.ResidenceStatus = "FI", "permanent"
	if r := find(d.Assess(pr, Leg{From: "BD", To: "US", Via: []string{"DE"}, Depart: dep}, now), "transit:DE"); r != nil {
		t.Errorf("permit exempts from ATV: %+v", r)
	}
	if r := find(d.Assess(pr, Leg{From: "FI", To: "MX", Via: []string{"US"}, Depart: dep}, now), "transit:US"); r == nil || r.Status != "warn" {
		t.Errorf("US transit needs a visa regardless of the permit: %+v", r)
	}
	fi := Traveller{Nationality: "FI", PassportExpires: date("2030-01-01")}
	if r := find(d.Assess(fi, Leg{From: "FI", To: "MX", Via: []string{"US"}, Depart: dep}, now), "transit:US"); r == nil || r.Status != "todo" {
		t.Errorf("ESTA: %+v", r)
	}
	if r := find(d.Assess(bd, Leg{From: "BD", To: "GB", Stops: 1, Depart: dep}, now), "transit"); r == nil {
		t.Error("connection hint missing")
	}
	if r := find(d.Assess(fi, Leg{From: "FI", To: "GB", Stops: 1, Depart: dep}, now), "transit"); r != nil {
		t.Error("EU passports get no connection hint")
	}
}

func TestPassportRules(t *testing.T) {
	d := load(t)
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	dep, ret := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	fi := Traveller{Nationality: "FI", PassportExpires: date("2026-12-01")}
	if p := find(d.Assess(fi, Leg{From: "FI", To: "PT", Depart: dep, Return: ret}, now), "passport"); p == nil || p.Status != "ok" {
		t.Errorf("EU citizen in Schengen: %+v", p)
	}
	if p := find(d.Assess(fi, Leg{From: "FI", To: "TH", Depart: dep, Return: ret}, now), "passport"); p == nil || p.Status != "warn" || !strings.Contains(p.Detail, "six months") {
		t.Errorf("Thailand six-month rule: %+v", p)
	}
	us := Traveller{Nationality: "US", PassportExpires: date("2027-01-15")}
	rs := d.Assess(us, Leg{From: "US", To: "FR", Depart: dep, Return: ret}, now)
	if p := find(rs, "passport"); p == nil || p.Status != "warn" || !strings.Contains(p.Source, "eur-lex") {
		t.Errorf("Schengen 3-month rule with source: %+v", p)
	}
	if e := find(rs, "eta"); e == nil || e.Title != "ETIAS" {
		t.Errorf("visa-exempt visitor to Schengen gets the ETIAS note: %+v", e)
	}
}

func TestHealthAndFormalities(t *testing.T) {
	d := load(t)
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	dep := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	fi := Traveller{Nationality: "FI", PassportExpires: date("2030-01-01")}
	if r := find(d.Assess(fi, Leg{From: "FI", To: "GH", Depart: dep}, now), "yellow_fever"); r == nil || r.Status != "warn" {
		t.Errorf("Ghana yellow fever: %+v", r)
	}
	fi.Vaccinations = []Paper{{Name: "Yellow fever certificate"}}
	if r := find(d.Assess(fi, Leg{From: "FI", To: "GH", Depart: dep}, now), "yellow_fever"); r == nil || r.Status != "ok" {
		t.Errorf("with certificate: %+v", r)
	}
	if r := find(d.Assess(fi, Leg{From: "FI", To: "TH", Depart: dep}, now), "arrival_card"); r == nil || !strings.Contains(r.Detail, "TDAC") {
		t.Errorf("Thailand arrival card: %+v", r)
	}
	tmp := Traveller{Nationality: "BD", PassportExpires: date("2030-01-01"), Residence: "FI", ResidenceStatus: "temporary", PermitExpires: date("2026-11-15")}
	rs := d.Assess(tmp, Leg{From: "FI", To: "PT", Depart: dep, Return: dep.AddDate(0, 0, 5)}, now)
	if e := find(rs, "entry"); e == nil || e.Status == "ok" {
		t.Errorf("an expiring permit cannot carry a Schengen trip past its expiry: %+v", e)
	}
	if len(rs) > 1 && rs[0].Status != "warn" {
		t.Errorf("warnings first, got %s", rs[0].Status)
	}
}
