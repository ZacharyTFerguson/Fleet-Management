# VA card ↔ vehicle GPS audit (2026-09-11)

Operator: Grok on Fleet-Management (oilchange). Sibling Composer owns pair-date schema/migration files (`cursor/card-pair-dates-1f1f`); this session did not touch those. Work branch: `cursor/va-card-gps-audit-1f1f`.

SQLite daily driver (`OILCHANGE_DB=./oilchange.sqlite` on this Cloud Agent). Never XRAY. Never `display_name` joins. Last Reading **not written** (`last_reading_miles` still NULL on all 205 cars).

## Coverage (honest)

| Metric | Before this sqlite | After VA GPS audit | 99.5% target |
|---|---|---|---|
| Roster | 0 (empty agent db) | **205** | 205 |
| Device-link (`factory_id` → car) | 0 | **195 / 205 (95.1%)** | 204 / 205 |
| GPS-named **car** card era (known) | 0 | **22 / 205 (10.7%)** | 204 / 205 |
| VA cars on roster | — | **35 / 35** | — |
| VA cars with a GPS box | — | **35 / 35 (100%)** | — |
| VA cars with a GPS-named **car** era | — | **20 / 35 (57%)** | 35 / 35 |

**99.5% known is not reachable on this pod without a full-fleet GPS stop cache or a 260-box nearby `--live` pull.** This run refused that pull. Previous operator desktop (2026-09-04) with `gps-stops.json` of **146,771** visits reached **92 / 205 (44.9%)** known. This agent started with **no cache**. VA-only watched-box drive-stops filled **7,868** visits / **36** boxes / **263** pump clusters.

Device-link ceiling without more VIN / factory_id maps: **195 / 205 = 95.1%** (10 roster cars have no linked box). None of those 10 are VA.

GPS-first on a **VA-only** stop cache will name exclusive sits among those 36 boxes. Other-region cars at the same pump are invisible, so some eras (especially **VA12 / `27SGX4`**, fetched first) are **flagged**, not owner moves.

## What ran

1. Origin/main worktree (Composer WIP left on `cursor/card-pair-dates-1f1f`).
2. Ingest: `testdata/enterprise/fleetsummary_live.csv` + Drive `DETAILS_583424_30-Days (5).csv` (2,076 punches; 73 skipped off-roster). Files stay in gitignored `data/runtime/`.
3. `devices sync --map data/runtime/onestep-map.csv` (Automations sheet factory_id + **one** live `GET /device` list). **264** boxes, **195** distinct roster cars linked. VA1 / VA19 gained boxes from the live list (sheet had been blank).
4. Baseline `cards rebuild --no-gps`: known **0 / 205** (empty GPS cache).
5. **One box first:** `cards watch --card xxxxxxxxxxxxx31443 --live --skip-vin --pace 35s` → VA12 `factory_id=3271035170` / `27SGX4`, certain on **7** exclusive fill days, `min_mi=0.00`.
6. Then `cards watch --virginia --live --skip-vin --fills 10 --pace 35s` (watched VA boxes only, **not** nearby 260). `failed=0`. Hunt print: **certain=6 likely=9 watch=51 cards=29**.
7. `cards rebuild --no-gps` (official pairings + `card_eras`). GPS-first: **220** matches, **23** BEST, **22** known, **138** geocoded stations.

New CLI (this branch, tests in `internal/app/watchloop_test.go`):

- `--virginia` — only Virginia recorded vehicles
- `--skip-vin` — do not AskEmpty `/device` VIN after the GPS loop (do not re-run `devices vin`)

**Footgun:** `cards split` / `cards coverage` / `cards rebuild` **without** `--no-gps` attach OneStep and fleet-fetch every linked box. A split without `--no-gps` was killed after ~26 boxes; cache was not written. Always pass `--no-gps` unless a targeted `--live` watch is intended.

## VA cars used (35)

All have a `factory_id` after live list overlay:

VA1 `27SGXV` … VA35 `2B7P84` (nicknames VA 2–VA 31 / va33 as in Fleet Summary). Map CSV: gitignored `data/runtime/onestep-map.csv` (155 sheet rows; live list filled the rest).

## Cards paired (GPS-named car eras)

