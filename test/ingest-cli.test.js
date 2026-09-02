const { describe, it, before, after } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { ingestPing } = require("../lib/ingest");

const cli = path.join(__dirname, "..", "bin", "record-location.js");

describe("ingestPing", () => {
  it("does not invent a location when no vehicle was sent", () => {
    const result = ingestPing({});
    assert.equal(result.recorded, false);
    assert.equal(result.reason, "missing_vehicle");
    assert.equal(result.location, null);
  });

  it("persists a dwell to the catalog file", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "important-loc-"));
    const filePath = path.join(dir, "important-locations.json");
    ingestPing(
      {
        vehicleId: "DEMO-1",
        lat: 40.7128,
        lng: -74.006,
        recordedAt: "2026-09-02T12:00:00.000Z",
        speedMph: 0,
      },
      { filePath },
    );
    const recorded = ingestPing(
      {
        vehicleId: "DEMO-1",
        lat: 40.7128,
        lng: -74.006,
        recordedAt: "2026-09-02T12:16:00.000Z",
        speedMph: 0,
      },
      { filePath },
    );
    assert.equal(recorded.recorded, true);
    const saved = JSON.parse(fs.readFileSync(filePath, "utf8"));
    assert.equal(saved.locations.length, 1);
    assert.equal(saved.locations[0].vehicleId, "DEMO-1");
  });
});

describe("record-location CLI", () => {
  let dir;
  let filePath;

  before(() => {
    dir = fs.mkdtempSync(path.join(os.tmpdir(), "important-cli-"));
    filePath = path.join(dir, "important-locations.json");
  });

  after(() => {
    fs.rmSync(dir, { recursive: true, force: true });
  });

  it("reports no_vehicle_payload for an empty stdin body", () => {
    const result = spawnSync(process.execPath, [cli, "--file", filePath], {
      encoding: "utf8",
      input: "",
    });
    assert.equal(result.status, 0);
    const body = JSON.parse(result.stdout);
    assert.equal(body.recorded, false);
    assert.equal(body.reason, "missing_vehicle");
  });

  it("records after two sitting pings 16 minutes apart", () => {
    const first = spawnSync(
      process.execPath,
      [
        cli,
        "--file",
        filePath,
        "--vehicle",
        "DEMO-1",
        "--lat",
        "40.7128",
        "--lng",
        "-74.006",
        "--speed",
        "0",
        "--at",
        "2026-09-02T12:00:00.000Z",
      ],
      { encoding: "utf8" },
    );
    assert.equal(first.status, 0);
    assert.equal(JSON.parse(first.stdout).recorded, false);

    const second = spawnSync(
      process.execPath,
      [
        cli,
        "--file",
        filePath,
        "--vehicle",
        "DEMO-1",
        "--lat",
        "40.7128",
        "--lng",
        "-74.006",
        "--speed",
        "0",
        "--at",
        "2026-09-02T12:16:00.000Z",
      ],
      { encoding: "utf8" },
    );
    assert.equal(second.status, 0);
    const body = JSON.parse(second.stdout);
    assert.equal(body.recorded, true);
    assert.equal(body.location.vehicleId, "DEMO-1");
  });
});
