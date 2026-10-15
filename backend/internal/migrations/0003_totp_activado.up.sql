-- 0003 — 2FA TOTP: el secreto puede existir en estado pendiente (aún no exige código).
-- Solo usuarios con totp_activado=true pasan por el segundo factor al login.
ALTER TABLE users ADD COLUMN totp_activado boolean NOT NULL DEFAULT false;
