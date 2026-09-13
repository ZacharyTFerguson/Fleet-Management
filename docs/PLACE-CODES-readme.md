# Place codes (gas stations & maintenance shops)

Stable IDs for places the fleet visits. Used in the oil app DB, sheets, and (slowly) OneStep zones.

## Composite label

```
GeneralCode_Type_Branding_TopTier_TopTierGrade
     7    _  3 _    5    _    1   _      1
```

Example: `A3K9M2Q_001_SHELL_C_C`

| Segment | Length | Alphabet | Role |
|--------|--------|----------|------|
| **GeneralCode** | 7 | `A–Z` `0–9` | Permanent place ID. Search and join on this. Never reuse. |
| **Type** | 3 digits | `000–999` | Kind of place (gas, shop, …). |
| **Branding** | 5 | `A–Z` `0–9` | Current brand code. **Can change** when a site rebrands. |
| **TopTier** | 1 | `A` `B` `C` | Is the brand Top Tier certified? |
| **TopTierGrade** | 1 | `A` `B` `C` | How strong / which tier grade. |

Capacity: `36^7` general codes is far past one billion. Prefer **uppercase** only so codes never differ by case.

### Why general code is first

Brand names change. The physical place should not. Putting general code first means:

- Prefix search and exact match stay stable after a rebrand.
- You update `branding` (and maybe Top Tier letters) without minting a new place.

Store each segment as its **own column**. The underscore string is a **label** (zone name, human paste). Do not parse the label as the only source of truth.

---

## TopTier (`A` / `B` / `C`)

| Code | Meaning |
|------|---------|
| `A` | Don’t know / not checked |
| `B` | False — not Top Tier |
| `C` | True — Top Tier |

