import type { CardsSnapshot } from "./types";

const emptyStats: CardsSnapshot["stats"] = {
  cards: 0,
  stations: 0,
  unknown: 0,
  suspects: 0,
  ambiguous: 0,
  singletons: 0,
  cars_without_card: 0,
  swipes: 0,
};

// oilchange serve writes Go JSON; nil slices become null. Never crash Card Desk on that.
export function normalizeCardsSnapshot(parsed: Partial<CardsSnapshot> | null | undefined): CardsSnapshot {
  if (!parsed) {
    return {
      synced_at: new Date().toISOString(),
      source: "mock-seed",
      stats: { ...emptyStats },
      unknown: [],
      stations: [],
      cars_without_card: [],
      gps_best: [],
      nicknames: {},
    };
  }
  return {
    ...parsed,
    synced_at: parsed.synced_at || new Date().toISOString(),
    source: parsed.source || "card-swipes",
    unknown: parsed.unknown ?? [],
    stations: parsed.stations ?? [],
    cars_without_card: parsed.cars_without_card ?? [],
    gps_best: parsed.gps_best ?? [],
    eras: parsed.eras ?? [],
    calls: parsed.calls ?? [],
    nicknames: parsed.nicknames ?? {},
    stats: { ...emptyStats, ...(parsed.stats ?? {}) },
  };
}
