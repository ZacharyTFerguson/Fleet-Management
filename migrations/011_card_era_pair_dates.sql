-- Card↔car era pair dates from GPS pump sits + swipe evidence (never Last Reading).

ALTER TABLE card_eras ADD COLUMN pair_started_at TIMESTAMPTZ;
ALTER TABLE card_eras ADD COLUMN switched_at TIMESTAMPTZ;
ALTER TABLE card_eras ADD COLUMN next_pair_at TIMESTAMPTZ;
