-- 0009 — centro de atención: notificaciones, anuncios generales y avisos importantes.

CREATE TABLE notificaciones (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tipo       text NOT NULL,                 -- dm_solicitud | chat_anuncio | aviso_importante | anuncio_general | votacion
    ref_id     text NOT NULL DEFAULT '',      -- id de la solicitud / sección / aviso / anuncio
    titulo     text NOT NULL,                 -- texto corto que se muestra en la campanita
    leida      boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notificaciones_usuario_idx ON notificaciones (usuario_id, leida, created_at DESC);

-- Anuncios de la universidad: los publica cualquier mantenedor; SIEMPRE visibles arriba del inicio de todos.
CREATE TABLE anuncios_generales (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    autor_id   uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    titulo     text NOT NULL,
    cuerpo     text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- El profesor/vocero/mantenedor marca un aviso como importante → sale en el inicio de los miembros.
ALTER TABLE avisos ADD COLUMN importante boolean NOT NULL DEFAULT false;