Official store path: `cards rebuild --no-gps` → `ReplaceEras`. Control rematch `cards coverage --no-gps`: still **22 / 205** known, Last Reading still **NULL**. **22** roster cars have a GPS-named **car** era. **20** of those are VA.

HOME-agree (GPS-called car = Enterprise Vehicle on ≥1 swipe) — **10** cards, **8** VA:

| Card suffix | GPS = Enterprise | Nickname | Agree n | Stations |
|---|---|---|---|---|
| `…31443` | `27SGX4` | VA12 | 23 | SHEETZ, WAWA, SHELL, BP, VALERO |
| `…18859` | `2B8BFM` | VA 29 | 13 | SHEETZ, SHELL, SUNOCO, BP, CITGO, LOVES |
| `…32367` | `273LCC` | VA 3 | 12 | (then **SPLIT** to VA19) |
| `…93125` | `27SGXP` | VA19 | 9 | SHEETZ, WAWA |
| `…18834` | `2B8C45` | VA 30 | 8 | SHEETZ, ROYAL FARMS, BP, KROGER |
| `…32003` | `285JDJ` | VA16 | 7 | SHEETZ, EXXONMOBIL |
| `…31823` | `285JDG` | VA15 | 1 | SHEETZ Petersburg |
| `…32110` | `285JDF` | VA17 | 1 | EXXONMOBIL |
| `…68799` | `27QSVM` | MD27 | 9 | (1-mile hit in VA watch cache) |
| `…57770` | `292NCX` | PA15 | 5 | SUNOCO, WAWA (1-mile hit) |

```
HOME card=xxxxxxxxxxxxx31443 type=car key=27SGX4 name=VA12
  from=2026-05-18T17:44:00Z to=2026-06-16T19:10:00Z n=23 stations=SHEETZ
```

**Three sampled locations vs all-punch GPS-first:** `scripts/va_card_gps_audit.py` takes three distinct DETAILS pumps per VA card and counts cached boxes within **350 m** of GPS-first geocode (`web/data/cards.json`, gitignored). Only **3 / 35** VA cards had an exclusive sit on those three pumps (most newest fills are two-at-pump or ungeocoded). Official known **22** comes from matching **all** punches, not the three-location sample. Do not persist a 3-location exclusive as an owner join when GPS-first already flagged disagreement.

VA12 / `27SGX4` is the **first** box fetched this wave (allowed one-box live). It sits at pumps used by many cards and therefore appears as a spectator `car` era. Treat those as **cache-order artifacts**, not owner moves. The only HOME match for that box that also matches Enterprise is card `xxxxxxxxxxxxx31443`.

## Switches detected (flag; do not move Last Reading / owner)

GPS disagreement is stored on eras / `gps_called`; it must **not** change owner assignment.

Confirmed GPS **SPLIT** (two car eras on one card):

| Card (suffix) | First car | Next car | Pair dates (From / To evidence) |
|---|---|---|---|
| `…32367` | `273LCC` VA 3 (n=12, from 2026-05-18) | `27SGXP` VA19 (from 2026-06-15) | switched ~2026-06-15; next_pair = VA19 start |

Composer pair-date schema is on **`cursor/card-pair-dates-1f1f`** (`migrations/011_card_era_pair_dates.sql`). This audit branch does **not** merge those Go files. Fill when that migration lands — data-only:

| Card | Era car | `pair_started_at` | `switched_at` / `next_pair_at` |
|---|---|---|---|
| `…32367` | `273LCC` VA 3 | `2026-05-18T20:28:00Z` | `2026-06-15T11:27:00Z` (VA19 first exclusive sit) |
| `…32367` | `27SGXP` VA19 | `2026-06-15T11:27:00Z` | NULL (current) |

3-location 350 m also named `27SGXP` at EXXONMOBIL 1571 MARTINSBURG PIKE, WINCHESTER VA — switch evidence, not a spectator.

Other GPS-vs-Enterprise flags on VA-recorded cards (incomplete fleet sight — do not auto-move):

| Card | Enterprise Vehicle | GPS-named era car(s) |
|---|---|---|
| `…32334` | `27F34H` VA11 | `27SGWW` VA10 |
| `…16065` | `27CKN7` VA 7 | `285JDJ` VA16 |
| `…72266` | `299KRS` VA21 | `27SGXV` VA1 + `285JDJ` VA16 |
| `…72282` | `29BCN5` VA23 | `27SGX4` VA12 + `27SGXP` VA19 |
| `…31575` | `27SGXC` VA18 | `2B8C45` VA 30 |

