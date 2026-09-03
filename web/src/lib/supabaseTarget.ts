export const FLEET_SUPABASE_REF = "hdtwfdjdvdzdxfdriyzn";
export const XRAY_SUPABASE_REF = "chjqcznyxvtjbamttqdj";
export const FLEET_SUPABASE_HOST = `${FLEET_SUPABASE_REF}.supabase.co`;

/** Allow only Zachary's fleet project (or loopback). Never XRAY. */
export function assertFleetSupabaseURL(raw: string): void {
  const lowered = raw.toLowerCase();
  if (lowered.includes("xray") || lowered.includes(XRAY_SUPABASE_REF)) {
    throw new Error("Refusing XRAY Supabase project for fleet-oil data");
  }
  let parsed: URL;
  try {
    parsed = new URL(raw.includes("://") ? raw : `https://${raw}`);
  } catch {
    throw new Error("Refusing invalid Supabase URL for fleet-oil data");
  }
  const host = parsed.hostname.toLowerCase().replace(/\.$/, "");
  if (host === "localhost" || host === "127.0.0.1" || host === "::1") {
    return;
  }
  if (host === FLEET_SUPABASE_HOST) {
    if (parsed.protocol !== "https:") {
      throw new Error("Refusing non-https fleet Supabase URL");
    }
    return;
  }
  throw new Error("Refusing non-fleet Supabase host for fleet-oil data");
}
