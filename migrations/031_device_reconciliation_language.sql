-- Align device_reconciliation with the device-authority ubiquitous language
-- (docs/authority-reference-module-2026-10-05/UBIQUITOUS_LANGUAGE.md):
--   * opening_boot_id always equalled boot_id, so it carried no information;
--   * authority_epoch is the owner epoch every other table calls owner_epoch;
--   * opened_at recorded when the device's first state was seen, not when a
--     reconciliation opened.
ALTER TABLE device_reconciliation DROP COLUMN opening_boot_id;
ALTER TABLE device_reconciliation RENAME COLUMN authority_epoch TO owner_epoch;
ALTER TABLE device_reconciliation RENAME COLUMN opened_at TO first_seen_at;
