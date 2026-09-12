-- ready for Supabase project fleet-oil
-- RLS on; deny anon/authenticated. CLI uses service role from env only.

ALTER TABLE cars ENABLE ROW LEVEL SECURITY;
ALTER TABLE cards ENABLE ROW LEVEL SECURITY;
ALTER TABLE gas_stations ENABLE ROW LEVEL SECURITY;
ALTER TABLE maintenance_locations ENABLE ROW LEVEL SECURITY;
ALTER TABLE fills ENABLE ROW LEVEL SECURITY;
ALTER TABLE shop_ros ENABLE ROW LEVEL SECURITY;
ALTER TABLE onestep_devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE drive_stop_miles ENABLE ROW LEVEL SECURITY;
ALTER TABLE hold_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE oil_changes ENABLE ROW LEVEL SECURITY;

CREATE POLICY deny_anon_cars ON cars FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_cars ON cars FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_cards ON cards FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_cards ON cards FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_gas ON gas_stations FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_gas ON gas_stations FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_ml ON maintenance_locations FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_ml ON maintenance_locations FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_fills ON fills FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fills ON fills FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_ros ON shop_ros FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_ros ON shop_ros FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_dev ON onestep_devices FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_dev ON onestep_devices FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_dsm ON drive_stop_miles FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_dsm ON drive_stop_miles FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_hold ON hold_events FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_hold ON hold_events FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_oc ON oil_changes FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_oc ON oil_changes FOR ALL TO authenticated USING (false);

ALTER TABLE card_transactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE card_pairings ENABLE ROW LEVEL SECURITY;
CREATE POLICY deny_anon_cardtx ON card_transactions FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_cardtx ON card_transactions FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_cardpair ON card_pairings FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_cardpair ON card_pairings FOR ALL TO authenticated USING (false);

-- Later oilchange tables (005–011). Neon skips missing anon; owner still bypasses RLS.
ALTER TABLE card_eras ENABLE ROW LEVEL SECURITY;
ALTER TABLE transaction_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE assignment_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE desk_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE vault_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE places ENABLE ROW LEVEL SECURITY;
ALTER TABLE gas_station_marker_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE desk_heartbeats ENABLE ROW LEVEL SECURITY;
ALTER TABLE mileage_ledger ENABLE ROW LEVEL SECURITY;
ALTER TABLE drive_stop_windows ENABLE ROW LEVEL SECURITY;
ALTER TABLE place_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE place_brands ENABLE ROW LEVEL SECURITY;
ALTER TABLE toptier_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE toptier_grades ENABLE ROW LEVEL SECURITY;

CREATE POLICY deny_anon_eras ON card_eras FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_eras ON card_eras FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_asg ON transaction_assignments FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_asg ON transaction_assignments FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_asgev ON assignment_events FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_asgev ON assignment_events FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_deskuser ON desk_users FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_deskuser ON desk_users FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_vault ON vault_secrets FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_vault ON vault_secrets FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_places ON places FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_places ON places FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_marker ON gas_station_marker_jobs FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_marker ON gas_station_marker_jobs FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_hb ON desk_heartbeats FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_hb ON desk_heartbeats FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_ledger ON mileage_ledger FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_ledger ON mileage_ledger FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_dsw ON drive_stop_windows FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_dsw ON drive_stop_windows FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_ptypes ON place_types FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_ptypes ON place_types FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_pbrands ON place_brands FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_pbrands ON place_brands FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_ttcodes ON toptier_codes FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_ttcodes ON toptier_codes FOR ALL TO authenticated USING (false);
CREATE POLICY deny_anon_ttgrades ON toptier_grades FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_ttgrades ON toptier_grades FOR ALL TO authenticated USING (false);
