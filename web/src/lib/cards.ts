import { readFile } from "fs/promises";
import path from "path";
import type { CardsSnapshot } from "./types";
import { normalizeCardsSnapshot } from "./cardsSnapshot";

export async function loadCardsSnapshot(): Promise<CardsSnapshot> {
  const mirror = path.join(process.cwd(), "data", "cards.json");
  try {
    const raw = await readFile(mirror, "utf8");
    return normalizeCardsSnapshot(JSON.parse(raw) as CardsSnapshot);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
      throw error;
    }
    return normalizeCardsSnapshot(null);
  }
}
