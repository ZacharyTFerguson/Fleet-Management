const EARTH_RADIUS_M = 6_371_000;

function toRadians(degrees) {
  return (degrees * Math.PI) / 180;
}

/** Great-circle distance in meters between two WGS84 points. */
function haversineMeters(lat1, lon1, lat2, lon2) {
  const dLat = toRadians(lat2 - lat1);
  const dLon = toRadians(lon2 - lon1);
  const a =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(toRadians(lat1)) * Math.cos(toRadians(lat2)) * Math.sin(dLon / 2) ** 2;
  return 2 * EARTH_RADIUS_M * Math.asin(Math.min(1, Math.sqrt(a)));
}

function isFiniteNumber(value) {
  return typeof value === "number" && Number.isFinite(value);
}

function isValidCoordinate(lat, lng) {
  return isFiniteNumber(lat) && isFiniteNumber(lng) && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180;
}

module.exports = {
  haversineMeters,
  isFiniteNumber,
  isValidCoordinate,
};
