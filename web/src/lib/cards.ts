import { readFile } from "fs/promises";
import path from "path";
import type { CardsSnapshot } from "./types";

export async function loadCardsSnapshot(): Promise<CardsSnapshot> {
  const mirror = path.join(process.cwd(), "data", "cards.json");
  try {
    const raw = await readFile(mirror, "utf8");
    const parsed = JSON.parse(raw) as CardsSnapshot;
    return {
      ...parsed,
      source: parsed.source || "card-swipes",
      unknown: parsed.unknown ?? [],
      stations: parsed.stations ?? [],
      cars_without_card: parsed.cars_without_card ?? [],
      nicknames: parsed.nicknames ?? {},
      stats: parsed.stats ?? {
        cards: 0,
        stations: 0,
        unknown: 0,
        suspects: 0,
        ambiguous: 0,
        singletons: 0,
        cars_without_card: 0,
        swipes: 0,
      },
    };
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
      throw error;
    }
    return {
      synced_at: new Date().toISOString(),
      source: "mock-seed",
      stats: {
        cards: 0,
        stations: 0,
        unknown: 0,
        suspects: 0,
        ambiguous: 0,
        singletons: 0,
        cars_without_card: 0,
        swipes: 0,
      },
      unknown: [],
      stations: [],
      cars_without_card: [],
      nicknames: {},
    };
  }
}
