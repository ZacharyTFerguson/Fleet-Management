import { createClient, type SupabaseClient } from "@supabase/supabase-js";
import { readFile } from "fs/promises";
import path from "path";
import type { FleetSnapshot } from "./types";

function supabase(): SupabaseClient | null {
  const url = process.env.NEXT_PUBLIC_SUPABASE_URL?.trim();
  const key =
    process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY?.trim() ||
    process.env.SUPABASE_ANON_KEY?.trim();
  if (!url || !key) return null;
  // Guard: never point the oil UI at the XRAY project by accident.
  if (/xray/i.test(url) || url.includes("chjqcznyxvtjbamttqdj")) {
    throw new Error("Refusing XRAY Supabase project for fleet-oil data");
  }
  return createClient(url, key);
}

export async function loadFleetSnapshot(): Promise<FleetSnapshot> {
  const client = supabase();
  if (client) {
    // Shared project uses fleet_cars (never bare cars — that would collide with other apps).
    const { data, error } = await client
      .from("fleet_cars")
      .select(
        "pdi_id,efleets_id,nickname,plate,vin,region,last_oil_miles,last_oil_date,last_reading_miles,last_reading_at,last_reading_source,hold_reason,interval_miles,updated_at",
      )
      .order("efleets_id");
    if (error) throw new Error(error.message);
    const newestUpdate = (data ?? []).reduce((latest, row) => {
      const updated = Date.parse(row.updated_at ?? "");
      return Number.isFinite(updated) && updated > latest ? updated : latest;
    }, 0);
    return {
      synced_at:
        newestUpdate > 0
          ? new Date(newestUpdate).toISOString()
          : new Date().toISOString(),
      source: "supabase",
      cars: data ?? [],
    };
  }

  const mirror = path.join(process.cwd(), "data", "cars.json");
  try {
    const raw = await readFile(mirror, "utf8");
    const parsed = JSON.parse(raw) as FleetSnapshot;
    return {
      ...parsed,
      source: parsed.source || "mock-mirror",
      cars: parsed.cars ?? [],
    };
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
      throw error;
    }
    return {
      synced_at: new Date().toISOString(),
      source: "mock-seed",
      cars: [],
    };
  }
}
