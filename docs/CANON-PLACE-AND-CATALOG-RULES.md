# Canon Place names & places catalog rules

Public-safe product rules for fleet gas and maintenance places. No account IDs, customer numbers, mailboxes, personal names, or internal project IDs.

This file is a companion to `docs/PLACE-CODES-readme.md` (segment grammar, type/brand tables, SQL shape). That README is the code-layout source. This file owns catalog lifecycle, how the Canon label is shown, and the oil / OneStep locks that touch places.

Nothing below invents miles or new place codes. Examples are labels already used in the public place-code docs or in the ask that added this file.

## Canon Place name (`7_3_5_1_1`)

Composite label (human / zone / UI display):

```
GeneralCode_Type_Branding_TopTier_TopTierGrade
7    _  3 _    5    _    1   _      1
```

Examples already in circulation (not newly minted):

- `A000001_001_WAWAA_A_A`
- `A3K9M2Q_001_SHELL_C_C`

| Segment | Length | Role |
|--------|--------|------|
| **GeneralCode** | 7 | Permanent place ID. Search and join on this. Never reuse. Never encode region or state. |
| **Type** | 3 digits | Kind of place. Starter set in PLACE-CODES: `001` gas, `002` shop, `003` gas_shop, `004` other. Never renumber an existing type. |
| **Branding** | 5 | Current brand code (`WAWAA`, `SHELL`, `UNKWN`, …). **Can change** on rebrand. |
| **TopTier** | 1 | `A` unknown / `B` not Top Tier / `C` Top Tier. Default `A` until checked. |
| **TopTierGrade** | 1 | `A` unknown / `B` base Top Tier / `C` Top Tier Plus. Default `A` until checked. |

Store each segment as its own column. The underscore string is a **label**. Do not parse the label as the only source of truth.

## Catalog lifecycle

1. The places catalog is **already seeded**. Do not treat an empty `places` table as the live start state.
2. Nightly work is **fill-missing only**: create OneStep markers/zones for catalog rows that still have a null `onestep_zone_id`.
3. Zone / marker name = that row’s existing Canon Place name **exactly**.
4. **Do not reseed** from fuel DETAILS. **Do not mint** new codes for existing rows. **Do not renumber** existing rows.
5. Later new gas stations: **APPEND only** at the bottom of the catalog. Next `general_code` = last `A######` + 1. Locked increment example (not a newly minted code): after `A002275` → `A002276`.
6. Do not dump the seeded catalog onto this public repo.

**Contradiction to leave marked:** PLACE-CODES seed snapshot says `places` starts empty. The catalog lock says the table is already seeded. Seeded catalog wins. Do not empty-reseed to match the older seed note.

## Fuel UI Canon display

When a catalog place is shown to a human (fuel UI, zone name, paste label), show the composite Canon label.

- Columns remain the source of truth. The UI does not parse or rewrite the label to invent segments.
- Rebrand updates branding (and Top Tier letters if needed). `general_code` does not change.
- Oil Desk today still shows raw merchant / station strings on fills. That is not a license to invent a new place-code widget, miles, or codes. When a catalog place is wired in, the display string is the Canon label.

## OneStep and Last Reading locks

These locks apply wherever places meet oil math or GPS:

- Last Reading is Enterprise odometer at a trusted fill or shop second, plus OneStep **drive-stop miles since that second**. Formula lives only in `internal/oil`.
- **Never** use OneStep odometer / device mileage as Last Reading.
- **Never invent miles.** A missing GPS sum is a HOLD, not zero, unless drive-stop returned a measured empty trip list (that measured 0 is stored on purpose).
- HOLD skips the Last Reading write. Do not seed a number to clear a HOLD.
- Last Reading and HOLD **never join on** `onestep_zone_id`.
- Compute does **not** write `onestep_zone_id`. Zone fill is a later, one-zone-at-a-time job. Do not invent zone API field names.
- Cars join GPS boxes on `factory_id`. `device_id` is History / drive-stop identity. `display_name` is never a join key.

## Anonymity

Keep this file public-safe:

- No customer numbers, account IDs, or mailboxes.
- No personal names.
- No internal project IDs or hosting refs.
- No live catalog dump.
