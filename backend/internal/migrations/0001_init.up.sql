-- 0001_init — núcleo de identidad y estructura académica mínima.
-- Alcance Parte 1: users, vocerias, universidades, carreras, periodos,
-- refresh_tokens, audit_log, config. El resto del esquema llega por módulo.
-- Timestamp: timestamptz (UTC siempre; la conversión a Caracas es del frontend).

CREATE TABLE users (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email                  text NOT NULL,
    password_hash          text NOT NULL,                -- Argon2id en formato PHC
    nombre                 text NOT NULL,
    rol_base               text NOT NULL DEFAULT 'estudiante'
                           CHECK (rol_base IN ('estudiante', 'profesor')),
    es_mantenedor          boolean NOT NULL DEFAULT false,
    totp_secret            text,                         -- NULL = 2FA desactivado
    reset_token_hash       text,                         -- SHA-256 del token (S2: nunca en claro)
    reset_exp              timestamptz,
    force_password_change  boolean NOT NULL DEFAULT false,
    activo                 boolean NOT NULL DEFAULT true,
    created_at             timestamptz NOT NULL DEFAULT now()
);
-- unicidad de email sin extensión citext: funciona sin superusuario (VPS)
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));

CREATE TABLE universidades (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    nombre         text NOT NULL UNIQUE,
    logo_url       text,
    color_primario text,
    color_acento   text,
    nombre_corto   text
);

CREATE TABLE carreras (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    universidad_id uuid NOT NULL REFERENCES universidades(id) ON DELETE RESTRICT,
    nombre         text NOT NULL,
    logo_url       text,
    tipo_periodo   text NOT NULL DEFAULT 'semestre'
                   CHECK (tipo_periodo IN ('semestre', 'trimestre', 'anual')),
    activo         boolean NOT NULL DEFAULT true,
    UNIQUE (universidad_id, nombre)
);
CREATE INDEX carreras_universidad_idx ON carreras (universidad_id);

CREATE TABLE periodos (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    carrera_id   uuid NOT NULL REFERENCES carreras(id) ON DELETE CASCADE,
    nombre       text NOT NULL,                          -- ej: '2026-1'
    fecha_inicio date NOT NULL,
    fecha_fin    date NOT NULL,
    estado       text NOT NULL DEFAULT 'abierto'
                 CHECK (estado IN ('abierto', 'en_curso', 'cerrado')),
    CHECK (fecha_fin > fecha_inicio),
    UNIQUE (carrera_id, nombre)
);
CREATE INDEX periodos_carrera_estado_idx ON periodos (carrera_id, estado);

CREATE TABLE vocerias (
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nivel        text NOT NULL CHECK (nivel IN ('clase', 'carrera', 'universidad')),
    alcance_id   uuid NOT NULL,                          -- sección|carrera|universidad según nivel
    otorgado_por uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, nivel, alcance_id)
);
CREATE INDEX vocerias_alcance_idx ON vocerias (nivel, alcance_id);

CREATE TABLE refresh_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,                     -- SHA-256; el token en claro solo vive en cookie httpOnly
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,                              -- rotación: al refrescar, el anterior se revoca
    ip         inet,
    user_agent text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);

CREATE TABLE audit_log (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    accion       text NOT NULL,                          -- ej: 'permisos.otorgar_voceria'
    objeto_tipo  text NOT NULL,                          -- ej: 'voceria'
    objeto_id    text,
    antes_json   jsonb,
    despues_json jsonb,
    ip           inet,
    "timestamp"  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_id);
CREATE INDEX audit_log_timestamp_idx ON audit_log ("timestamp");
-- append-only: el backend solo INSERTA y LEE esta tabla, jamás UPDATE/DELETE.

CREATE TABLE config (
    key        text PRIMARY KEY,
    value_json jsonb NOT NULL
);
