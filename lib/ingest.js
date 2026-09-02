const { applyDwell, emptyState, normalizePing } = require("./dwell");
const { readCatalog, saveState } = require("./store");

function ingestPing(raw, options = {}) {
  const parsed = normalizePing(raw);
  if (!parsed.ok) {
    return {
      recorded: false,
      reason: parsed.reason,
      vehicleId: null,
      location: null,
    };
  }

  const filePath = options.filePath;
  const catalog = options.state
    ? {
        locations: options.state.locations,
        stays: options.state.stays,
      }
    : filePath
      ? readCatalog(filePath)
      : emptyState();

  const result = applyDwell(
    {
      locations: catalog.locations,
      stays: catalog.stays,
    },
    parsed.ping,
  );

  const nextState = { locations: result.locations, stays: result.stays };
  if (filePath) {
    saveState(nextState, filePath);
  }
  if (options.state) {
    options.state.locations = result.locations;
    options.state.stays = result.stays;
  }

  return {
    recorded: result.recorded,
    reason: result.reason,
    vehicleId: parsed.ping.vehicleId,
    ping: parsed.ping,
    location: result.location ?? null,
    stay: result.stay,
    locations: result.locations,
  };
}

function ingestMany(pings, options = {}) {
  const results = [];
  for (const ping of pings) {
    results.push(ingestPing(ping, options));
  }
  return results;
}

module.exports = {
  ingestPing,
  ingestMany,
};
