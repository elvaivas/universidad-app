-- 0002 — baja lógica de universidades (el catálogo la necesita).
ALTER TABLE universidades ADD COLUMN activo boolean NOT NULL DEFAULT true;
