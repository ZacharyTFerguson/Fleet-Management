# Canopy audit — even chunk 4 of 4

Notes-only visual audit of PDI create centers against ESRI World Imagery (~zoom 19).
Agent `CursorCloud-4`. No OneStep API calls; no zone reshapes.

## Counts

| Verdict | n |
|---|---:|
| PASS | 31 |
| FAIL | 200 |
| HOLD | 2 |
| **Total** | **233** |

All 233 CSV rows had valid lat/lng and received an ESRI mosaic.

## PASS list

Create center sits on the pump canopy roof or fuel pad.

| Code | Reason |
|---|---|
| `A000389` | Center sits on the standalone white rectangular pump canopy; store is a separate building to the north. |
| `A000559` | Center sits on the west side of the striped rectangular pump canopy over the fuel pad. |
| `A000649` | Center sits on the north edge of the striped pump canopy roof. |
| `A000715` | Center sits on the west edge of the striped pump canopy roof. |
| `A000723` | Center sits on the striped pump canopy roof over the pump islands. |
| `A000903` | Crosshair sits on the dark pump canopy roof beside the store. |
| `A000969` | Crosshair sits on the striped pump canopy roof. |
| `A000977` | Crosshair sits on the north edge of the striped pump canopy. |
| `A001025` | Crosshair sits on the reddish striped pump canopy roof. |
| `A001041` | Crosshair sits on the striped pump canopy roof. |
| `A001091` | Crosshair sits on the striped pump canopy roof. |
| `A001115` | Center sits on the north edge of the long pump canopy and adjacent fuel pad. |
| `A001155` | Center sits on the pump canopy roof of the corner station. |
| `A001221` | Center sits on the east edge of the pump canopy next to the store. |
| `A001229` | Center sits on the small pump canopy attached to the east side of the store. |
| `A001279` | Center sits on the small street-front pump canopy roof. |
| `A001311` | Center sits on the south portion of the long pump canopy. |
| `A001385` | Center sits on the dark pump canopy roof over the fuel islands. |
| `A001401` | Center sits on the west side of the striped pump canopy roof. |
| `A001457` | Center sits on the west edge of the white pump canopy roof. |
| `A001523` | Center sits on the striped pump canopy roof over the pump islands. |
| `A001531` | Center sits on the west edge of the striped pump canopy roof. |
| `A001539` | Center sits on the striped pump canopy roof over the fuel islands. |
| `A001555` | Center sits on the west edge of the orange pump canopy roof. |
| `A001565` | Center sits on the north edge of the striped pump canopy roof. |
| `A001605` | Crosshair sits on the white pump canopy roof beside the convenience store. |
| `A001719` | Crosshair sits on the pump canopy roof in front of the convenience store. |
| `A001769` | Crosshair sits on the north edge of the pump canopy along the street. |
| `A002127` | Center sits on the west edge of the white pump canopy beside the store. |
| `A002209` | Center sits on the south edge of the dark pump canopy along the road. |
| `A002225` | Center sits on the east edge of the solar-panel pump canopy. |

## HOLD

- `A000535` — White-roofed structure at the pin could be a small store or canopy; cannot reliably tell them apart.
- `A002095` — Create-center is under solid cloud cover, so the imagery is unusable at the marker.

## FAIL theme summary

Typical miss: the pin is on the convenience-store roof while a distinct rectangular pump canopy sits a few meters away, or the pin is not on a station at all.

| Theme | n | What we saw |
|---|---:|---|
| store roof | 71 | Pin on C-store / kiosk roof (HVAC, larger building); canopy is a separate rectangle. |
| offset canopy / on-parcel miss | 41 | Station is in frame but pin is on drive aisle, parking, or an offset canopy. |
| road/intersection | 31 | Pin on travel lanes, intersection, or median. |
| woods/field/grass | 20 | Pin in woods, field, grass, or dirt — no station at the mark. |
| residential/wrong parcel | 16 | Pin on a house, yard, or clearly residential lot. |
| other commercial / wrong building | 10 | Pin on warehouse, shopping-center, restaurant, or other non-canopy roof. |
| no station visible / wrong parcel | 7 | No gas station / canopy visible at the create center. |
| other off-canopy | 4 | Other off-canopy placements. |

## Method

- Imagery: ESRI World Imagery (`server.arcgisonline.com` export and/or zoom-19 tiles), ~140 m window, red crosshair at create center.
- PASS only if the crosshair is on the pump canopy or the fuel pad/pump islands.
- HOLD preferred over guessing when the roof cannot be told from a canopy, or when clouds cover the mark.
