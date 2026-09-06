# Mileage measurement and box score

Two related numbers. Do not collapse them. Never invent miles. Never use OneStep odometer.

## Last Reading (sheet / oil due)

Lives only in `internal/oil`. Formula:

**Last Reading** = Enterprise odometer at a known second (Fuel & Charging with hour:minute:second, else shop RO) **plus** OneStep `GET /v3/api/public/route/drive-stop` miles since that second (`device_id`, `dt_tracker_from`, `dt_tracker_to`). Sum `distance` / `drive_stop_list` only.

HOLD `NO_DEVICE` / dead GPS / ambiguous pairing / `NO_DRIVESTOP`. Missing GPS is not zero. `oilchange compute` writes Last Reading; the box score does **not**.

## Box score / variance ledger (gas card move)

Answers: when a **gas card transaction** posts, how far is that punch odometer from maintenance + measured drive-stop, and is `|gap|` going up or down?

| Side | Definition |
|------|------------|
| **Recorded mileage** | The **gas card transaction**: WEX/Enterprise fuel punch odometer + Provider Transaction Time. Not a vague “Enterprise reading.” |
| **Expected** | Last **good maintenance** odometer + OneStep drive-stop miles from that maintenance timestamp to the punch second |

Sign (locked):

- **Overage** = recorded − expected when the card is ahead
- **Shortage** = expected − recorded when the card is behind
- **abs_diff** = `|recorded − expected|`

Suspect / HOLD punches stay visible and are **excluded from trend** until corrected or dismissed.

### What expected is *not*

Last oil + oil interval (`last_oil_miles + interval_miles`) is the Oil Desk **due** number. It is **not** box-score expected. An earlier draft assumed that if “expected” was unclear. Later lock replaced it: expected at a gas card punch is maintenance + drive-stop only.

## Persist

SQLite (`mileage_ledger`, `drive_stop_windows`) is the working store. `oilchange backup-neon` copies those tables to Neon. Do not write invented miles into Neon or Supabase. Vault / desk users stay off Neon.

## UI / CLI

- Desk `/boxscore/` — fleet rollup + per-unit cards + punch table
- `oilchange boxscore [--rebuild] [--efleets-id]`
- Measure one punch is opt-in live drive-stop. Rebuild uses stored windows only.
