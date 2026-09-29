# borderkit

**Given my papers, what will this border ask for?**

Passport-to-country visa tables exist. What does not exist in the open is the
layer travellers actually get wrong: what a **residence permit** changes, what a
**visa you already hold** covers, whether an **airside connection** needs a
transit visa, how long the passport must stay valid, which countries want a
digital **arrival card** or a **yellow fever** certificate. borderkit is that
layer: a sourced, versioned rules dataset plus a small engine that reasons over
it, with a basis stated for every line.

```
$ bk -passport BD -residence FI:permanent -from FI -to PT -passport-expires 2030-01-01
☐ Travel insurance       No insurance on file for this trip.
✓ Entry                  Your FI residence permit lets you move freely in the Schengen area for up to 90 days in any 180. Carry the permit card and your passport; no visa, no ETIAS.
  Schengen Borders Code art. 6(1)(b); Convention art. 21 https://eur-lex.europa.eu/eli/reg/2016/399/oj
✓ Passport validity      Valid until Jan 1, 2030.
✓ Residence permit       Permanent permit; carry the card with your passport.

$ bk -passport BD -from BD -to US -via DE -passport-expires 2030-01-01
! Entry                  Visa required for BD passports at an embassy or consulate — allow several weeks.
  Passport Index https://github.com/ilyankou/passport-index-dataset  (check)
! Transit via DE         BD passports need an airport transit visa even for an airside connection in DE, unless exempt. …
  airport transit visa list https://eur-lex.europa.eu/eli/reg/2009/810/oj
```

## What is in the box

| Path | What |
|---|---|
| `data/groups.yaml` | Country groupings the engine reasons with: EU, free movement, Schengen, US visa waiver, Schengen airport-transit-visa passports. Each with a source. |
| `data/regions/*.yaml` | Defaults a whole group inherits (Schengen passport rule, permit free movement, transit, ETIAS). |
| `data/countries/XX.yaml` | One file per destination: passport validity, permit holders admitted, arrival formalities, health, transit. Country files override region defaults. |
| `visa/` | The Passport Index matrix (MIT, [ilyankou/passport-index-dataset](https://github.com/ilyankou/passport-index-dataset)) for the plain passport → country rule. |
| `assess.go` | The engine. `Dataset.Assess(traveller, leg, now)` returns requirements, warnings first. |
| `cmd/validate` | Schema, reference and provenance checks. Runs in CI. |
| `cmd/bk` | The CLI above. |

Every rule carries `source`, `verified` and `confidence`. A rule with confidence
`high` is law or an official page we read. `medium` and `low` are official-ish or
secondary sources. `seed` means the rule was migrated from a curated table and
**still needs a source** — the validator lists these, and they are the easiest
first contribution.

## Use it from Go

```go
d, _ := borderkit.Load()
borderkit.Names = myCountryNames // optional, for display
reqs := d.Assess(borderkit.Traveller{
    Nationality: "BD", Residence: "FI", ResidenceStatus: "permanent",
    PassportExpires: &exp,
}, borderkit.Leg{From: "FI", To: "RS", Depart: dep, Return: ret}, time.Now())
for _, r := range reqs {
    fmt.Println(r.Status, r.Title, r.Detail, r.Basis, r.Source, r.Check)
}
```

The engine works in country codes. Resolve airports to countries before calling
it; that keeps airport data out of this repository.

## What the engine reasons about, in order

1. **Entry regime** — home country → free movement → resident → visa held →
   permit rules on the destination (Schengen free movement for permit holders,
   national "we admit Schengen permit holders" rules) → Passport Index.
2. **Electronic authorisations** for visa-exempt visitors (ETIAS, UK ETA).
3. **Passport validity** by regime: valid on the day for citizens, three months
   beyond departure for Schengen visitors, six months for countries that say so,
   and an honest "rule not on file" otherwise.
4. **Residence permit validity** when the trip depends on it (re-entry).
5. **Transit** through each connection country: Annex IV airport transit visas
   with permit and visa exemptions, US (everyone clears immigration), Canada
   (eTA), UK (DATV check).
6. **Insurance** (€30 000 for Schengen visa applicants), **health** (yellow
   fever), **arrival cards**, and what to have ready at the border.

## Try it in the browser

The viewer at **https://wander-kit.github.io/borderkit/** runs the same Go engine
compiled to WebAssembly, entirely in your browser. Pick passport, residence,
destination and connections; every line links its source and has a "this is
wrong" button that opens a pre-filled issue. Build it locally with
`GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o web/borderkit.wasm ./cmd/wasm` and serve `web/`.

The open work is listed in [docs/SEED.md](docs/SEED.md): rules that still need an
official source. Regenerate it with `go run ./cmd/validate -seed-md docs/SEED.md`.

## Contributing

The unit of contribution is one rule with one source. See
[CONTRIBUTING.md](CONTRIBUTING.md). Run `go run ./cmd/validate` before opening a
pull request; CI runs it too.

## Licence

Code: MIT. Data under `data/`: [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
The Passport Index matrix keeps its own MIT licence.

## Status

Early. The dataset seeds from the tables built for
[travel-agent](https://github.com/Wander-Kit) and most country rules are marked
`seed`. Roadmap: sources for every seed rule → TypeScript client → a static web
viewer with a "this is wrong" button → review-date bot.
