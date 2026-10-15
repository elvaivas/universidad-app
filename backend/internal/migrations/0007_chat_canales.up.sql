-- 0007 — canales del chat: "general" (todos hablan) y "anuncios"
-- (solo profesor, vocero de clase o mantenedor escriben; todos leen).
ALTER TABLE mensajes_chat ADD COLUMN canal text NOT NULL DEFAULT 'general'
    CHECK (canal IN ('general', 'anuncios'));
CREATE INDEX mensajes_chat_canal_idx ON mensajes_chat (seccion_id, canal, created_at);
