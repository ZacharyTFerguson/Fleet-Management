import assert from "node:assert/strict";
import {
  FLEET_DEFAULT_INTERVAL_MILES,
  IMPREZA_2024_OIL,
  OIL_CHANGE_BY_USE,
  OIL_CIRCUIT_STEPS,
  OIL_CONSUMPTION_DRIVERS,
  SUBARU_SCHEDULE_MILES,
  SUBARU_SCHEDULE_MONTHS,
} from "./imprezaOil.ts";

assert.equal(IMPREZA_2024_OIL.requiredViscosity, "0W-16");
assert.equal(IMPREZA_2024_OIL.capacityWithFilterUsQt, 4.7);
assert.equal(IMPREZA_2024_OIL.capacityOilOnlyUsQt, 4.4);
assert.equal(IMPREZA_2024_OIL.lowToFullUsQt, 1.1);
assert.equal(IMPREZA_2024_OIL.engine, "FB20");
assert.equal(IMPREZA_2024_OIL.replenishIfUnavailable, "0W-20");
assert.equal(FLEET_DEFAULT_INTERVAL_MILES, 5000);
assert.equal(SUBARU_SCHEDULE_MILES, 6000);
assert.equal(SUBARU_SCHEDULE_MONTHS, 6);
assert.equal(OIL_CIRCUIT_STEPS.length, 6);
assert.equal(OIL_CONSUMPTION_DRIVERS.length, 9);
assert.ok(IMPREZA_2024_OIL.requiredGrade.includes("GF-6B"));
assert.equal(OIL_CHANGE_BY_USE.length, 3);
assert.equal(OIL_CHANGE_BY_USE[0].miles, 6000);
assert.equal(OIL_CHANGE_BY_USE[1].id, "fleet");
assert.equal(OIL_CHANGE_BY_USE[1].miles, 5000);
assert.equal(OIL_CHANGE_BY_USE[2].miles, null);
assert.ok(OIL_CHANGE_BY_USE[2].check.toLowerCase().includes("2nd"));

console.log("imprezaOil: ok");
