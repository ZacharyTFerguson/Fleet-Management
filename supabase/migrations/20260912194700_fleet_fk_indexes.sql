-- Covering indexes for fleet_places FKs and existing desk FKs.
-- Never apply to XRAY. Reception/Users untouched.

CREATE INDEX IF NOT EXISTS fleet_places_type_idx ON fleet_places (type_code);
CREATE INDEX IF NOT EXISTS fleet_places_toptier_idx ON fleet_places (toptier);
CREATE INDEX IF NOT EXISTS fleet_places_toptier_grade_idx ON fleet_places (toptier_grade);
CREATE INDEX IF NOT EXISTS fleet_cards_linked_car_idx ON fleet_cards (linked_car_efleets_id);
CREATE INDEX IF NOT EXISTS fleet_onestep_devices_linked_car_idx ON fleet_onestep_devices (linked_car_efleets_id);
