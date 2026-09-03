import assert from "node:assert/strict";
import test from "node:test";
import { formatMiles, remainingMiles, type Car } from "./types";
import { assertFleetSupabaseURL } from "./supabaseTarget";

function car(over: Partial<Car>): Car {
  return {
    pdi_id: "PDI-0001",
    efleets_id: "27TESTA",
    nickname: "Alpha",
    plate: "ABC123",
    vin: "VIN",
    region: "VA",
    last_oil_miles: 100000,
    last_oil_date: "2026-06-01T00:00:00Z",
    last_reading_miles: 101000,
    last_reading_at: "2026-06-02T00:00:00Z",
    last_reading_source: "fuel_details",
    hold_reason: null,
    interval_miles: 5000,
    ...over,
  };
}

test("HOLD returns null remaining even when last_reading is present", () => {
  assert.equal(
    remainingMiles(
      car({
        hold_reason: "NO_DEVICE",
        last_oil_miles: 100000,
        last_reading_miles: 104000,
      }),
    ),
    null,
  );
});

test("null last_oil_miles returns null remaining", () => {
  assert.equal(remainingMiles(car({ last_oil_miles: null })), null);
});

test("null last_reading_miles returns null remaining", () => {
  assert.equal(remainingMiles(car({ last_reading_miles: null })), null);
});

test("computes remaining from interval minus miles since oil", () => {
  assert.equal(
    remainingMiles(
      car({
        last_oil_miles: 100000,
        last_reading_miles: 101250,
        interval_miles: 5000,
      }),
    ),
    3750,
  );
});

test("interval 0 uses the 5000-mile default", () => {
  assert.equal(
    remainingMiles(
      car({
        last_oil_miles: 100000,
        last_reading_miles: 101000,
        interval_miles: 0,
      }),
    ),
    4000,
  );
});

test("formatMiles renders an em dash for null remaining", () => {
  assert.equal(formatMiles(null), "—");
  assert.equal(formatMiles(undefined), "—");
});

test("assertFleetSupabaseURL allows the fleet host and loopback", () => {
  assert.doesNotThrow(() =>
    assertFleetSupabaseURL("https://hdtwfdjdvdzdxfdriyzn.supabase.co"),
  );
  assert.doesNotThrow(() =>
    assertFleetSupabaseURL("https://HDTWFDJDVDZDXFDRIYZN.supabase.co"),
  );
  assert.doesNotThrow(() => assertFleetSupabaseURL("http://127.0.0.1:54321"));
});

test("assertFleetSupabaseURL refuses XRAY and other supabase hosts", () => {
  assert.throws(
    () => assertFleetSupabaseURL("https://CHJQCZNYXVTJBAMTTQDJ.supabase.co"),
    /XRAY/,
  );
  assert.throws(
    () => assertFleetSupabaseURL("https://xray.example/supabase"),
    /XRAY/,
  );
  assert.throws(
    () => assertFleetSupabaseURL("https://otherprojectref12345.supabase.co"),
    /non-fleet/,
  );
  assert.throws(
    () => assertFleetSupabaseURL("http://hdtwfdjdvdzdxfdriyzn.supabase.co"),
    /non-https/,
  );
});
