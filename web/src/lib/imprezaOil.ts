/** 2024 Subaru Impreza engine-oil spec, from the owner's manual Ch. 12. */

/** Matches Go `model.DefaultInterval` — Oil Desk due clock when `interval_miles` is unset. */
export const FLEET_DEFAULT_INTERVAL_MILES = 5000;

/** Subaru Warranty & Maintenance Booklet: oil + filter, whichever comes first. */
export const SUBARU_SCHEDULE_MILES = 6000;
export const SUBARU_SCHEDULE_MONTHS = 6;

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
  source: "2024 Subaru Impreza Owner’s Manual, Engine Oil (Ch. 11) and Specifications (Ch. 12)",
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

/** Oil-change clock by how the car is used. Severe has no printed mile number — Subaru says sooner. */
export const OIL_CHANGE_BY_USE = [
  {
    id: "normal",
    n: 1,
    title: "Highway / mixed",
    when: "Longer trips. Engine fully warms. Not dusty, not extreme cold.",
    changeLabel: "6,000 mi / 6 mo",
    changeDetail: "Subaru booklet — whichever first",
    miles: SUBARU_SCHEDULE_MILES,
    check: "At scheduled service. Top off if the dipstick is below L.",
    action: "Change oil and filter on the booklet clock.",
  },
  {
    id: "fleet",
    n: 2,
    title: "This fleet (PDI)",
    when: "Regional mix: stop-and-go, idle, heat and cold, short hops between jobs.",
    changeLabel: "5,000 mi",
    changeDetail: "Oil Desk due clock — do not wait for 6,000",
    miles: FLEET_DEFAULT_INTERVAL_MILES,
    check: "Treat as closer to severe than highway. Watch the dipstick.",
    action: "Change oil and filter when Oil Desk remaining hits zero.",
  },
  {
    id: "severe",
    n: 3,
    title: "Severe / high use",
    when: "Dusty roads, repeated short trips, extreme cold, long idle, heavy traffic, hard accel.",
    changeLabel: "Sooner",
    changeDetail: "Subaru: more often than the booklet",
    miles: null,
    check: "Every 2nd fuel fill. Change sooner if you are adding oil between services.",
    action: "Do not wait for 6,000 or 5,000 if the car is using oil or the use is severe.",
  },
] as const;

export type OilChangeUseId = (typeof OIL_CHANGE_BY_USE)[number]["id"];
export type OilCircuitStepId = (typeof OIL_CIRCUIT_STEPS)[number]["id"];
