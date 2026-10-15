-- 0005 — feed de sección: avisos, votaciones (secreto mínimo viable),
-- comentarios. Separación votos/votantes: votos NO guarda quién votó
-- qué; votantes solo marca participación (una vez por persona).

CREATE TABLE avisos (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    seccion_id uuid NOT NULL REFERENCES secciones(id) ON DELETE CASCADE,
    autor_id   uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    tipo       text NOT NULL DEFAULT 'aviso' CHECK (tipo IN ('aviso', 'votacion')),
    titulo     text NOT NULL,
    cuerpo     text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX avisos_seccion_idx ON avisos (seccion_id, created_at DESC);

CREATE TABLE votaciones (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    aviso_id  uuid NOT NULL UNIQUE REFERENCES avisos(id) ON DELETE CASCADE,
    pregunta  text NOT NULL,
    opciones  jsonb NOT NULL,
    multiple  boolean NOT NULL DEFAULT false,
    cerrada   boolean NOT NULL DEFAULT false,
    creada    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE votos (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    votacion_id  uuid NOT NULL REFERENCES votaciones(id) ON DELETE CASCADE,
    opcion_idx   integer NOT NULL,
    peso         integer NOT NULL DEFAULT 1,
    emitido      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX votos_votacion_idx ON votos (votacion_id);

CREATE TABLE votantes (
    votacion_id  uuid NOT NULL REFERENCES votaciones(id) ON DELETE CASCADE,
    estudiante_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emitido      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (votacion_id, estudiante_id)
);

CREATE TABLE comentarios (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    aviso_id   uuid NOT NULL REFERENCES avisos(id) ON DELETE CASCADE,
    autor_id   uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    cuerpo     text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX comentarios_aviso_idx ON comentarios (aviso_id, created_at);
