export type FillBlock = {
  tx_key: string;
  card_id: string;
  at: string;
  station?: string;
  address?: string;
  gallons?: number | null;
  odometer?: number | null;
  driver?: string;
  enterprise_efleets_id?: string;
  enterprise_pdi_id?: string;
  enterprise_name?: string;
  gps_called_efleets_id?: string;
  gps_called_name?: string;
  assigned_efleets_id?: string;
  assigned_pdi_id?: string;
  assigned_name?: string;
  gps_disagrees?: boolean;
  source?: string;
};

export type HistoryCar = {
  pdi_id: string;
  efleets_id: string;
  nickname: string;
  region: string;
  plate?: string;
  fills: FillBlock[];
};

export type HistoryBoard = {
  synced_at: string;
  region: string;
  regions: string[];
  cars: HistoryCar[];
  unassigned: FillBlock[];
  assigned_n: number;
  unassigned_n: number;
  gps_flag_n: number;
  swipes: number;
};

export type FillEvidence = {
  tx_key: string;
  card_id: string;
  at: string;
  station?: string;
  enterprise_efleets_id?: string;
  assigned_efleets_id?: string;
  gps_called_efleets_id?: string;
  gps_disagrees?: boolean;
  nearby: {
    efleets_id: string;
    nickname?: string;
    pdi_id?: string;
    factory_id?: string;
    device_id?: string;
    from: string;
    to: string;
    has_pos?: boolean;
  }[];
  live_note?: string;
};

export type BoxEvidence = {
  factory_id: string;
  device_id: string;
  display_name?: string;
  vin?: string;
  linked_car_efleets_id?: string;
  linked_car_pdi_id?: string;
  active?: boolean;
  stops_in_cache?: number;
};

export function emptyBoard(): HistoryBoard {
  return {
    synced_at: new Date().toISOString(),
    region: "",
    regions: [],
    cars: [],
    unassigned: [],
    assigned_n: 0,
    unassigned_n: 0,
    gps_flag_n: 0,
    swipes: 0,
  };
}
