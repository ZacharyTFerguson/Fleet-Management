const { haversineMeters, isFiniteNumber, isValidCoordinate } = require("./geo");
const {
  DWELL_RADIUS_METERS,
  DWELL_MINUTES,
  CLUSTER_RADIUS_METERS,
  STAY_GAP_MINUTES,
  MAX_SIT_SPEED_MPH,
} = require("./defaults");

function parseIsoTime(value) {
  if (value instanceof Date && !Number.isNaN(value.getTime())) {
    return value.toISOString();
  }
  if (typeof value === "number" && Number.isFinite(value)) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? null : date.toISOString();
  }
  if (typeof value === "string" && value.trim()) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? null : date.toISOString();
  }
  return null;
}

function minutesBetween(startIso, endIso) {
  return (new Date(endIso).getTime() - new Date(startIso).getTime()) / 60_000;
}

function centroid(points) {
  const n = points.length;
  return {
    lat: points.reduce((sum, point) => sum + point.lat, 0) / n,
    lng: points.reduce((sum, point) => sum + point.lng, 0) / n,
  };
}

function isSittingSpeed(speedMph) {
  if (speedMph == null) {
    return true;
  }
  return isFiniteNumber(speedMph) && speedMph <= MAX_SIT_SPEED_MPH;
}

function normalizePing(raw) {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
    return { ok: false, reason: "no_vehicle_payload" };
  }

  const vehicleId = String(raw.vehicleId ?? raw.vehicle ?? raw.unit ?? "").trim();
  if (!vehicleId) {
    return { ok: false, reason: "missing_vehicle" };
  }

  const lat = Number(raw.lat ?? raw.latitude);
  const lng = Number(raw.lng ?? raw.longitude ?? raw.lon);
  if (!isValidCoordinate(lat, lng)) {
    return { ok: false, reason: "invalid_coordinates" };
  }

  const recordedAt = parseIsoTime(raw.recordedAt ?? raw.at ?? raw.timestamp ?? Date.now());
  if (!recordedAt) {
    return { ok: false, reason: "invalid_timestamp" };
  }

  const speedRaw = raw.speedMph ?? raw.speed;
  const speedMph = speedRaw == null || speedRaw === "" ? null : Number(speedRaw);
  if (speedMph != null && !isFiniteNumber(speedMph)) {
    return { ok: false, reason: "invalid_speed" };
  }

  return {
    ok: true,
    ping: {
      vehicleId,
      lat,
      lng,
      recordedAt,
      speedMph,
    },
  };
}

function findMatchingLocation(locations, vehicleId, lat, lng) {
  let best = null;
  let bestDistance = Infinity;
  for (const location of locations) {
    if (location.vehicleId !== vehicleId) {
      continue;
    }
    const distance = haversineMeters(lat, lng, location.lat, location.lng);
    if (distance <= CLUSTER_RADIUS_METERS && distance < bestDistance) {
      best = location;
      bestDistance = distance;
    }
  }
  return best;
}

function nextLocationId(locations) {
  const used = new Set(locations.map((location) => location.id));
  let index = locations.length + 1;
  let id = `loc-${String(index).padStart(4, "0")}`;
  while (used.has(id)) {
    index += 1;
    id = `loc-${String(index).padStart(4, "0")}`;
  }
  return id;
}

function applyDwell(state, ping) {
  const nowIso = ping.recordedAt;
  const stays = { ...state.stays };
  const locations = state.locations.map((location) => ({ ...location }));
  const open = stays[ping.vehicleId] ?? null;

  const sitting = isSittingSpeed(ping.speedMph);
  const gapMinutes = open ? minutesBetween(open.lastSeenAt, nowIso) : Infinity;
  const stillHere =
    sitting &&
    open &&
    gapMinutes >= 0 &&
    gapMinutes <= STAY_GAP_MINUTES &&
    haversineMeters(ping.lat, ping.lng, open.lat, open.lng) <= DWELL_RADIUS_METERS;

  if (!stillHere) {
    const nextStay = sitting
      ? {
          vehicleId: ping.vehicleId,
          lat: ping.lat,
          lng: ping.lng,
          arrivedAt: nowIso,
          lastSeenAt: nowIso,
          points: [{ lat: ping.lat, lng: ping.lng }],
        }
      : null;

    if (nextStay) {
      stays[ping.vehicleId] = nextStay;
    } else {
      delete stays[ping.vehicleId];
    }

    return {
      recorded: false,
      reason: sitting ? "dwell_not_long_enough" : "vehicle_moving",
      stay: nextStay,
      locations,
      stays,
    };
  }

  const points = [...open.points, { lat: ping.lat, lng: ping.lng }];
  const center = centroid(points);
  const stay = {
    vehicleId: ping.vehicleId,
    lat: center.lat,
    lng: center.lng,
    arrivedAt: open.arrivedAt,
    lastSeenAt: nowIso,
    points,
  };
  stays[ping.vehicleId] = stay;

  const dwellMinutes = minutesBetween(stay.arrivedAt, stay.lastSeenAt);
  if (dwellMinutes < DWELL_MINUTES) {
    return {
      recorded: false,
      reason: "dwell_not_long_enough",
      stay,
      locations,
      stays,
    };
  }

  const existing = findMatchingLocation(locations, ping.vehicleId, stay.lat, stay.lng);
  if (existing) {
    const blended = centroid([
      { lat: existing.lat, lng: existing.lng },
      { lat: stay.lat, lng: stay.lng },
    ]);
    existing.lat = blended.lat;
    existing.lng = blended.lng;
    existing.lastSeenAt = stay.lastSeenAt;
    existing.dwellMinutes = Math.max(existing.dwellMinutes, Math.round(dwellMinutes * 10) / 10);
    if (new Date(stay.arrivedAt) < new Date(existing.firstSeenAt)) {
      existing.firstSeenAt = stay.arrivedAt;
    }
    if (existing.lastVisitArrivedAt !== stay.arrivedAt) {
      existing.visitCount += 1;
      existing.lastVisitArrivedAt = stay.arrivedAt;
    }
    existing.updatedAt = nowIso;
    return {
      recorded: true,
      reason: "updated_existing",
      location: existing,
      stay,
      locations,
      stays,
    };
  }

  const location = {
    id: nextLocationId(locations),
    vehicleId: ping.vehicleId,
    lat: stay.lat,
    lng: stay.lng,
    firstSeenAt: stay.arrivedAt,
    lastSeenAt: stay.lastSeenAt,
    lastVisitArrivedAt: stay.arrivedAt,
    dwellMinutes: Math.round(dwellMinutes * 10) / 10,
    visitCount: 1,
    updatedAt: nowIso,
  };
  locations.push(location);

  return {
    recorded: true,
    reason: "new_important_location",
    location,
    stay,
    locations,
    stays,
  };
}

function emptyState() {
  return { locations: [], stays: {} };
}

module.exports = {
  DWELL_RADIUS_METERS,
  DWELL_MINUTES,
  CLUSTER_RADIUS_METERS,
  STAY_GAP_MINUTES,
  MAX_SIT_SPEED_MPH,
  normalizePing,
  applyDwell,
  emptyState,
  findMatchingLocation,
  minutesBetween,
};