**Lookup:** [TOP TIER™](https://www.toptiergas.com/#stations) and the [Station Finder](https://stationfinder.toptiergas.com/).

- **TopTier** `C` / `B`: station or brand is TOP TIER™ approved (finder / licensed brand list), or clearly not.
- **TopTierGrade** is the Base vs **TOP TIER™+** (Plus) split. Those levels exist on the program ([performance standards](https://www.toptiergas.com/performance-standards/), [TOP TIER™+](https://www.toptiergas.com/top-tier-plus/)).
- The finder is still the best “is this TOP TIER?” tool. Site/app updates to mark **Plus** brands are in progress ([May 2026 note](https://www.toptiergas.com/2026/05/26/top-tier-announces-new-top-tier-standard/)). Until a station shows Plus in the finder, treat Plus as **brand-announced** only.

Default both letters to `A` on new places. Set `toptier_source` to `toptiergas.com` (or `stationfinder.toptiergas.com`) when that was the evidence.

## TopTierGrade (`A` / `B` / `C`)

| Code | Meaning |
|------|---------|
| `A` | Don’t know (TopTier may already be `C`) |
| `B` | Basic — TOP TIER™ (base), not confirmed Plus |
| `C` | Top Tier Plus — TOP TIER™+ |

**Interim Plus brands** (national, announced TOP TIER™+ as of 2026-05-26): Amoco, bp, Costco. If TopTier is `C` and the brand is on that list (or later Plus list / finder Plus flag), set grade `C`. If TopTier is `C` but Plus is not announced for that brand/station, leave grade `A` or set `B` only when you know it’s base-only. Do not invent Plus from a generic TOP TIER sticker alone.

---

## Type codes (starter)

Table: `place_types`

| code | slug | name |
|------|------|------|
| `001` | `gas` | Gas / fuel station |
| `002` | `shop` | Maintenance / repair shop |
| `003` | `gas_shop` | Fuel + service at same site |
| `004` | `other` | Other place (use sparingly) |

Add new types as three-digit codes. Never renumber an existing type.

## Brand codes (starter)

Table: `place_brands` — growing list. Five characters, uppercase.

| code | name | notes |
|------|------|-------|
| `UNKWN` | Unknown brand | Default when merchant name is junk |
| `SHELL` | Shell | |
| `EXXON` | Exxon | |
| `MOBIL` | Mobil | |
| `BPUSA` | BP | |
| `CHEVR` | Chevron | |
| `TEXAC` | Texaco | |
| `CITGO` | Citgo | |
| `SUNOC` | Sunoco | |
| `MARTH` | Marathon | |
| `VALER` | Valero | |
| `SPEED` | Speedway | |
| `CIRCL` | Circle K | |
| `WAWAA` | Wawa | |
| `SHETZ` | Sheetz | |
| `QTRAC` | QuikTrip | |
| `RACET` | RaceTrac | |
| `CASEY` | Casey's | |
| `THINK` | Think / independent | Independent or unclear banner |

When a new banner appears in Fuel DETAILS or a shop RO, **add a row** here, then point the place at that code. Do not encode brand into `general_code`.

---

## Tables

### `place_types`

```sql
CREATE TABLE place_types (
  code CHAR(3) PRIMARY KEY,          -- 001 …
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  notes TEXT
);
```

### `place_brands`

```sql
CREATE TABLE place_brands (
  code CHAR(5) PRIMARY KEY,          -- SHELL …
  name TEXT NOT NULL,
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### `toptier_codes`

```sql
CREATE TABLE toptier_codes (
  code CHAR(1) PRIMARY KEY,          -- A B C
  meaning TEXT NOT NULL
);
-- A Don't know | B False | C True
```

### `toptier_grades`

```sql
CREATE TABLE toptier_grades (
  code CHAR(1) PRIMARY KEY,          -- A B C
  meaning TEXT NOT NULL
);
-- A Don't know | B Basic | C Top Tier Plus
```

### `places`

```sql
CREATE TABLE places (
  general_code CHAR(7) PRIMARY KEY,  -- stable
  type_code CHAR(3) NOT NULL REFERENCES place_types(code),
  brand_code CHAR(5) NOT NULL REFERENCES place_brands(code),
  toptier CHAR(1) NOT NULL DEFAULT 'A' REFERENCES toptier_codes(code),
  toptier_grade CHAR(1) NOT NULL DEFAULT 'A' REFERENCES toptier_grades(code),
  label TEXT GENERATED ALWAYS AS (
    general_code || '_' || type_code || '_' || brand_code || '_' || toptier || '_' || toptier_grade
  ) STORED,
  name TEXT,                         -- local site name if useful
  address TEXT,
  lat DOUBLE PRECISION,
  lng DOUBLE PRECISION,
  merchant_id TEXT,                  -- WEX / Enterprise merchant if known
  onestep_zone_id TEXT,              -- null until zone skill draws it
  toptier_source TEXT,               -- toptiergas.com | manual | unknown | …
  active BOOLEAN NOT NULL DEFAULT true,
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

If your Postgres build dislikes generated columns, compute `label` in the app instead.

---

## OneStep zones (slow)

For each place the fleet actually visits, draw a matching zone in OneStep **over time** (skill owned by One Step Liason). One zone at a time. No bulk import.

- Zone name / label: use the composite `label`.
- Persist `places.onestep_zone_id` when the zone exists.
- Rebrand updates brand letters in DB and (later) zone rename; `general_code` stays.

---

## Rules of the road

1. **Never** put region or state into `general_code`.
2. **Never** reuse a `general_code` for a different place.
3. Rebrand → change `brand_code` (and Top Tier fields if needed) only.
4. Default TopTier and TopTierGrade to `A` until known. Use [toptiergas.com](https://www.toptiergas.com/#stations) / [Station Finder](https://stationfinder.toptiergas.com/) for TopTier. Use announced TOP TIER™+ brands (and later finder Plus flags) for grade `C`; otherwise keep grade `A` (or `B` if base-only is certain).
5. Search, foreign keys, and OneStep pairing of places → `general_code`.
6. Keep this README as the source of the code layout; oil-app migrations should match it.
7. Do not park this data in the XRAY Supabase project. Prefer project `fleet-oil` (Zach Presentation Trial, us-east-2) when that project exists.

---

## Seed snapshot

Same meanings as the tables above. Load order: `place_types` → `place_brands` → `toptier_codes` → `toptier_grades` → `places` (empty at start).

Locked 2026-09-02 with Cheif / Zachary.
