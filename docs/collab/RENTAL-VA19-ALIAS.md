# Rental 1 / Rental 2 vs VA19 (one-tx control)

Zachary's identity lock: **Rental 1** and **Rental 2** are OneStep `display_name` labels for **rental cars**. They are people/rental/office buckets. They are **not** VA15/VA19 aliases. Never join GPS on `display_name`. `factory_id` / exact 17-char OBD VIN = `cars.vin` only.

## What maps where

| Label (human) | DETAILS `Provider Company Vehicle Number` | Card (testdata / live) | OneStep `factory_id` | `display_name` | Linked car |
|---|---|---|---|---|---|
| Rental 1 | `Rental 1` | `CARD-RENT-1` / live `xxxxxxxxxxxxx71698` | `351358810724200` | `PDI Rental #1` / testdata `Rental 1` | none (must stay unpaired) |
| Rental 2 | `VA RENTAL 2` | `CARD-RENT-2` / live `xxxxxxxxxxxxx52142` | `350457794656486` | `NYC-6: Julian (PDI Rental #2)` / testdata `Rental 2` | none |
| VA19 | `VA19` | **CARD-19** (one swipe) / live `xxxxxxxxxxxxx93125` on `27SGXP` | `3271045657` | `VA-19: Lindsey Glazier G,V DFR` | `27VA19` testdata / live `27SGXP` |

Live sqlite also has unlinked `Pdi Rental 3` (`351358810743788`) and `Pdi Rental 4` (`351358810740784`). Same rule: rental token on the sticker is not a fleet join.

## The one VA19 transaction

Testdata control is **CARD-19** in `testdata/enterprise/details_wrongcard.csv` and `details_rental.csv`: one DETAILS row recorded on `27VA19` (Marathon, 2026-08-23). That fill is the VA19 car-era control.

`CARD-MIX-99` may also show a `27VA19` **GPS** era after VA15 (split card). That is not a rental alias and is not extra CARD-19 dumps. Enterprise Vehicle on the MIX swipe is last-write-wins, not the join.

Do **not** assign rental-card eras onto VA19 because:

- a rental box sat at the same pump, or
- `VA RENTAL 2` starts with `VA` (`NicknameRegion` used to treat it as Virginia), or
- `display_name` looks like a nickname.

## Coverage impact

Rental cards persist as **office** eras (`holder_type=office`, holder key `Rental 1` / `VA RENTAL 2`). They do not raise roster `known%`. A rental `display_name` wrongly stored with `linked_car_efleets_id=27VA19` is skipped for device-link coverage and watch seeds. VA19 stays known from CARD-19 (testdata) or the Lindsey/`27SGXP` card (live), plus MIX only when GPS exclusive sits name it.

## Pair dates

Office/rental eras keep `switched_at` / `next_pair_at` null (see [`CARD-PAIR-DATES.md`](CARD-PAIR-DATES.md)). CARD-19's VA19 car era gets `pair_started_at` from the swipe/GPS window and no switch. MIX VA15→VA19 still records the switch at the first VA19 exclusive sit.

## Locks

No Last Reading writes. Never XRAY. Owner assignment outranks later GPS (flag only). `go test`, `cards split`, `cards ladder` are the control.
