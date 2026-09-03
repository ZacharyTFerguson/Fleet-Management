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
