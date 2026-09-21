# Canopy audit — even chunk 3 of 4

Notes-only visual audit of PDI create centers against ESRI World Imagery (~zoom 19, ~160 m FOV). Crosshair = CSV `lat`/`lng`. No OneStep API calls; no zone reshapes.

## Counts

| Verdict | n | share |
|---|---:|---:|
| PASS | 40 | 17.2% |
| FAIL | 187 | 80.3% |
| HOLD | 6 | 2.6% |
| **Total** | **233** | 100% |

All 233 CSV rows had valid lat/lng and usable (non-blank) imagery. HOLDs are pin-on-structure ambiguity, not missing tiles.

Agent: `CursorCloud-3`. Imagery: `esri`. Line-level records: [`canopy-audit-even-chunk-3.jsonl`](canopy-audit-even-chunk-3.jsonl).

## PASS (40)

Create center sits on the pump canopy or fuel pad.

- **A000525** — BURLINGTON, CT: Pin sits on the pump canopy roof with vehicles at the islands around it.
- **A000541** — BURLINGTON, NJ: Pin sits on the elongated pump canopy roof beside the small store.
- **A000567** — RAMSEY, NJ: Pin sits on the reddish rectangular pump canopy beside the highway.
- **A000591** — LITTLE FALLS, NJ: Pin sits on the reddish-brown pump canopy between the highways.
- **A000623** — NIAGARA FALLS, NY: Pin sits on the center of the pump canopy over the pump islands.
- **A000631** — PIKESVILLE, MD: Pin sits on the brown pump canopy north of the dark-roofed store.
- **A000673** — BOWIE, MD: Pin sits on the center of the paneled pump canopy.
- **A000689** — CHESTER, VA: Pin sits on the long paneled pump canopy next to the highway.
- **A000827** — JAMAICA, NY: Pin sits on the striped rectangular pump canopy roof next to the shopping-center building.
- **A000859** — BALTIMORE, MD: Pin sits on the south edge of the white striped pump canopy.
- **A001015** — ENGLEWOOD, FL: Pin sits on the north edge of the striped pump canopy roof.
- **A001031** — MITCHELLVILLE, MD: Pin sits on the rectangular pump canopy roof at the fuel pad.
- **A001081** — CORAOPOLIS, PA: Pin sits on the rectangular pump canopy roof.
- **A001105** — TARPON SPRINGS, FL: Pin sits on the pump canopy roof over the fuel islands.
- **A001121** — ST PETERSBURG, FL: Pin sits on the tan rectangular pump canopy roof.
- **A001269** — NEW MARKET, VA: Pin sits on the light gray pump canopy roof of the station.
- **A001301** — RICHMOND, VA: Pin sits on the center of the square pump canopy roof.
- **A001317** — CRESCENT, PA: Pin sits on the east edge of the pump canopy and attached kiosk on the fuel pad.
- **A001383** — PETERSBURG, VA: Pin sits on the light striped pump canopy beside the dark-roofed store.
- **A001439** — ALEXANDRIA, VA: Pin sits on the light rectangular pump canopy with cars at the pump islands.
- **A001455** — BATAVIA, NY: Pin sits on the light paneled rectangular pump canopy.
- **A001473** — ERIE, PA: Pin sits on the light rectangular pump canopy beside the dark-roofed store.
- **A001513** — RICHMOND, VA: Pin sits on the striped rectangular pump canopy roof.
- **A001521** — BRISTOL, CT: Pin sits on the small white pump canopy in the fuel lot.
- **A001529** — BRANDON, FL: Pin sits on the rectangular pump canopy roof.
- **A001603** — CHATHAM, NY: Pin sits on a small pump canopy or kiosk in the fuel pad.
- **A001611** — SPRINGFIELD, NJ: Pin sits on a light rectangular pump canopy roof.
- **A001677** — PORTSMOUTH, RI: Create center sits on the small light rectangular pump canopy in the paved lot.
- **A001701** — PETERSBURG, VA: Create center sits on the white paneled pump canopy beside the dark-roofed store.
- **A001839** — SANBORN, NY: Pin sits on the white rectangular pump canopy with cars at the pump islands.
- **A001873** — MONROE, NY: Pin sits on a small white pump canopy south of the convenience store.
- **A001913** — HAGERSTOWN, MD: Pin sits on the southern pump canopy roof.
- **A001937** — HEWLETT, NY: Pin sits on the dark paneled pump canopy next to the C-store.
- **A001953** — SELBYVILLE, DE: Pin sits on the southern edge of the long white pump canopy.
- **A002157** — FELTON, DE: Pin sits on the east edge of the narrow white pump canopy.
- **A002167** — PLEASANTVILLE, NJ: Pin sits on the light rectangular pump canopy roof of a corner gas station.
- **A002175** — LAWRENCE, NY: Pin sits on the large white rectangular pump canopy roof with panel seams.
- **A002223** — PRINCESS ANNE, MD: Pin sits on the white rectangular pump canopy roof with panel seams.
- **A002247** — ISLANDIA, NY: Pin sits on the isolated rectangular pump canopy roof at the interchange.
- **A002255** — ABINGDON, VA: Pin sits on the pump canopy roof just north of the island kiosk.

## HOLD (6)

Prefer HOLD over guessing.

- **A000501** — WEST HAVEN, CT: Cannot tell whether the pin is on a small front canopy, the fuel pad, or street-edge pavement.
- **A001357** — BALTIMORE, MD: Small corner station; cannot tell whether the pin is on the store roof or the pump canopy.
- **A001481** — WAVERLY, VA: Small rural buildings; cannot tell whether the pin is on a pump canopy versus a shed or store.
- **A001579** — GOOCHLAND, VA: Small rural cluster; cannot tell whether the pin is on a pump canopy or a shed.
- **A001881** — CHESTER, MD: Small light roof beside a store; cannot tell canopy versus a building wing without guessing.
- **A002117** — LAUREL, MD: Small two-tone corner structure; cannot tell canopy versus store without guessing.

## FAIL theme summary (187)

Typical FAIL is a create center on the convenience-store roof while a clear pump canopy sits next door, or a pin that is not on a gas station at all (woods, road, residential, vacant lot).

| Theme | n |
|---|---:|
| Convenience-store roof (canopy nearby) | 68 |
| Wrong site / woods / field / residential | 26 |
| Building / commercial roof (not a fuel canopy) | 23 |
| Road / shoulder beside a station | 17 |
| Offset: pavement / driveway off the canopy | 15 |
| Offset: pavement between store and canopy | 11 |
| Offset into trees / grass beside a station | 8 |
| Parking / empty pavement / vacant lot | 7 |
| Other offset / unclear structure | 7 |
| Wrong site / road / empty lot | 5 |

**Takeaway:** most FAILs with a station in frame are store-roof or driveway-offset pins, not missing lots. A large minority are geocodes that land off-site (woods, roads, residential). Do not reshape zones from these notes.
