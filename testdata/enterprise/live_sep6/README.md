# live_sep6 operator drop (demo scale)

These paths mirror a September 2026 operator desktop dump without committing PHI.
Use the checked-in **demo** CSVs for unit tests; use your local Downloads copies for live ingest.

## Full-dump ingest (sqlite)

```bash
./bin/oilchange sync-enterprise \
  --vehicles testdata/enterprise/fleetsummary_live.csv \
  --fuel-details testdata/enterprise/details_live.csv \
  --shop-ro path/to/your/MaintenanceDetail.csv
```

Maintenance may land before Fleet Summary. A later `--vehicles` import reconciles `cars.last_oil_*`
from shop ROs already in sqlite (`internal/store/reconcileLastOil`).

DETAILS + Maintenance never compute Last Reading. Run `sync-onestep` then `compute` separately.

## Desk / report path

```
sync-enterprise → sqlite (fills, shop_ros, card_transactions, last_oil_*)
       ↓
compute (Last Reading / HOLD only)
       ↓
sync --mirror web/data/cars.json   # Oil Desk read surface
```

History and box score read `card_transactions` / `fills` sorted **newest-first**
(Provider Transaction Date+Time desc, stable ties).
