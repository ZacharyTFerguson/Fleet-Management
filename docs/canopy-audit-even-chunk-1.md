# Canopy audit — even chunk 1 of 4

Notes-only visual audit of PDI create centers against ESRI World Imagery (~zoom 19). The magenta crosshair in each mosaic is the given `lat`/`lng`. No OneStep calls, no zone reshapes.

## Counts

| Verdict | n | share |
|---|---:|---:|
| PASS | 38 | 16.2% |
| FAIL | 192 | 82.1% |
| HOLD | 4 | 1.7% |
| **Total** | **234** | 100% |

All 234 CSV rows have valid lat/lng and were imaged.

## PASS list

Create center sits on the pump canopy roof or on the fuel pad / pump islands.

| code | city, ST | reason |
|---|---|---|
| A000465 | ELKTON, MD | Center sits on the striped pump-canopy roof beside the small store. |
| A000481 | OAKLAND, NJ | Center sits on the reddish paneled pump canopy; the darker store is attached to the northwest. |
| A000489 | STANHOPE, NJ | Center sits on the pump-canopy roof; the darker store section is attached to the north. |
| A000619 | BRANDON, FL | Center sits on the rectangular pump canopy roof. |
| A000635 | TRUMBULL, CT | Center sits on the long rectangular pump canopy roof. |
| A000693 | CINCINNATI, OH | Center sits on the dark rectangular pump canopy in the fueling lot. |
| A000725 | BROOKLYN, NY | Center sits on the rectangular pump canopy roof along the street. |
| A000783 | BALTIMORE, MD | Center sits on the striped rectangular pump canopy roof. |
| A000815 | PARAMUS, NJ | Center sits on the small rectangular pump canopy roof. |
| A000963 | YONKERS, NY | Center sits on the long rectangular pump canopy beside the highway; the store is the square roof to the west. |
| A001077 | CHARLOTTESVILLE, VA | Center sits on the paneled pump canopy over the fuel islands; a smaller kiosk is attached to the south. |
| A001093 | RICHMOND, VA | Center sits on the blue-edged rectangular pump canopy along the road. |
| A001141 | NORWALK, CT | Center sits on the fuel pad at the pump islands. |
| A001247 | FISHKILL, NY | Center sits on the pump canopy roof. |
| A001273 | MILLVILLE, NJ | Center sits on the pump canopy roof. |
| A001305 | RICHMOND, VA | Center sits on the pump canopy roof. |
| A001321 | BUFFALO, NY | Center sits on the long rectangular pump canopy. |
| A001329 | SHENANDOAH, VA | Center sits on the pump canopy roof. |
| A001411 | NORTH PORT, FL | Center sits on the west end of the pump canopy over the fuel islands. |
| A001419 | ARLINGTON, VA | Center sits on the south end of the rectangular pump canopy in the triangle lot. |
| A001591 | RICHMOND, VA | Center sits on the pump canopy roof. |
| A001631 | SICKLERVILLE, NJ | Center sits on the pump canopy roof. |
| A001673 | FREDERICK, MD | Center sits on the pump canopy roof. |
| A001689 | BRISTOL, CT | Center sits on the pump canopy roof. |
| A001705 | LORTON, VA | Center sits on the pump canopy roof. |
| A001771 | TRUMBULL, CT | Center sits on the pump canopy roof. |
| A001803 | PLANTSVILLE, CT | Center sits on the small pump canopy next to the store. |
| A001843 | SOUTH OZONE PARK, NY | Center sits on the rectangular pump canopy. |
| A001893 | CALIFORNIA, MD | Center sits on the fuel pad between two pump canopies. |
| A001917 | HARRINGTON, DE | Center sits on the rectangular pump canopy. |
| A001933 | ST PETERSBURG, FL | Center sits on the rectangular pump canopy. |
| A002023 | GAITHERSBURG, MD | Center sits on the rectangular pump canopy. |
| A002039 | WESTAMPTON, NJ | Center sits on the dark rectangular pump canopy beside the store. |
| A002073 | ROCHESTER, NY | Center sits on the rectangular pump canopy. |
| A002105 | PARLIN, NJ | Center sits on the rectangular pump canopy roof along the highway. |
| A002113 | MIDLOTHIAN, VA | Center sits on the north edge of the pump canopy roof. |
| A002121 | JAMAICA, NY | Center sits on the long pump canopy roof in the fuel lot. |
| A002211 | COLONIAL BEACH, VA | Center sits on the striped pump canopy roof. |

**38 PASS**

## HOLD list

Preferred over guessing when canopy vs store vs lot is unreadable.

| code | city, ST | reason |
|---|---|---|
| A000563 | WHITE PLAINS, NY | Heavy building shadows make it impossible to tell canopy versus roof versus lot. |
| A001949 | LITTLE ORLEANS, MD | Cannot tell whether the gravel by the small building is a fuel pad or parking. |
| A002219 | WASHINGTON, DC | Heavy shadow from the adjacent building makes canopy versus store versus pavement unclear. |
| A002269 | READING, PA | Cannot tell whether the small rectangular corner roof is a pump canopy or a store. |

## FAIL theme summary

Typical FAIL is not a missing station in the catalog — it is a create center that landed on the convenience-store roof, the lot, or a geocode that is not the fuel pad.

| Theme | n |
|---|---:|
| On-site store / kiosk roof (canopy nearby) | 73 |
| Offset onto lot pavement / parking (canopy nearby) | 34 |
| Wrong site or adjacent commercial roof | 22 |
| Wrong site: woods / field / grass | 18 |
| Wrong site: road / intersection | 15 |
| Wrong site: residential street / house | 15 |
| Parking lot / plaza pavement, not a fuel pad | 10 |
| Vacant, demolished, or gravel lot | 5 |
| **FAIL total** | **192** |

### Theme notes

- **Store roof:** Common at Wawa, Sheetz, Royal Farms, and other c-store sites where the point is on the building and the canopy is a separate rectangle a few bays away.
- **Offset pavement:** Point is in the drive aisle or apron between store and canopy, or just off a canopy edge onto parking — not on pump islands.
- **Wrong site:** Point is on a road, woods, house, or another business. Several branded rows (Shell, Sheetz, Love's, Speedway) geocode into neighborhoods or offices rather than the fuel court.
- **Travel centers:** Love's / Rutter's store roofs FAIL even when auto and diesel canopies are clearly on the same lot.

## Method

- Imagery: ESRI World Imagery MapServer export (tile mosaic fallback), ~90 m half-span, 768 px, center crosshair.
- Agent: `CursorCloud-1`.
- Rule: PASS only on canopy / fuel pad; FAIL for store, parking, road, woods, offset canopy; HOLD if imagery or structure is unreadable.