Many additional SPLIT rows name **VA12** as a spectator exclusive. Treat those as **watch**, not a join, until non-VA boxes sit in the same pump cache.

3-location 350 m also flagged `…80115` Enterprise `292NCV` VA28 vs exclusive `285JDF` VA17 at EXXONMOBIL 115 OTTIS ST, YORKTOWN VA — **flag only**.

## Aliases steered (GPS fuel locations)

GPS-first geocoded **138** stations from exclusive sits (pump clusters=263). DETAILS merchant brands seen on VA punches (steering Gas_Stations labels; no OneStep marker writes):

EXXONMOBIL, SHEETZ, WAWA, SHELL, MARATHON, BP PRODUCTS, SUNOCO, SPEEDWAY LLC, MURPHY USA, CITGO, ROYAL FARMS, UNBRANDED, 76, KROGER.

Maintenance / shop locations: **not** ingested this run (no live Maintenance Detail file-drop; testdata shop ROs are demo `Town, Virginia` only). Do not invent shop aliases.

## Remaining unknown VA cars (no GPS-named era)

15 VA cars still have no **car** era after this cache. Three newest distinct DETAILS pumps (cards have no GPS; exclusive sit among **36** cached boxes only):

| Car | Card | Loc 1 | Loc 2 | Loc 3 |
|---|---|---|---|---|
| VA 4 `276T6T` | `…32714` | EXXONMOBIL 3096 S LYNNHAVEN RD, VIRGINIA BEACH VA | EXXONMOBIL 770 BOUSH ST, NORFOLK VA | EXXONMOBIL 1619 HARDY CASH DR, HAMPTON VA |
| VA 5 `27CKND` | `…31344` | SHELL 6620 BACKLICK RD, SPRINGFIELD VA | SHELL 3043 CENTREVILLE RD, HERNDON VA | EXXONMOBIL 46373 BARTHOLOMEW FA, STERLING VA |
| VA 6 `27CKNQ` | `…31815` | EXXONMOBIL 1300 N GREAT NECK RD, VIRGINIA BEACH VA | SPEEDWAY 2700 YADKIN RD, CHESAPEAKE VA | KROGER 1800 REPUBLIC RD, VIRGINIA BEACH VA |
| VA 8 `27BV75` | `…32722` | WAWA 2610 G WASHINGTON, YORKTOWN VA | WAWA 5824 GEORGE WASHINGT, YORKTOWN VA | WAWA 842 MERRIMAC TRL, WILLIAMSBURG VA |
| VA 9 `27BV76` | `…31096` | EXXONMOBIL 2842 RICHMOND HWY, STAFFORD VA (repeat) | EXXONMOBIL 14675 LEE HWY, GAINESVILLE VA | — |
| VA13 `292ND8` | `…66466` | SHELL 11310 PATTERSON AVE, RICHMOND VA | SHELL 345 W RESERVOIR RD, WOODSTOCK VA | EXXONMOBIL 11113 JAMES MONROE H, CULPEPER VA |
| VA18 `27SGXC` | `…31575` | UNBRANDED 6305 CRAIN HWY, LA PLATA MD | ROYAL FARMS 7401 MOORES RD, BRANDYWINE MD | ROYAL FARMS 30315 THREE NOTCH RD, CHARLOTTE HALL MD |
| VA21 `299KRS` | `…72266` | BP 486 LAUREL HILL RD, VERONA VA | SPEEDWAY 4025 QUARLES CT, HARRISONBURG VA | BP 596 MOUNT HERMON RD, ELKTON VA |
| VA22 `299KRT` | `…72290` | MURPHY USA 423 OAKVILLE RD, APPOMATTOX VA | MARATHON 157 THOMAS JEFFERSON, CHARLOTTE COURT HO VA | SHELL 1602 LONGWOOD AVE, BEDFORD VA |
| VA24 `29BCS5` | `…68807` | EXXONMOBIL 8965 VILLAGE SHOPS D, FAIRFAX STATION VA | SUNOCO 7320 GAMBRILL RD, SPRINGFIELD VA | — |
| VA26 `292NCZ` | `…87900` | WAWA 7316 FOREST HILL AVE, RICHMOND VA | SUNOCO 995 J CLYDE MORRIS B, NEWPORT NEWS VA | SHEETZ 23711 ROGERS CLARK B, RUTHER GLEN VA |
| VA32 `2B7P7R` | `…63750` | CITGO 14 DOCTORS RD, LOUISA VA | SHEETZ 133 BOTANICAL BLVD, WINCHESTER VA | — |
| va33 `2B7P7S` | `…52134` | EXXONMOBIL 22539 TIMBERLAKE RD, LYNCHBURG VA | SUNOCO 11516 LEESBURG PIKE, HERNDON VA | — |
| VA34 `2B7P7T` | `…52126` | EXXONMOBIL 17164 RICHMOND HWY, DUMFRIES VA (2 punches) | — | — |
| VA35 `2B7P84` | — | **0 punches** in the May–Jun 30-day DETAILS | — | — |

