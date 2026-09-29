// Command bk asks the engine from the terminal.
//
//	bk -passport BD -residence FI:permanent -from FI -to PT
//	bk -passport FI -from FI -to MX -via US -json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Wander-Kit/borderkit"
)

func main() {
	passport := flag.String("passport", "", "passport country (ISO alpha-2)")
	expires := flag.String("passport-expires", "", "YYYY-MM-DD")
	residence := flag.String("residence", "", "CC[:citizen|permanent|temporary][:YYYY-MM-DD]")
	visas := flag.String("visas", "", "comma-separated country codes of visas held")
	from := flag.String("from", "", "origin country")
	to := flag.String("to", "", "destination country")
	via := flag.String("via", "", "comma-separated connection countries")
	depart := flag.String("depart", "", "YYYY-MM-DD (default: in 30 days)")
	ret := flag.String("return", "", "YYYY-MM-DD (default: depart + 7)")
	asJSON := flag.Bool("json", false, "print JSON")
	flag.Parse()
	if *passport == "" || *to == "" {
		flag.Usage()
		os.Exit(2)
	}
	d, err := borderkit.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	t := borderkit.Traveller{Nationality: strings.ToUpper(*passport)}
	if *expires != "" {
		x, _ := time.Parse("2006-01-02", *expires)
		t.PassportExpires = &x
	}
	if *residence != "" {
		parts := strings.Split(*residence, ":")
		t.Residence = strings.ToUpper(parts[0])
		t.ResidenceStatus = "permanent"
		if len(parts) > 1 {
			t.ResidenceStatus = parts[1]
		}
		if len(parts) > 2 {
			x, _ := time.Parse("2006-01-02", parts[2])
			t.PermitExpires = &x
		}
	}
	for _, v := range strings.Split(*visas, ",") {
		if v = strings.TrimSpace(strings.ToUpper(v)); v != "" {
			t.Visas = append(t.Visas, borderkit.Paper{Name: v + " visa", Country: v})
		}
	}
	now := time.Now()
	dep := now.AddDate(0, 0, 30)
	if *depart != "" {
		dep, _ = time.Parse("2006-01-02", *depart)
	}
	rt := dep.AddDate(0, 0, 7)
	if *ret != "" {
		rt, _ = time.Parse("2006-01-02", *ret)
	}
	leg := borderkit.Leg{From: strings.ToUpper(*from), To: strings.ToUpper(*to), Depart: dep, Return: rt}
	for _, v := range strings.Split(*via, ",") {
		if v = strings.TrimSpace(v); v != "" {
			leg.Via = append(leg.Via, strings.ToUpper(v))
		}
	}
	rs := d.Assess(t, leg, now)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rs)
		return
	}
	mark := map[string]string{"ok": "✓", "todo": "☐", "warn": "!", "info": "·"}
	for _, r := range rs {
		check := ""
		if r.Check {
			check = "  (check)"
		}
		fmt.Printf("%s %-22s %s\n", mark[r.Status], r.Title, r.Detail)
		if r.Basis != "" || r.Source != "" {
			fmt.Printf("  %s %s%s\n", strings.TrimSpace(r.Basis), r.Source, check)
		}
	}
}
