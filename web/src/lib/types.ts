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

export type CardNeighbor = {
  efleets_id: string;
  card_id: string;
  station: string;
  days_apart: number;
  at: string;
};

export type UnknownMatchup = {
  kind: "suspect" | "ambiguous" | "singleton" | string;
  card_id: string;
  enterprise_car: string;
  best_car: string;
  best_n: number;
  best_score: number;
  runner_up_car?: string;
  runner_up_n?: number;
  latest_station: string;
  latest_at: string;
  neighbors?: CardNeighbor[];
  why: string;
};

export type StationSummary = {
  key: string;
  name: string;
  address: string;
  swipes: number;
  cars: number;
  cards: number;
};

export type GPSCardMatch = {
  efleets_id: string;
  card_id: string;
  evidence_n: number;
  stations?: string[];
  enterprise_cars?: string[];
  best: boolean;
};

export type CardEra = {
  card_id: string;
  efleets_id: string;
  nickname?: string;
  from: string;
  to: string;
  evidence_n: number;
  stations?: string[];
  split: boolean;
};

export type RecordCall = {
  card_id: string;
  at: string;
  station?: string;
  enterprise_car?: string;
  called_car: string;
  called_name?: string;
  why: string;
};

export type CardsSnapshot = {
  synced_at: string;
  source: string;
  stats: {
    cards: number;
    stations: number;
    unknown: number;
    suspects: number;
    ambiguous: number;
    singletons: number;
    cars_without_card: number;
    swipes: number;
    gps_best?: number;
    gps_matches?: number;
    gps_splits?: number;
    gps_calls?: number;
    gps_disagree?: number;
  };
  unknown: UnknownMatchup[];
  stations: StationSummary[];
  cars_without_card: string[];
  gps_best?: GPSCardMatch[];
  eras?: CardEra[];
  calls?: RecordCall[];
  nicknames?: Record<string, string>;
};

export function remainingMiles(car: Car): number | null {
  if (car.hold_reason) return null;
  if (car.last_oil_miles == null || car.last_reading_miles == null) return null;
  const interval = car.interval_miles > 0 ? car.interval_miles : 5000;
  return interval - (car.last_reading_miles - car.last_oil_miles);
}

export function formatMiles(n: number | null | undefined): string {
  if (n == null || Number.isNaN(n)) return "—";
  return n.toLocaleString("en-US");
}
