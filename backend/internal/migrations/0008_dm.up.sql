-- 0008 — mensajes directos 1-a-1 con solicitud.
-- Reglas de negocio (aprobadas): profesor o mantenedor inician sin solicitud;
-- estudiantes requieren solicitud (con mensaje de intención) y SOLO pueden
-- contactar personas con quienes comparten sección — así "el profesor solo
-- recibe de sus estudiantes" queda garantizado estructuralmente.

CREATE TABLE solicitudes_dm (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    solicitante_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    destinatario_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mensaje         text NOT NULL DEFAULT '',
    estado          text NOT NULL DEFAULT 'pendiente'
                    CHECK (estado IN ('pendiente', 'aceptada', 'rechazada')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    responded_at    timestamptz,
    CHECK (solicitante_id <> destinatario_id)
);
CREATE UNIQUE INDEX solicitudes_dm_pendiente_unica
    ON solicitudes_dm (solicitante_id, destinatario_id) WHERE estado = 'pendiente';
CREATE INDEX solicitudes_dm_dest_idx ON solicitudes_dm (destinatario_id, estado);

CREATE TABLE conversaciones_dm (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_a uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    usuario_b uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    creada_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (usuario_a, usuario_b),
    CHECK (usuario_a <> usuario_b)
);

CREATE TABLE mensajes_dm (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversacion_id uuid NOT NULL REFERENCES conversaciones_dm(id) ON DELETE CASCADE,
    autor_id        uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    cuerpo          text NOT NULL CHECK (char_length(cuerpo) BETWEEN 1 AND 2000),
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mensajes_dm_conv_idx ON mensajes_dm (conversacion_id, created_at);
