export type Car = {
  pdi_id: string;
  efleets_id: string;
  nickname: string;
  plate: string;
  vin: string;
  region: string;
  last_oil_miles: number | null;
  last_oil_date: string | null;
  last_reading_miles: number | null;
  last_reading_at: string | null;
  last_reading_source: string | null;
  hold_reason: string | null;
  interval_miles: number;
};

export type Hold = {
  efleets_id: string;
  reason: string;
  detail: string;
  at: string;
};

export type FleetSnapshot = {
  synced_at: string;
  source: "supabase" | "mock-mirror" | "mock-seed";
  cars: Car[];
  holds?: Hold[];
};

export function remainingMiles(car: Car): number | null {
  if (car.hold_reason) return null;
  if (car.last_oil_miles == null || car.last_reading_miles == null) return null;
  const interval = car.interval_miles > 0 ? car.interval_miles : 5000;
  return interval - (car.last_reading_miles - car.last_oil_miles);
}

export function formatMiles(n: number | null | undefined): string {
  if (n == null || Number.isNaN(n)) return "\u2014";
  return n.toLocaleString("en-US");
}
