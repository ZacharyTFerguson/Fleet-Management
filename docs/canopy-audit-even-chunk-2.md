# Canopy audit — even chunk 2 of 4

Notes-only visual check of PDI create centers against ESRI World Imagery (~zoom 19). Agent `CursorCloud-2`. No OneStep API calls; zones were not reshaped.

## Counts

| Verdict | Count |
|---------|------:|
| PASS | 33 |
| FAIL | 200 |
| HOLD | 0 |
| **Total** | **233** |

All 233 CSV rows had valid lat/lng. Imagery was usable at every site (no HOLDs).

## PASS list

Create center sits on the pump canopy roof or fuel pad.

| Code | Reason |
|------|--------|
| A000417 | Center sits on the rectangular fuel canopy. |
| A000449 | Center sits on the small rectangular fuel canopy north of the store. |
| A000475 | Center sits on the rectangular fuel canopy. |
| A000499 | Center sits on the striped fuel canopy north of the small attached store. |
| A000565 | Center sits on the small white fuel canopy beside the dark store roof. |
| A000597 | Center sits on the light rectangular fuel canopy west of the store. |
| A000687 | Center sits on the dark rectangular fuel canopy immediately north of the store. |
| A000703 | Center sits on the small rectangular fuel canopy in the paved lot. |
| A000867 | Center sits on the rectangular pump canopy roof; the store is the orange-roofed building to the southeast. |
| A000947 | Center sits on the north edge of the striped pump canopy roof. |
| A000965 | Center sits on the striped pump canopy roof; the store is the darker building to the southeast. |
| A000973 | Center sits on the rectangular pump canopy roof; the store is the white-roofed building to the south. |
| A000989 | Center sits on the pump canopy roof; the white-roofed store is to the north. |
| A000997 | Center sits on the striped pump canopy roof; the small store is to the south. |
| A001037 | Center sits on the square pump canopy roof; the store is the gray-roofed building to the south. |
| A001063 | Center sits on the elongated pump canopy roof; the white-roofed store is to the southwest. |
| A001079 | Center sits on the diamond-oriented pump canopy roof; the store is to the southeast. |
| A001087 | Center sits on the dark pump-canopy roof beside the store. |
| A001127 | Center sits on the dark pump-canopy roof north of the white store. |
| A001177 | Center sits on the striped pump-canopy roof with vehicles at the islands. |
| A001421 | Center sits on the striped pump canopy roof over the fuel islands. |
| A001471 | Center sits on the striped pump canopy over the fueling area. |
| A001559 | Center sits on the rectangular pump canopy roof. |
| A001625 | Center sits on the rectangular pump canopy roof. |
| A001895 | Center sits on the white pump-canopy roof with vehicles at the islands. |
| A001919 | Center sits on the long striped pump-canopy roof. |
| A001985 | Center sits on the striped pump-canopy roof. |
| A002025 | Center sits on the small rectangular pump-canopy roof. |
| A002057 | Center sits on the white pump canopy roof beside the store. |
| A002147 | Center sits on the white pump canopy roof next to the store. |
| A002181 | Center sits on the striped pump canopy roof in the fueling lot. |
| A002197 | Center sits on the white pump canopy roof beside the store. |
| A002221 | Center sits on the striped pump canopy roof south of the store. |

## FAIL theme summary

Typical miss: the create point is on the convenience-store roof while the canopy is a separate rectangle. Next most common: woods/field (wrong pin), road/street, and the drive aisle between store and canopy.

| Theme | Count | Example codes |
|-------|------:|---------------|
| store / kiosk / commercial roof | 105 | A000377, A000385, A000409, A000425, A000441, A000483, A000491, A000507, … (+97 more) |
| woods / field / yard / demolished pad | 37 | A000393, A000401, A000467, A000589, A000653, A000695, A000711, A000719, … (+29 more) |
| offset canopy / drive aisle / parking / landscaping | 29 | A000369, A000457, A000581, A000785, A000841, A000915, A001021, A001053, … (+21 more) |
| road / street / highway | 28 | A000433, A000637, A000671, A000759, A000801, A000809, A001143, A001159, … (+20 more) |
| water / off-site | 1 | A001389 |

## Method

- Source CSV columns: `general_code,label,address,city,state,lat,lng,zone_id`.
- Mosaic: ESRI World Imagery XYZ tiles composited at zoom 19, 640×640 px, crosshair at the exact create lat/lng.
- PASS = center on canopy roof or pump islands / fuel pad.
- FAIL = store roof, parking, road, woods, offset canopy, or other off-pad land cover.
- HOLD = blank/unusable imagery (none in this chunk). Prefer HOLD over guessing.

