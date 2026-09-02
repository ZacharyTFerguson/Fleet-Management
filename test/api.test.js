const { describe, it, before, after } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { createServer } = require("../server");

async function listen(server) {
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address();
  return `http://127.0.0.1:${port}`;
}

async function jsonRequest(base, method, urlPath, body) {
  const response = await fetch(base + urlPath, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  return { status: response.status, body: await response.json() };
}

describe("important location API", () => {
  let dir;
  let server;
  let base;

  before(async () => {
    dir = fs.mkdtempSync(path.join(os.tmpdir(), "important-api-"));
    process.env.IMPORTANT_LOCATIONS_FILE = path.join(dir, "important-locations.json");
    delete require.cache[require.resolve("../server")];
    const fresh = require("../server");
    server = fresh.createServer();
    base = await listen(server);
  });

  after(async () => {
    await new Promise((resolve) => server.close(resolve));
    fs.rmSync(dir, { recursive: true, force: true });
    delete process.env.IMPORTANT_LOCATIONS_FILE;
  });

  it("serves health and an empty catalog", async () => {
    const health = await jsonRequest(base, "GET", "/api/v1/health");
    assert.equal(health.status, 200);
    assert.equal(health.body.ok, true);

    const catalog = await jsonRequest(base, "GET", "/api/v1/important-locations");
    assert.equal(catalog.status, 200);
    assert.equal(catalog.body.count, 0);
  });

  it("does not record an empty ping", async () => {
    const result = await jsonRequest(base, "POST", "/api/v1/pings", {});
    assert.equal(result.status, 200);
    assert.equal(result.body.recorded, false);
    assert.equal(result.body.reason, "missing_vehicle");
  });

  it("records a vehicle after it sits for a while", async () => {
    const arrive = await jsonRequest(base, "POST", "/api/v1/pings", {
      vehicleId: "DEMO-1",
      lat: 40.7128,
      lng: -74.006,
      speedMph: 0,
      recordedAt: "2026-09-02T12:00:00.000Z",
    });
    assert.equal(arrive.body.recorded, false);

    const stay = await jsonRequest(base, "POST", "/api/v1/pings", {
      vehicleId: "DEMO-1",
      lat: 40.7128,
      lng: -74.006,
      speedMph: 0,
      recordedAt: "2026-09-02T12:16:00.000Z",
    });
    assert.equal(stay.body.recorded, true);
    assert.equal(stay.body.location.vehicleId, "DEMO-1");

    const catalog = await jsonRequest(base, "GET", "/api/v1/important-locations?vehicle=DEMO-1");
    assert.equal(catalog.body.count, 1);
    assert.equal(catalog.body.locations[0].vehicleId, "DEMO-1");
  });

  it("serves the recorder page", async () => {
    const response = await fetch(base + "/");
    assert.equal(response.status, 200);
    const html = await response.text();
    assert.match(html, /Important locations/);
    assert.match(html, /Simulate 16-minute sit/);
  });
});
