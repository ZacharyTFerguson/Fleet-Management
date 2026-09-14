/** 2024 Subaru Impreza engine-oil spec: Owner’s Manual Ch. 11–12 plus Warranty & Maintenance Booklet (MSA5M2401W) Note 1. */

/** Matches Go `model.DefaultInterval` — Oil Desk due clock when `interval_miles` is unset. */
export const FLEET_DEFAULT_INTERVAL_MILES = 5000;

/** Warranty & Maintenance Booklet schedule table items 1 (engine oil) and 2 (oil filter): R every 6,000 miles / 6 months, whichever first. */
export const SUBARU_SCHEDULE_MILES = 6000;
export const SUBARU_SCHEDULE_MONTHS = 6;

/** Booklet Note 1: under severe driving conditions**, oil AND filter every 3,000 miles (4,800 km) or 3 months. */
export const SUBARU_SEVERE_MILES = 3000;
export const SUBARU_SEVERE_MONTHS = 3;

export const IMPREZA_2024_OIL = {
  year: 2024,
  model: "Subaru Impreza",
  engine: "FB20",
  displacement: "2.0 L BOXER (1,995 cc)",
  engineType: "Horizontally opposed, liquid-cooled 4-cylinder, DOHC, direct injection, non-turbo",
  firingOrder: "1 – 3 – 2 – 4",
  compression: "12.5 : 1",
  requiredViscosity: "0W-16",
  requiredGrade: "ILSAC GF-6B (shield mark)",
  altGrade: "ILSAC GF-6A (starburst) or API SP Resource Conserving",
  requiredForm: "synthetic",
  /** If 0W-16 is unavailable, 0W-20 may be used to top off, then return to 0W-16 at the next change. */
  replenishIfUnavailable: "0W-20",
  capacityWithFilterUsQt: 4.7,
  capacityWithFilterL: 4.4,
  capacityOilOnlyUsQt: 4.4,
  capacityOilOnlyL: 4.2,
  lowToFullUsQt: 1.1,
  lowToFullL: 1.0,
  checkWaitMinutes: 5,
  source:
    "2024 Subaru Impreza Owner’s Manual, Engine Oil (Ch. 11) and Specifications (Ch. 12); 2024 Subaru Warranty & Maintenance Booklet (MSA5M2401W) Note 1",
} as const;

export const OIL_CIRCUIT_STEPS = [
  {
    id: "pan",
    n: 1,
    title: "Oil pan",
    body: "Sump under the boxer. After shutdown, oil drains back here — wait 5 minutes on level ground before reading the dipstick.",
  },
  {
    id: "pump",
    n: 2,
    title: "Pickup and pump",
    body: "The pickup draws from the pan. The pump pushes oil through the filter, then into the main gallery.",
  },
  {
    id: "filter",
    n: 3,
    title: "Oil filter",
    body: "Replace the filter at every oil change. Manual fill with filter is 4.7 US qt (4.4 L) — a guideline; always finish on the dipstick.",
  },
  {
    id: "gallery",
    n: 4,
    title: "Main gallery",
    body: "Pressurized oil feeds the crank, cams, and both cylinder banks of the horizontally opposed 2.0 L FB20.",
  },
  {
    id: "banks",
    n: 5,
    title: "Left and right banks",
    body: "Boxer layout: left bank cylinders 2 and 4, right bank 1 and 3. Firing order 1–3–2–4. Oil then drains back to the pan.",
  },
  {
    id: "check",
    n: 6,
    title: "Level gauge",
    body: "Read both sides of the dipstick; the true level is the lower of the two. Low to Full is about 1.1 US qt (1.0 L). Do not fill above Full when the engine is cold.",
  },
] as const;

/**
 * Owner’s Manual consumption / “change more often” drivers (Ch. 11).
 * Dusty roads belong here — booklet dusty conditions map to Item 8 (air cleaner), not oil.
 */
export const OIL_CONSUMPTION_DRIVERS = [
  "New engine / break-in",
  "Wrong viscosity or lower-quality oil",
  "Repeated engine braking",
  "High RPM for long stretches",
  "Heavy load for long stretches",
  "Long idle",
  "Stop-and-go / heavy traffic",
  "Severe heat or cold",
  "Frequent accel / decel",
] as const;

/**
 * Booklet Note 1 ** conditions that apply to Maintenance Items 1 and 2 (engine oil and filter) only.
 * a. Repeated short distance driving (Items 1 and 2 only)
 * d. Driving in extremely cold weather (Items 1, 2, 17, 18)
 * g. Repeated trailer towing (Items 1, 2, …)
 * Dusty is Item 8 (air cleaner), not oil.
 */
export const SUBARU_BOOKLET_OIL_SEVERE_CONDITIONS = [
  "Repeated short distance driving (Items 1 and 2 only)",
  "Driving in extremely cold weather (Items 1, 2, 17, 18)",
  "Repeated trailer towing (Items 1, 2, ...)",
] as const;

/** Oil-change clock by how the car is used. Booklet table + Note 1; fleet is Oil Desk policy between them. */
export const OIL_CHANGE_BY_USE = [
  {
    id: "normal",
    n: 1,
    title: "Highway / mixed",
    when: "Longer trips. Engine fully warms. Not short-trip, extreme-cold, or trailer-tow.",
    changeLabel: "6,000 mi / 6 mo",
    changeDetail: "Booklet schedule table items 1 and 2 — whichever first",
    miles: SUBARU_SCHEDULE_MILES,
    months: SUBARU_SCHEDULE_MONTHS,
    check: "At scheduled service. Top off if the dipstick is below L.",
    action: "Change oil and filter on the booklet clock.",
  },
  {
    id: "fleet",
    n: 2,
    title: "This fleet (PDI)",
    when: "Regional mix: stop-and-go, idle, heat and cold, short hops between jobs.",
    changeLabel: "5,000 mi",
    changeDetail: "Oil Desk due clock — between booklet normal and severe",
    miles: FLEET_DEFAULT_INTERVAL_MILES,
    months: null,
    check: "Treat as closer to severe than highway. Watch the dipstick.",
    action: "Change oil and filter when Oil Desk remaining hits zero.",
  },
  {
    id: "severe",
    n: 3,
    title: "Severe / high use",
    when: "Repeated short distance driving, extremely cold weather, or repeated trailer towing.",
    changeLabel: "3,000 mi / 3 mo",
    changeDetail:
      "Warranty & Maintenance Booklet Note 1 — 3,000 mi (4,800 km) or 3 months, whichever first",
    miles: SUBARU_SEVERE_MILES,
    months: SUBARU_SEVERE_MONTHS,
    check: "Each fuel fill (booklet p.27). Every 2nd fill under consumption (Owner’s Manual).",
    action: "Change oil and filter on the Note 1 clock. Do not wait for 6,000 or 5,000.",
  },
] as const;

export type OilChangeUseId = (typeof OIL_CHANGE_BY_USE)[number]["id"];
export type OilCircuitStepId = (typeof OIL_CIRCUIT_STEPS)[number]["id"];
