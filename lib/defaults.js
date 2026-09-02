/** Sitting still: stay inside this radius. */
const DWELL_RADIUS_METERS = 100;

/** How long a vehicle must sit before the place is important. */
const DWELL_MINUTES = 15;

/** Merge later visits to the same vehicle + nearby place. */
const CLUSTER_RADIUS_METERS = 150;

/** Close the open sit if no ping arrives within this many minutes. */
const STAY_GAP_MINUTES = 60;

/** Treat speeds at or below this as parked / creeping in a lot. */
const MAX_SIT_SPEED_MPH = 3;

module.exports = {
  DWELL_RADIUS_METERS,
  DWELL_MINUTES,
  CLUSTER_RADIUS_METERS,
  STAY_GAP_MINUTES,
  MAX_SIT_SPEED_MPH,
};
