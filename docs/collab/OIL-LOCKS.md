# Oil locks (collaborator card)

If a step would violate one of these, stop and HOLD. Do not “make the desk look finished.”

1. **Last Reading** = Enterprise odometer at a trusted fill/shop second + OneStep **drive-stop miles since that second**. Formula lives only in `internal/oil`.
2. **Never** use OneStep `odometer` / `odometer_from` / `odometer_to` / device mileage as Last Reading.
3. **Never invent miles.** A missing GPS sum is `NO_DRIVESTOP`, not zero, unless drive-stop returned a measured empty trip list (that measured 0 is stored on purpose).
4. **HOLD skips the Last Reading write.** `WriteLastReading` must not seed a number to clear a HOLD.
5. Join cars to GPS boxes on **`factory_id` only**. `device_id` is for History / drive-stop. **`display_name` is never a join key.**
6. Live roster map: `data/runtime/onestep-map.csv`. Fixture `testdata/onestep/map.csv` is not the fleet.
7. SQLite is the working store. Neon is the backup. This fleet’s Supabase is Oil Desk today (`fleet_cars`); later a second **full copy** so there is still a backup if Neon is down. Do **not** add S3, extra Neon, or direct AWS. Refuse XRAY (`chjqcznyxvtjbamttqdj`) and `-pooler` as `DATABASE_URL`. Today’s `fleet_cars` sync is not that full copy.
8. ExpectedBand (90 mph) rejects punches. It does not invent Last Reading.
