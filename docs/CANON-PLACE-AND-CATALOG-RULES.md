# Canon Place names & places catalog rules

Public-safe product rules for fleet gas/maintenance places. No account IDs, customer numbers, or personal identifiers.

This file is the **catalog lifecycle + fuel UI + OneStep fill-missing** layer. It extends, and does not replace, the ID grammar in [`docs/PLACE-CODES-readme.md`](PLACE-CODES-readme.md) (composite `7_3_5_1_1`, type/brand dictionaries, SQL sketch). Do not fork a second type or brand table here. Last Reading locks stay in [`docs/collab/OIL-LOCKS.md`](collab/OIL-LOCKS.md); the oil section below only restates place-join locks.

## Canon Place name (`7_3_5_1_1`)

Composite label (human / zone / UI display):

```
GeneralCode_Type_Branding_TopTier_TopTierGrade
     7    _  3 _    5    _    1   _      1
```

Example: `A000001_001_WAWAA_A_A`

| Segment | Length | Alphabet | Role |
|--------|--------|----------|------|
| **GeneralCode** | 7 | `A–Z` `0–9` | Permanent place ID. Search and join on this. Never reuse. |
| **Type** | 3 digits | `000–999` | Kind of place (gas, shop, …). |
| **Branding** | 5 | `A–Z` `0–9` | Current brand code. **Can change** on rebrand. Column: `brand_code`. |
| **TopTier** | 1 | `A` `B` `C` | Top Tier certification status. |
| **TopTierGrade** | 1 | `A` `B` `C` | Base vs Plus strength when known. |

- Mint general codes **sequentially** starting at **`A000001`** (never `A000000`). The PLACE-CODES shape example is grammar-only, not a mint recipe.
- Prefer **uppercase** only.
- Store each segment as its **own column**. The underscore string is a **label**, not the only source of truth.
- Do **not** put region or state into `general_code`.
- Do **not** encode brand into `general_code`.

### Why general code is first

Brand names change. The physical place should not. Prefix search and FKs stay stable after a rebrand; update `brand_code` / Top Tier letters only.

---

## Type codes (starter)

Same codes as PLACE-CODES. Never renumber an existing type.

| code | slug | name |
|------|------|------|
| `001` | `gas` | Gas / fuel station |
| `002` | `shop` | Maintenance / repair shop |
| `003` | `gas_shop` | Fuel + service at same site |
| `004` | `other` | Other place (use sparingly) |

## Brand codes (starter)

Five characters, uppercase. Grow the PLACE-CODES `place_brands` table as banners appear. Examples: `UNKWN`, `SHELL`, `EXXON`, `MOBIL`, `BPUSA`, `CHEVR`, `WAWAA`, `SHETZ`, `CIRCL`, `QTRAC`, …

`UNKWN` = default when merchant text is junk.

## TopTier (`A` / `B` / `C`)

| Code | Meaning |
|------|---------|
| `A` | Don't know / not checked |
| `B` | False — not Top Tier |
| `C` | True — Top Tier |

Lookup: https://www.toptiergas.com/#stations and https://stationfinder.toptiergas.com/

## TopTierGrade (`A` / `B` / `C`)

| Code | Meaning |
|------|---------|
| `A` | Don't know |
| `B` | Basic TOP TIER (not confirmed Plus) |
| `C` | TOP TIER Plus |

Default both TopTier letters to `A` on new places. Do not invent Plus from a generic sticker alone.

---

## Catalog lifecycle

### Initial seed

- Build unique places from fuel DETAILS / merchant address identity (dedupe so the same physical site is not doubled).
- Assign sequential `general_code` values from `A000001` upward.
- Persist segments + composite label in `places` (app DB / local sqlite / warehouse — same grammar).

### Append-only for new stations

When **new** stations appear later:

- **APPEND only** at the bottom of `places`.
- Next `general_code` = **last `A######` + 1** (e.g. after `A000001` → `A000002`).
- **Never reseed** the catalog from DETAILS in a way that renumbers or rewrites existing rows.
- **Never invent** street numbers, lat/lng, or TopTier `C` from brand alone.

### OneStep markers / zones (fill-missing)

- Catalog rows already exist; nightly (or batch) work is **fill-missing only**.
- Create OneStep markers/zones only for rows still missing `onestep_zone_id`.
- OneStep Place **name** = Canon Place name **exactly** (the composite label).
- One place at a time. **No bulk import.**
- Map-search → form-bound marker on real pump canopy / fill pad when possible; address = billing from catalog.
- After SAVE, write `onestep_zone_id` back to the catalog row.
- Two SAVE failures → HOLD and move on. Do not invent addresses to force a SAVE.
- Skip existing HOLDs unless a human clears them.

---

## Fuel / gas-card UI display

- Gas-card / fuel transaction lists should show the **Canon Place name**, not raw merchant text alone.
- Join punches to `places` by stable keys (merchant identity / address match as implemented).
- If unmatched: show a clear unmatched / HOLD fallback — **never invent** a Canon label.

Raw merchant strings are billing noise; the Canon label is the place.

---

## Related oil math (do not violate)

These are adjacent product locks when places join Last Reading work. Full card: [`docs/collab/OIL-LOCKS.md`](collab/OIL-LOCKS.md).

- **Last Reading** = trusted Enterprise fill odometer at a known second + OneStep **drive-stop** miles since that second.
- Prefer `GET /v3/api/public/route/drive-stop` with JWT Bearer and required query: `device_id`, `dt_tracker_from`, `dt_tracker_to`.
- Use `distance` / `drive_stop_list`. **Never** OneStep odometer as Last Reading. **Never invent miles.**
- Empty/missing drive-stop query can return a misleading **403** — that is not "API disabled."
- Pair vehicles by OneStep `factory_id` (hardware); `device_id` for History; `display_name` is never the join key.

---

## Rules of the road (checklist)

1. Never put region/state into `general_code`.
2. Never reuse a `general_code` for a different place.
3. Rebrand → change `brand_code` / Top Tier fields only.
4. Default TopTier letters to `A` until evidenced.
5. Search, FKs, and OneStep place pairing → `general_code`.
6. New stations → append last `A######` + 1 only.
7. Nightly / batch markers → fill-missing only; no catalog reseed.
8. Fuel UI → Canon Place name; no invented labels for unmatched punches.
9. OneStep zones → one at a time; name = Canon label; no bulk.

---

## Suggested schema sketch

Store segments as columns; compute or store `label` as:

```
general_code || '_' || type_code || '_' || brand_code || '_' || toptier || '_' || toptier_grade
```

Nullable `onestep_zone_id` until a zone exists. Full `CREATE TABLE` sketch lives in PLACE-CODES.

---

*Anonymized product rules doc for public GitHub. No customer numbers, mailbox addresses, or personal names.*
