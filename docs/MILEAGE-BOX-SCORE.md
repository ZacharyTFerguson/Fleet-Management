# Mileage measurement and box score

Two related numbers. Do not collapse them. Never invent miles. Never use OneStep odometer. Never use last oil + oil interval as box-score expected.

## Last Reading (sheet / oil due)

Lives only in `internal/oil`. Formula:

**Last Reading** = Enterprise odometer at a known second (Fuel & Charging with hour:minute:second, else shop RO) **plus** OneStep `GET /v3/api/public/route/drive-stop` miles since that second (`device_id`, `dt_tracker_from`, `dt_tracker_to`). Sum `distance` / `drive_stop_list` only.

HOLD `NO_DEVICE` / dead GPS / ambiguous pairing / `NO_DRIVESTOP`. Missing GPS is not zero. `oilchange compute` writes Last Reading; the box score does **not**.

## Expected vs recorded (gas-card move)

When a **gas card / fuel punch** is recorded:

1. **Expected mileage** = last **good maintenance record** odometer (trusted shop/RO or validated maintenance) **+** OneStep drive-stop miles since that maintenance timestamp (JWT + `device_id` + `dt_from`/`dt_to`). Never invent miles; never OneStep odo as the base.
2. **Recorded mileage** = what the **gas card / Enterprise fuel punch** reported at that fill (Provider Transaction Time second).
3. **Difference** = recorded − expected (locked sign). Card ahead → positive **overage**. Card behind → negative difference / **shortage**. Also store `abs_diff` = `|difference|`.
4. **Box score / ledger**: for each vehicle over time, show whether that `|gap|` is **going up or down** (gap growing = getting worse vs maintenance+GPS truth; gap shrinking = improving). Track overage vs shortage relative to expected, and a running series of `abs_diff` so trends are visible.

The box score specifically answers: “when the gas card moves, how far is the card odo from maintenance+OneStep expectation, and is that error trending up or down?”

### HOLD / skip

HOLD (visible, not scored) when the maintenance base or OneStep pairing is missing. Do not invent expected. Do not treat missing drive-stop as `maint + 0`. Last oil + oil interval (`last_oil_miles + interval_miles`) is the Oil Desk **due** number only.

Suspect / unusual-Y punches stay visible and are **excluded from trend** until corrected or dismissed.

## Persist

SQLite (`mileage_ledger`, `drive_stop_windows`) is the working store. `oilchange backup-neon` copies those tables to Neon. Do not write invented miles into Neon or Supabase. Vault / desk users stay off Neon.

## UI / CLI

- Desk `/boxscore/` — fleet rollup + per-unit cards + punch table + `|gap|` series
- `oilchange boxscore [--rebuild] [--efleets-id]`
- Measure one punch is opt-in live drive-stop. Rebuild uses stored windows only.

How each unit card is assembled (inputs, trusted vs HOLD, `abs_diff` series, rebuild): [`docs/MILEAGE-BOX-SCORE-CARDS.md`](MILEAGE-BOX-SCORE-CARDS.md).
