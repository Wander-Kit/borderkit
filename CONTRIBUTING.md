# Contributing

Thank you. Most contributions here are ten-minute jobs: one rule, one source.

## Adding or fixing a rule

1. Find or create `data/countries/XX.yaml` (ISO 3166-1 alpha-2, upper case; the
   file name must equal the `country` field).
2. Add or edit the section. Every rule block has four provenance fields:

   ```yaml
   source: https://…          # an official page: ministry, embassy, border agency, EUR-Lex, WHO
   verified: 2026-09-29       # the day you read it
   confidence: high           # high = law or official page; medium = official-ish; low = secondary
   note: one or two sentences a traveller can act on
   ```

3. Run `go run ./cmd/validate`. It fails on missing sources or unknown values and
   lists rules that still carry `confidence: seed`.
4. Open a pull request. Say what changed and paste the sentence from the source
   that supports it.

## Sections a country file may have

| Section | Fields |
|---|---|
| `passport_validity` | `rule`: `valid_on_day` · `valid_for_stay` · `three_months_after_departure` · `six_months_on_arrival` · `six_months_after_departure` |
| `permit_holders[]` | `permit_from` (group name or country), `accepts` (`permanent`, `temporary`, `visa`), `result` (`visa_free`, `on_arrival`, `e_visa`), `days`, `per` |
| `arrival` | `form`, `applies_to` (`all` or `visa_exempt`) |
| `health` | `yellow_fever`: `all` · `none` · `from_endemic` |
| `transit` | `airside_visa_for`: a group, `all_except:<group>`, `check` or `none`; `exemptions`: `schengen_permit`, `schengen_visa`, `free_movement`, `destination_visa` |
| `eta` | `name`, `applies_to`, `from` |

Region defaults live in `data/regions/`; a country file overrides a section by
setting it. Groupings live in `data/groups.yaml` and need a source too.

## What we do not accept

- Rules without a source, unless marked `confidence: seed` and clearly flagged
  in the note as needing one.
- Anything copied from a commercial visa service's terms.
- Passport → country visa rules: those come from the Passport Index dataset;
  fix them upstream.

## Code

Go 1.24. `go test ./...` must pass. Keep the engine generic: if a rule needs a
new field, add the field and its validation rather than a special case.
