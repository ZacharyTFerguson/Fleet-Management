# Car codes (fleet vehicles)

Stable IDs for PDI fleet vehicles. Parallel to [PLACE-CODES-readme.md](./PLACE-CODES-readme.md). Same `7_3_5_1_1` shape so humans and tools learn one pattern.

## Composite label

```
GeneralCode_Type_Make_Fuel_Status
     7    _  3 _  5  _  1  _   1
```

Example: `K7M2P9Q_001_FORDX_B_C`

| Segment | Length | Alphabet | Role |
|--------|--------|----------|------|
| **GeneralCode** | 7 | `A–Z` `0–9` | Permanent vehicle ID. Search and join in *our* systems. Never reuse. **No state or region in this code.** |
| **Type** | 3 digits | `000–999` | Body / role class (van, pickup, …). |
| **Make** | 5 | `A–Z` `0–9` | OEM make code. Can change only if the physical unit is truly re-badged (rare). |
| **Fuel** | 1 | `A` `B` `C` | Fuel kind. |
| **Status** | 1 | `A` `B` `C` | In-service flag. |

Prefer **uppercase** only. Capacity of the general code is the same as places (`36^7`).

### Why general code is first

Nicknames (`VA10`), plates, and even Enterprise company-vehicle numbers can get messy. The general code is the durable key *we* assign. Enterprise **eFleets ID** stays the join key into eFleets dumps; it is stored beside the code, not inside it.

Legacy opaque ids like `PDI-0042` map into `general_code` (or sit in `legacy_pdi_id`) — do not put `VA` / state into the general code.

Store each segment as its **own column**. The underscore string is a **label** (UI, OneStep display, sheets). Do not parse the label as the only source of truth.

---

## Fuel (`A` / `B` / `C`)

| Code | Meaning |
|------|---------|
| `A` | Don’t know |
| `B` | Gasoline |
| `C` | Diesel |

Default new cars to `A` until confirmed. Add more letters later only with a README bump (do not overload `C`).

## Status (`A` / `B` / `C`)

| Code | Meaning |
|------|---------|
| `A` | Don’t know |
| `B` | Out of service / parked / sold pending |
| `C` | In service |

Default active fleet units to `C` when you know they’re rolling; otherwise `A`.

---

## Type codes (starter)

Table: `car_types`

| code | slug | name |
|------|------|------|
| `001` | `cargo_van  | Cargo van |
| `002` | `passenger` | Passenger / car |
| `003` | `pickup` | Pickup truck |
| `004` | `suv` | SUV / crossover |
| `005` | `other` | Other (use sparingly) |

Never renumber an existing type.

## Make codes (starter)

Table: `car_makes` — growing list. Five characters, uppercase.

| code | name |
|------|------|
| `UNKWN` | Unknown |
| `FORDX` | Ford |
| `CHEVY` | Chevrolet |
| `RAMUS` | Ram |
| `GMCUS` | GMC |
| `TOYOT` | Toyota |
| `NISSA` | Nissan |
| `HONDA` | Honda |
| `DODGE` | Dodge |
| `MERCE` | Mercedes-Benz |
| `OTHER` | Other OEM |

When a new OEM shows up on an RO or eFleets, add a row, then point the car at that code.

---

## Tables

### `car_types` / `car_makes` / `fuel_codes` / `status_codes`

Same idea as place lookup tables: small code → meaning.

### `cars` (coding columns; oil fields live elsewhere)

```sql
CREATE TABLE cars (
  general_code CHAR(7) PRIMARY KEY,
  type_code CHAR(3) NOT NULL REFERENCES car_types(code),
  make_code CHAR(5) NOT NULL REFERENCES car_makes(code),
  fuel CHAR(1) NOT NULL DEFAULT 'A' REFERENCES fuel_codes(code),
  status CHAR(1) NOT NULL DEFAULT 'A' REFERENCES status_codes(code),
  label TEXT,                        -- or GENERATED: 7_3_5_1_1
  efleets_id TEXT UNIQUE,            -- Enterprise join key
  legacy_pdi_id TEXT,                -- e.g. PDI-0042 if migrating
  nickname TEXT,                     -- VA10, WNY1 — display only, never join
  plate TEXT,
  vin TEXT,
  region TEXT,                       -- attribute; NOT in general_code
  -- oil / OneStep columns live elsewhere
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

## Rules of the road

1. **Never** put state or region into `general_code`.
2. **Never** reuse a `general_code` for a different vehicle.
3. Match Enterprise data by **`efleets_id`**, not nickname, not plate alone.
4. OneStep join stays **`factory_id`** (hardware); car general code is our fleet key, not the GPS join.
5. Nickname can change anytime; general code does not.
6. Keep this README aligned with PLACE-CODES (`7_3_5_1_1` family).
7. Do not park this in the XRAY Supabase project. Prefer `fleet-oil` when that project exists.

---

## Relation to place codes

| | Places | Cars |
|--|--------|------|
| Layout | `7_3_5_1_1` | `7_3_5_1_1`|
| Segment 3 | Branding (station banner) | Make (OEM) |
| Segment 4 | TopTier | Fuel |
| Segment 5 | TopTierGrade | Status |

Same length grammar; different dictionaries.

Locked 2026-09-02 with Cheif / Zachary.
