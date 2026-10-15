-- 0004 — núcleo académico: materias, secciones e inscripciones.
-- La cadena: universidad → carrera → periodo + materia → sección → inscripción.

CREATE TABLE materias (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    carrera_id uuid NOT NULL REFERENCES carreras(id) ON DELETE RESTRICT,
    codigo     text NOT NULL,                       -- ej: 'PS-101'
    nombre     text NOT NULL,
    activo     boolean NOT NULL DEFAULT true,
    UNIQUE (carrera_id, codigo)
);
CREATE INDEX materias_carrera_idx ON materias (carrera_id);

CREATE TABLE secciones (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    materia_id  uuid NOT NULL REFERENCES materias(id) ON DELETE RESTRICT,
    periodo_id  uuid NOT NULL REFERENCES periodos(id) ON DELETE RESTRICT,
    profesor_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    codigo      text NOT NULL,                      -- ej: '1', '2'
    cupo        integer NOT NULL DEFAULT 30 CHECK (cupo > 0),
    inscritos   integer NOT NULL DEFAULT 0 CHECK (inscritos >= 0),
    activo      boolean NOT NULL DEFAULT true,
    CHECK (inscritos <= cupo),
    UNIQUE (materia_id, periodo_id, codigo)
);
CREATE INDEX secciones_periodo_idx ON secciones (periodo_id);
CREATE INDEX secciones_profesor_idx ON secciones (profesor_id);

CREATE TABLE inscripciones (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    seccion_id    uuid NOT NULL REFERENCES secciones(id) ON DELETE CASCADE,
    estudiante_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    estado        text NOT NULL DEFAULT 'activa' CHECK (estado IN ('activa', 'retirada')),
    fecha         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (seccion_id, estudiante_id)
);
CREATE INDEX inscripciones_estudiante_idx ON inscripciones (estudiante_id);
CREATE INDEX inscripciones_seccion_idx ON inscripciones (seccion_id);
