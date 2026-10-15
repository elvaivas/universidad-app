// Package audit: escritura append-only en audit_log. Best-effort: un fallo
// de auditoría se loguea pero nunca rompe la petición del usuario.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Registrar inserta un evento. actorID vacío = evento anónimo (ej: login
// fallido); antes/despues nil = sin estado; ip vacío = NULL.
func Registrar(ctx context.Context, pool *pgxpool.Pool, actorID, accion, objetoTipo, objetoID string, antes, despues any, ip string) {
	var antesB, despuesB []byte
	var err error
	if antes != nil {
		if antesB, err = json.Marshal(antes); err != nil {
			antesB = nil
		}
	}
	if despues != nil {
		if despuesB, err = json.Marshal(despues); err != nil {
			despuesB = nil
		}
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO audit_log (actor_id, accion, objeto_tipo, objeto_id, antes_json, despues_json, ip)
		 VALUES (NULLIF($1,'')::uuid, $2, $3, NULLIF($4,''), $5, $6, NULLIF($7,'')::inet)`,
		actorID, accion, objetoTipo, objetoID, antesB, despuesB, ip)
	if err != nil {
		slog.Error("auditoría: no se pudo registrar evento", "accion", accion, "error", err)
	}
}
