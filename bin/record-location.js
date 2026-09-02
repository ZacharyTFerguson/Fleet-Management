#!/usr/bin/env node
/**
 * Record a vehicle ping. When the unit sits still long enough, the place
 * is stored as an important location.
 *
 * Usage:
 *   node bin/record-location.js
 *   node bin/record-location.js --file data/important-locations.json
 *
 * Payload (stdin JSON, or --vehicle/--lat/--lng/--at/--speed):
 *   { "vehicleId": "DEMO-1", "lat": 40.7128, "lng": -74.006, "recordedAt": "...", "speedMph": 0 }
 *
 * An empty body (this automation's current trigger) exits 0 with
 * recorded: false and reason: no_vehicle_payload.
 */
const fs = require("node:fs");
const path = require("node:path");
const { ingestPing } = require("../lib/ingest");

function parseArgs(argv) {
  const args = { file: process.env.IMPORTANT_LOCATIONS_FILE || null, payload: {} };
  for (let i = 0; i < argv.length; i += 1) {
    const key = argv[i];
    const value = argv[i + 1];
    if (key === "--file" && value) {
      args.file = value;
      i += 1;
    } else if (key === "--vehicle" && value) {
      args.payload.vehicleId = value;
      i += 1;
    } else if (key === "--lat" && value) {
      args.payload.lat = Number(value);
      i += 1;
    } else if (key === "--lng" && value) {
      args.payload.lng = Number(value);
      i += 1;
    } else if (key === "--at" && value) {
      args.payload.recordedAt = value;
      i += 1;
    } else if (key === "--speed" && value) {
      args.payload.speedMph = Number(value);
      i += 1;
    } else if (key === "--json" && value) {
      args.json = value;
      i += 1;
    }
  }
  return args;
}

function readStdin() {
  if (process.stdin.isTTY) {
    return "";
  }
  return fs.readFileSync(0, "utf8");
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  let raw = null;

  if (args.json) {
    raw = JSON.parse(args.json);
  } else {
    const stdin = readStdin().trim();
    if (stdin) {
      raw = JSON.parse(stdin);
    } else if (Object.keys(args.payload).length) {
      raw = args.payload;
    } else {
      raw = {};
    }
  }

  const filePath = args.file
    ? path.resolve(args.file)
    : path.join(__dirname, "..", "data", "important-locations.json");

  const result = ingestPing(raw, { filePath });
  const output = {
    recorded: result.recorded,
    reason: result.reason,
    vehicleId: result.vehicleId,
    location: result.location,
  };
  process.stdout.write(`${JSON.stringify(output, null, 2)}\n`);
  process.exitCode = 0;
}

main();
