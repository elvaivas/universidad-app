-- 0007 — reversión.
DROP INDEX IF EXISTS mensajes_chat_canal_idx;
ALTER TABLE mensajes_chat DROP COLUMN IF EXISTS canal;
