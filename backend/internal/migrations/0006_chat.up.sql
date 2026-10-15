-- 0006 — chat en tiempo real por sección.
-- Borrado físico + entrada en audit_log (moderación trazable, sin tabla muerta).

CREATE TABLE mensajes_chat (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    seccion_id uuid NOT NULL REFERENCES secciones(id) ON DELETE CASCADE,
    autor_id   uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    cuerpo     text NOT NULL CHECK (char_length(cuerpo) BETWEEN 1 AND 2000),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mensajes_chat_seccion_idx ON mensajes_chat (seccion_id, created_at);
