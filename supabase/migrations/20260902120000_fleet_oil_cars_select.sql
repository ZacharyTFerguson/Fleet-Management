-- Shared ZacharyTFerguson's Project (hdtwfdjdvdzdxfdriyzn) — fleet_* tables.
-- Never apply to XRAY (chjqcznyxvtjbamttqdj).
-- UI anon SELECT on fleet_cars; writes via service role or fleet-sync edge function.

-- See migrations/005_shared_project_fleet_prefix.sql for the full shared-project DDL+RLS.
-- This file documents the SELECT policies expected by the Next UI.

DROP POLICY IF EXISTS fleet_cars_select_anon ON fleet_cars;
DROP POLICY IF EXISTS fleet_cars_select_authenticated ON fleet_cars;

CREATE POLICY fleet_cars_select_anon ON fleet_cars FOR SELECT TO anon USING (true);
CREATE POLICY fleet_cars_select_authenticated ON fleet_cars FOR SELECT TO authenticated USING (true);
-- No INSERT/UPDATE/DELETE policies for anon/authenticated: sync uses service role or fleet-sync.
