// Package notifs: centro de atención (campanita). Una fila por destinatario;
// el origen (tipo + ref_id) permite marcar leída automáticamente al abrir el
// sitio correspondiente. Historial: se muestran las leídas recientes y las
// mayores de 30 días se depuran (anti-saturación).
// Es un paquete HOJA: los demás módulos (dm, chat, feed, catalog) lo llaman
// para emitir; nadie depende de ellos.
package notifs

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Tipos de notificación (extensibles: notas, examenes, asistencia…).
const (
	TipoDMSolicitud     = "dm_solicitud"
	TipoChatAnuncio     = "chat_anuncio"
	TipoAvisoImportante = "aviso_importante"
	TipoAnuncioGeneral  = "anuncio_general"
	TipoVotacion        = "votacion"
)

// Retención del historial: más viejo que esto se borra (anti-saturación).
const retencion = 30 * 24 * time.Hour

// Crear inserta una notificación para un usuario.
func Crear(ctx context.Context, pool *pgxpool.Pool, usuarioID, tipo, refID, titulo string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO notificaciones (usuario_id, tipo, ref_id, titulo) VALUES ($1::uuid, $2, $3, $4)`,
		usuarioID, tipo, refID, titulo)
	return err
}

// CrearParaMiembrosSeccion notifica a todos los miembros de una sección
// (inscritos activos + profesor). El remitente no se notifica a sí mismo.
func CrearParaMiembrosSeccion(ctx context.Context, pool *pgxpool.Pool, seccionID, autorID, tipo, refID, titulo string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO notificaciones (usuario_id, tipo, ref_id, titulo)
		 SELECT DISTINCT i.estudiante_id, $3, $4, $5
		 FROM inscripciones i
		 WHERE i.seccion_id = $1::uuid AND i.estado = 'activa' AND i.estudiante_id <> $2::uuid
		 UNION
		 SELECT sec.profesor_id, $3, $4, $5
		 FROM secciones sec
		 WHERE sec.id = $1::uuid AND sec.profesor_id <> $2::uuid`,
		seccionID, autorID, tipo, refID, titulo)
	return err
}

// CrearParaTodos notifica a todos los usuarios activos (anuncios generales).
func CrearParaTodos(ctx context.Context, pool *pgxpool.Pool, autorID, tipo, refID, titulo string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO notificaciones (usuario_id, tipo, ref_id, titulo)
		 SELECT id, $2, $3, $4 FROM users WHERE activo AND id <> $1::uuid`,
		autorID, tipo, refID, titulo)
	return err
}

type Notificacion struct {
	ID        string `json:"id"`
	Tipo      string `json:"tipo"`
	RefID     string `json:"ref_id"`
	Titulo    string `json:"titulo"`
	Leida     bool   `json:"leida"`
	CreatedAt string `json:"created_at"`
}

type Panel struct {
	NoLeidas      []Notificacion `json:"no_leidas"`
	Recientes     []Notificacion `json:"recientes"` // leídas recientes (historial corto)
	TotalNoLeidas int            `json:"total_no_leidas"`
}

// Listar devuelve el panel: no leídas + últimas 20 leídas (el historial visible).
// Depura lo más viejo que la retención (llamada barata, una por vista de panel).
func Listar(ctx context.Context, pool *pgxpool.Pool, userID string) (*Panel, error) {
	_, _ = pool.Exec(ctx, `DELETE FROM notificaciones WHERE created_at < now() - $1::interval`, retencion.String())

	p := &Panel{NoLeidas: []Notificacion{}, Recientes: []Notificacion{}}
	rows, err := pool.Query(ctx,
		`SELECT id::text, tipo, ref_id, titulo, leida, created_at::text
		 FROM notificaciones WHERE usuario_id = $1::uuid
		 ORDER BY created_at DESC LIMIT 60`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	leidas := []Notificacion{}
	for rows.Next() {
		var n Notificacion
		if err := rows.Scan(&n.ID, &n.Tipo, &n.RefID, &n.Titulo, &n.Leida, &n.CreatedAt); err != nil {
			return nil, err
		}
		if n.Leida {
			leidas = append(leidas, n)
		} else {
			p.NoLeidas = append(p.NoLeidas, n)
		}
	}
	p.TotalNoLeidas = len(p.NoLeidas)
	if len(leidas) > 20 {
		leidas = leidas[:20]
	}
	p.Recientes = leidas
	return p, rows.Err()
}

// MarcarLeidaPorRef marca leídas las notificaciones del usuario que apuntan
// al sitio abierto (tipo + ref). Es el "marcado automático al abrir".
func MarcarLeidaPorRef(ctx context.Context, pool *pgxpool.Pool, userID, tipo, refID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE notificaciones SET leida = true
		 WHERE usuario_id = $1::uuid AND tipo = $2 AND ref_id = $3`,
		userID, tipo, refID)
	return err
}

// MarcarLeida marca una notificación concreta (tocar el ítem en la campanita).
func MarcarLeida(ctx context.Context, pool *pgxpool.Pool, userID, notifID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE notificaciones SET leida = true WHERE id = $1::uuid AND usuario_id = $2::uuid`,
		notifID, userID)
	return err
}