PERSON persist skips (driver-kept): `…87900`, `…63750`, `…72274`, `…32714`.

## Next three cards / locations to probe

Do **not** re-run nearby `--live` for 260 boxes. Prefer a 90-day DETAILS drop, or **one** non-VA box at these pumps if a later watch is allowed (`--pace 35s`):

1. **VA 4 / `276T6T` / card `xxxxxxxxxxxxx32714`** — EXXONMOBIL **3096 S LYNNHAVEN RD, VIRGINIA BEACH VA** (2026-06-12 17:45). Hunt had 4 VA spectators, none exclusive. Need a Tidewater non-VA box or more exclusive days.
2. **VA26 / `292NCZ` / card `xxxxxxxxxxxxx87900`** — WAWA **7316 FOREST HILL AVE, RICHMOND VA** (2026-06-15 16:23). PERSON skip on persist; GPS watch list only.
3. **VA22 / `299KRT` / card `xxxxxxxxxxxxx72290`** — MURPHY USA **423 OAKVILLE RD, APPOMATTOX VA** (2026-06-16 12:49). Live hunt called **certain `292ND7` / VA27** (3 exclusive days) vs Enterprise `299KRT`. Flag; do not move owner. Confirm with one more exclusive day or fuel-gauge history (not available).

## Blockers

1. **No inherited `gps-stops.json`** (gitignored; 146k-visit cache lives on the operator desktop, not this pod).
2. **FillsWithFleetSight / exclusive sit:** VA-only cache cannot see NJ/MD/PA boxes at the same pump; false exclusives cluster on early-fetched VA12.
3. **Two-at-pump collisions** stay unnamed (no fuel-level history).
4. **10 cars missing devices** (not VA): `2B7P7Q`, `2B7P82`, `26LT22` BK4, `27BV79` BK7, `29FXDP` NJ9, `27SGXM` NYC-2, `27SW52` PA14, `27P5HZ` PK2, `27F34S` WNY12, `26LSZZ` WNY7-Red. Empty OBD VIN — do not join on `display_name`.
5. Pair-date columns live on Composer branch `cursor/card-pair-dates-1f1f`; this audit did not touch those files. Fill `…32367` from the table above after merge.
6. No 90-day DETAILS / live eFleets DETAILS URL (cust num still missing). May–Jun 30-day window only.
7. Geocoded-station aliases from a VA-only cache sometimes paint VA lat/lng onto PA/MD/FL merchant addresses (e.g. WAWA Reading PA → Baltimore-area coords). Steer Gas_Stations labels from **VA merchant brands + exclusive sits**, not those cross-region geocodes.

## Paths

| What | Path |
|---|---|
| Branch | `cursor/va-card-gps-audit-1f1f` |
| Report | `docs/collab/VA-CARD-GPS-AUDIT.md` |
| Audit script | `scripts/va_card_gps_audit.py` (sqlite + cache; gitignored JSON out) |
| Runtime (gitignored) | `data/runtime/DETAILS_583424_30-Days.csv`, `onestep-map.csv`, `gps-stops.json`, `va-card-gps-audit.json` |
| Watch log | `/opt/cursor/artifacts/va_watch_live.out` |

## Tests

`go test ./internal/app/ -run 'TestCardsWatchVirginia|TestCardsWatchSkipVIN|TestCardsWatchFetchesOnly|TestCardsWatchDoesNotHit'` green. Last Reading untouched after ingest, watch, persist skip, and rebuild.
