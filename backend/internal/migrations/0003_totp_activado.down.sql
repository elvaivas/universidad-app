-- 0003 — reversión.
ALTER TABLE users DROP COLUMN IF EXISTS totp_activado;
