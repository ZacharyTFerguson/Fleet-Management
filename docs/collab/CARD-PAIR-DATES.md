# Card↔car pair dates (Composer contract)

Oilchange persists **when a gas card sat in which car** on `card_eras`. These columns are evidence from **swipe times + GPS exclusive pump sits** (3/5/10-station ladder) and maintenance-location pairing — never Enterprise last-write-wins and never Last Reading.

## Columns (`card_eras`)

| Column | Meaning |
|--------|---------|
| `from_at` / `to_at` | Earliest and latest swipe attributed to this era (backprop included). |
| `pair_started_at` | First evidence this card was in this car (usually equals `from_at`). |
| `switched_at` | When the **next** car takes over: first GPS anchor of the following car era. `NULL` if current. |
| `next_pair_at` | Start of the following car era for the same card (= `switched_at` when another car follows). `NULL` if current. |

Person and office eras keep `switched_at` / `next_pair_at` null.

## How to fill (Grok audit)

1. **GPS 3-station ladder** (`cards ladder`, `cards rebuild`): exclusive pump sits name the card while a `factory_id`-linked box is stopped. Anchor times come from the first `gps-stop` call per car segment; backprop extends earlier swipes but does not move the switch anchor.
2. **Split cards** (`CARD-MIX-99` on `27VA15` then `27VA19`): two car eras on one card. VA15 `switched_at` = first VA19 exclusive sit; VA19 `switched_at` / `next_pair_at` stay null.
3. **Maintenance / nearby** (`cards nearby --persist`): certain linked-car eras may be preserved when ladder has not run; pair dates should still reflect GPS or maintenance-location evidence, not Enterprise `recorded_efleets_id`.
4. **Never** join on `display_name`. GPS boxes join on `factory_id`; VIN rematch uses exact 17-char OBD VIN = `cars.vin`.
5. **Owner assignment outranks GPS**: flag disagreement; do not auto-move owner `Assigned*` or write Last Reading from card pair dates.

## CLI

- `oilchange cards split` — prints `pair_started`, `switched`, `next_pair` per era.
- `oilchange cards ladder` — rung summary plus era lines with pair dates.
- `oilchange cards rebuild` — persists eras via `ReplaceEras` after ladder + backprop.

## Tests

- `internal/cards/pairdates_test.go` — anchor/switch math for VA15/VA19 split.
- `internal/cards/eras_test.go` — backprop does not paint later car over earlier window.
- `internal/app/cards_history_test.go` — history pipeline persists `card_eras`.
