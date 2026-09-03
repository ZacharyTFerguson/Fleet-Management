/** OneStep device registry row. Join on factory_id only; display_name is label. */
export type OneStepDevice = {
  factory_id: string;
  device_id: string;
  display_name: string | null;
  linked_car_efleets_id: string | null;
  linked_car_pdi_id: string | null;
  dead: boolean;
  active: boolean;
  retired_at: string | null;
  last_synced_at: string | null;
  created_at: string;
  updated_at: string;
};
