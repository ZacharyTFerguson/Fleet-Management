import assert from "node:assert/strict";
import {
  FLEET_DEFAULT_INTERVAL_MILES,
  IMPREZA_2024_OIL,
  OIL_CHANGE_BY_USE,
  OIL_CIRCUIT_STEPS,
  OIL_CONSUMPTION_DRIVERS,
  SUBARU_BOOKLET_OIL_SEVERE_CONDITIONS,
  SUBARU_SCHEDULE_MILES,
  SUBARU_SCHEDULE_MONTHS,
  SUBARU_SEVERE_MILES,
  SUBARU_SEVERE_MONTHS,
} from "./imprezaOil.ts";

assert.equal(IMPREZA_2024_OIL.requiredViscosity, "0W-16");
assert.equal(IMPREZA_2024_OIL.capacityWithFilterUsQt, 4.7);
assert.equal(IMPREZA_2024_OIL.capacityOilOnlyUsQt, 4.4);
assert.equal(IMPREZA_2024_OIL.lowToFullUsQt, 1.1);
assert.equal(IMPREZA_2024_OIL.engine, "FB20");
assert.equal(IMPREZA_2024_OIL.replenishIfUnavailable, "0W-20");
assert.ok(IMPREZA_2024_OIL.source.includes("Ch. 11"));
assert.ok(IMPREZA_2024_OIL.source.includes("Ch. 12"));
assert.ok(IMPREZA_2024_OIL.source.includes("Note 1"));
assert.equal(FLEET_DEFAULT_INTERVAL_MILES, 5000);
assert.equal(SUBARU_SCHEDULE_MILES, 6000);
assert.equal(SUBARU_SCHEDULE_MONTHS, 6);
assert.equal(SUBARU_SEVERE_MILES, 3000);
assert.equal(SUBARU_SEVERE_MONTHS, 3);
assert.equal(OIL_CIRCUIT_STEPS.length, 6);
assert.equal(OIL_CONSUMPTION_DRIVERS.length, 9);
assert.ok(IMPREZA_2024_OIL.requiredGrade.includes("GF-6B"));
assert.equal(OIL_CHANGE_BY_USE.length, 3);
assert.equal(OIL_CHANGE_BY_USE[0].miles, 6000);
assert.equal(OIL_CHANGE_BY_USE[0].months, 6);
assert.equal(OIL_CHANGE_BY_USE[1].id, "fleet");
assert.equal(OIL_CHANGE_BY_USE[1].miles, 5000);
assert.equal(OIL_CHANGE_BY_USE[2].id, "severe");
assert.notEqual(OIL_CHANGE_BY_USE[2].miles, null);
assert.equal(OIL_CHANGE_BY_USE[2].miles, 3000);
assert.equal(OIL_CHANGE_BY_USE[2].months, 3);
assert.equal(OIL_CHANGE_BY_USE[2].changeLabel, "3,000 mi / 3 mo");
assert.ok(OIL_CHANGE_BY_USE[2].changeDetail.includes("Note 1"));
assert.ok(OIL_CHANGE_BY_USE[2].check.toLowerCase().includes("2nd"));
assert.ok(!OIL_CHANGE_BY_USE[2].when.toLowerCase().includes("dust"));
assert.equal(SUBARU_BOOKLET_OIL_SEVERE_CONDITIONS.length, 3);
assert.ok(
  SUBARU_BOOKLET_OIL_SEVERE_CONDITIONS.every((c) => !c.toLowerCase().includes("dust")),
);

console.log("imprezaOil: ok");
