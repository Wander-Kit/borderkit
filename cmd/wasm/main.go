//go:build js && wasm

// Command wasm exposes the engine to the browser for the static viewer:
//
//	borderkit.assess(JSON) → JSON   {traveller, leg} → requirements
//	borderkit.countries()  → JSON   {passports, destinations}
package main

import (
	"encoding/json"
	"sort"
	"syscall/js"
	"time"

	"github.com/Wander-Kit/borderkit"
	"github.com/Wander-Kit/borderkit/visa"
)

type input struct {
	Nationality     string   `json:"nationality"`
	PassportExpires string   `json:"passportExpires"`
	Residence       string   `json:"residence"`
	ResidenceStatus string   `json:"residenceStatus"`
	PermitExpires   string   `json:"permitExpires"`
	Visas           []string `json:"visas"`
	From            string   `json:"from"`
	To              string   `json:"to"`
	Via             []string `json:"via"`
	Depart          string   `json:"depart"`
	Return          string   `json:"return"`
}

func date(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

func main() {
	d, err := borderkit.Load()
	if err != nil {
		panic(err)
	}
	api := js.Global().Get("Object").New()
	api.Set("assess", js.FuncOf(func(_ js.Value, args []js.Value) any {
		var in input
		if err := json.Unmarshal([]byte(args[0].String()), &in); err != nil {
			return `{"error":"bad input"}`
		}
		t := borderkit.Traveller{Nationality: in.Nationality, PassportExpires: date(in.PassportExpires), Residence: in.Residence, ResidenceStatus: in.ResidenceStatus, PermitExpires: date(in.PermitExpires)}
		for _, v := range in.Visas {
			t.Visas = append(t.Visas, borderkit.Paper{Name: v + " visa", Country: v})
		}
		leg := borderkit.Leg{From: in.From, To: in.To, Via: in.Via}
		if dp := date(in.Depart); dp != nil {
			leg.Depart = *dp
		} else {
			leg.Depart = time.Now().AddDate(0, 0, 30)
		}
		if rt := date(in.Return); rt != nil {
			leg.Return = *rt
		}
		out, _ := json.Marshal(d.Assess(t, leg, time.Now()))
		return string(out)
	}))
	api.Set("countries", js.FuncOf(func(js.Value, []js.Value) any {
		dest := map[string]bool{}
		for _, cc := range d.CountryCodes() {
			dest[cc] = true
		}
		for _, p := range visa.Passports() {
			dest[p] = true
		}
		var dests []string
		for cc := range dest {
			dests = append(dests, cc)
		}
		sort.Strings(dests)
		out, _ := json.Marshal(map[string]any{"passports": visa.Passports(), "destinations": dests, "schengen": d.Groups["schengen"].Members})
		return string(out)
	}))
	// The page knows display names (Intl.DisplayNames); the engine only knows codes.
	api.Set("setNames", js.FuncOf(func(_ js.Value, args []js.Value) any {
		var m map[string]string
		if err := json.Unmarshal([]byte(args[0].String()), &m); err != nil {
			return false
		}
		borderkit.Names = func(cc string) string {
			if n, ok := m[cc]; ok && n != "" {
				return n
			}
			return cc
		}
		return true
	}))
	js.Global().Set("borderkit", api)
	select {}
}
