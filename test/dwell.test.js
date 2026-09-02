const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const { applyDwell, emptyState, normalizePing } = require("../lib/dwell");

function ping(overrides = {}) {
  return {
    vehicleId: "DEMO-1",
    lat: 40.7128,
    lng: -74.006,
    recordedAt: "2026-09-02T12:00:00.000Z",
    speedMph: 0,
    ...overrides,
  };
}

describe("normalizePing", () => {
  it("rejects an empty or missing payload", () => {
    assert.deepEqual(normalizePing({}), { ok: false, reason: "missing_vehicle" });
    assert.deepEqual(normalizePing(null), { ok: false, reason: "no_vehicle_payload" });
    assert.deepEqual(normalizePing(undefined), { ok: false, reason: "no_vehicle_payload" });
  });

  it("accepts unit aliases and ISO times", () => {
    const result = normalizePing({
      unit: "DEMO-2",
      latitude: 41.5,
      longitude: -72.1,
      timestamp: "2026-09-02T13:00:00Z",
    });
    assert.equal(result.ok, true);
    assert.equal(result.ping.vehicleId, "DEMO-2");
    assert.equal(result.ping.lat, 41.5);
    assert.equal(result.ping.lng, -72.1);
  });

  it("rejects bad coordinates", () => {
    assert.equal(normalizePing(ping({ lat: 200 })).ok, false);
  });
});

describe("applyDwell", () => {
  it("does not record a short sit", () => {
    const first = applyDwell(emptyState(), ping());
    assert.equal(first.recorded, false);
    assert.equal(first.reason, "dwell_not_long_enough");

    const second = applyDwell(
      { locations: first.locations, stays: first.stays },
      ping({ recordedAt: "2026-09-02T12:10:00.000Z" }),
    );
    assert.equal(second.recorded, false);
    assert.equal(second.locations.length, 0);
  });

  it("records the place after the vehicle sits long enough", () => {
    const first = applyDwell(emptyState(), ping());
    const recorded = applyDwell(
      { locations: first.locations, stays: first.stays },
      ping({ recordedAt: "2026-09-02T12:16:00.000Z", lat: 40.71285, lng: -74.00605 }),
    );
    assert.equal(recorded.recorded, true);
    assert.equal(recorded.reason, "new_important_location");
    assert.equal(recorded.locations.length, 1);
    assert.equal(recorded.location.vehicleId, "DEMO-1");
    assert.ok(recorded.location.dwellMinutes >= 15);
    assert.ok(Math.abs(recorded.location.lat - 40.7128) < 0.001);
    assert.ok(Math.abs(recorded.location.lng + 74.006) < 0.001);
  });

  it("does not treat a moving vehicle as sitting", () => {
    const result = applyDwell(emptyState(), ping({ speedMph: 28 }));
    assert.equal(result.recorded, false);
    assert.equal(result.reason, "vehicle_moving");
    assert.equal(result.stay, null);
  });

  it("starts a new stay after the vehicle leaves and comes back", () => {
    const parked = applyDwell(emptyState(), ping());
    const left = applyDwell(
      { locations: parked.locations, stays: parked.stays },
      ping({ lat: 40.73, lng: -74.0, recordedAt: "2026-09-02T12:20:00.000Z" }),
    );
    assert.equal(left.recorded, false);
    const returned = applyDwell(
      { locations: left.locations, stays: left.stays },
      ping({ recordedAt: "2026-09-02T12:21:00.000Z" }),
    );
    assert.equal(returned.stay.arrivedAt, "2026-09-02T12:21:00.000Z");
  });

  it("updates the same important location on a later visit", () => {
    const first = applyDwell(emptyState(), ping());
    const recorded = applyDwell(
      { locations: first.locations, stays: first.stays },
      ping({ recordedAt: "2026-09-02T12:16:00.000Z" }),
    );
    const nextDayArrive = applyDwell(
      { locations: recorded.locations, stays: recorded.stays },
      ping({ recordedAt: "2026-09-03T08:00:00.000Z" }),
    );
    const nextDayRecord = applyDwell(
      { locations: nextDayArrive.locations, stays: nextDayArrive.stays },
      ping({ recordedAt: "2026-09-03T08:20:00.000Z" }),
    );
    assert.equal(nextDayRecord.recorded, true);
    assert.equal(nextDayRecord.reason, "updated_existing");
    assert.equal(nextDayRecord.locations.length, 1);
    assert.equal(nextDayRecord.location.visitCount, 2);
  });
});
