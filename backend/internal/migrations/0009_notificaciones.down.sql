-- 0009 — reversión.
ALTER TABLE avisos DROP COLUMN IF EXISTS importante;
DROP TABLE IF EXISTS anuncios_generales, notificaciones CASCADE;
