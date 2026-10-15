// Package chat: mensajería en tiempo real por sección.
// Canales: "general" (todos hablan) y "anuncios" (solo profesor, vocero de
// clase o mantenedor escriben; todos leen). Moderación: el autor borra lo
// suyo; el profesor de la sección o un mantenedor, cualquiera. El borrado es
// físico y queda trazado en audit_log.
package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoEncontrado   = errors.New("mensaje no encontrado")
	ErrSinPermiso     = errors.New("sin permiso para moderar este mensaje")
	ErrCuerpoInvalido = errors.New("el mensaje debe tener entre 1 y 2000 caracteres")
)

type Mensaje struct {
	ID        string `json:"id"`
	SeccionID string `json:"seccion_id"`
	AutorID   string `json:"autor_id"`
	AutorNom  string `json:"autor_nombre"`
	AutorRol  string `json:"autor_rol,omitempty"` // profesor | vocero | mantenedor
	Canal     string `json:"canal"`
	Cuerpo    string `json:"cuerpo"`
	CreatedAt string `json:"created_at"`
}

// Canales soportados por la sala de una sección.
const (
	CanalGeneral  = "general"
	CanalAnuncios = "anuncios"
)

type Servicio struct {
	Pool *pgxpool.Pool
}

func (s *Servicio) validarCuerpo(cuerpo string) error {
	n := len(strings.TrimSpace(cuerpo))
	if n < 1 || n > 2000 {
		return ErrCuerpoInvalido
	}
	return nil
}

// EsMiembro replica la semántica del feed (módulos desacoplados, misma regla).
func (s *Servicio) EsMiembro(ctx context.Context, userID, seccionID string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM secciones sec
		    WHERE sec.id = $2::uuid AND sec.profesor_id = $1::uuid
		 ) OR EXISTS (
		    SELECT 1 FROM inscripciones i
		    WHERE i.seccion_id = $2::uuid AND i.estudiante_id = $1::uuid AND i.estado = 'activa'
		 ) OR EXISTS (
		    SELECT 1 FROM vocerias v
		    WHERE v.user_id = $1::uuid AND v.nivel = 'clase' AND v.alcance_id = $2::uuid
		 ) OR EXISTS (
		    SELECT 1 FROM users u WHERE u.id = $1::uuid AND u.es_mantenedor
		 )`, userID, seccionID).Scan(&ok)
	return ok, err
}

// PuedePublicar: profesor de la sección, vocero de clase del alcance o
// mantenedor. Es la guarda del canal "anuncios".
func (s *Servicio) PuedePublicar(ctx context.Context, userID, seccionID string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM secciones sec
		    WHERE sec.id = $2::uuid AND sec.profesor_id = $1::uuid
		 ) OR EXISTS (
		    SELECT 1 FROM vocerias v
		    WHERE v.user_id = $1::uuid AND v.nivel = 'clase' AND v.alcance_id = $2::uuid
		 ) OR EXISTS (
		    SELECT 1 FROM users u WHERE u.id = $1::uuid AND u.es_mantenedor
		 )`, userID, seccionID).Scan(&ok)
	return ok, err
}

// InsigniaPublica resuelve el rol visible del autor dentro de esta sección
// (exportada para la capa HTTP).
func InsigniaPublica(ctx context.Context, pool *pgxpool.Pool, userID, seccionID string) string {
	var rol string
	_ = pool.QueryRow(ctx,
		`SELECT CASE
		    WHEN u.es_mantenedor THEN 'mantenedor'
		    WHEN sec.profesor_id = $1::uuid THEN 'profesor'
		    WHEN EXISTS (SELECT 1 FROM vocerias v WHERE v.user_id = $1::uuid
		                 AND v.nivel = 'clase' AND v.alcance_id = $2::uuid) THEN 'vocero'
		    ELSE ''
		END
		FROM users u CROSS JOIN (SELECT profesor_id FROM secciones WHERE id = $2::uuid) sec
		WHERE u.id = $1::uuid`, userID, seccionID).Scan(&rol)
	return rol
}

// Guardar persiste el mensaje y lo devuelve listo para broadcast.
// En el canal "anuncios" exige PuedePublicar (profesor/vocero/mantenedor).
func (s *Servicio) Guardar(ctx context.Context, seccionID, autorID, cuerpo, canal string) (*Mensaje, error) {
	if err := s.validarCuerpo(cuerpo); err != nil {
		return nil, err
	}
	if canal != CanalGeneral && canal != CanalAnuncios {
		return nil, ErrCuerpoInvalido
	}
	if canal == CanalAnuncios {
		ok, err := s.PuedePublicar(ctx, autorID, seccionID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrSinPermiso
		}
	}
	cuerpo = strings.TrimSpace(cuerpo)
	var m Mensaje
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO mensajes_chat (seccion_id, autor_id, cuerpo, canal)
		 VALUES ($1::uuid, $2::uuid, $3, $4)
		 RETURNING id::text, seccion_id::text, autor_id::text, canal, cuerpo, created_at::text`,
		seccionID, autorID, cuerpo, canal).Scan(&m.ID, &m.SeccionID, &m.AutorID, &m.Canal, &m.Cuerpo, &m.CreatedAt)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23503" {
			return nil, ErrNoEncontrado
		}
		if errors.As(err, &pg) && pg.Code == "23514" {
			return nil, ErrCuerpoInvalido
		}
		return nil, err
	}
	m.AutorNom = "(tú)"
	return &m, nil
}

// Listar devuelve el historial de un canal en orden cronológico
// (el chat se lee de arriba abajo), con la insignia de rol de cada autor.
func (s *Servicio) Listar(ctx context.Context, seccionID, canal string, limit, offset int) ([]Mensaje, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if canal != CanalAnuncios {
		canal = CanalGeneral
	}
	var total int
	if err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM mensajes_chat WHERE seccion_id=$1::uuid AND canal=$2`, seccionID, canal).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT m.id::text, m.seccion_id::text, m.autor_id::text, u.nombre, m.canal, m.cuerpo, m.created_at::text
		 FROM mensajes_chat m JOIN users u ON u.id = m.autor_id
		 WHERE m.seccion_id=$1::uuid AND m.canal=$2
		 ORDER BY m.created_at ASC, m.id ASC
		 LIMIT $3 OFFSET $4`, seccionID, canal, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Mensaje{}
	for rows.Next() {
		var m Mensaje
		if err := rows.Scan(&m.ID, &m.SeccionID, &m.AutorID, &m.AutorNom, &m.Canal, &m.Cuerpo, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		m.AutorRol = InsigniaPublica(ctx, s.Pool, m.AutorID, seccionID)
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// PuedeModerar: autor del mensaje, profesor de la sección o mantenedor.
func (s *Servicio) PuedeModerar(ctx context.Context, userID, mensajeID string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM mensajes_chat m WHERE m.id = $2::uuid AND m.autor_id = $1::uuid
		 ) OR EXISTS (
		    SELECT 1 FROM mensajes_chat m
		    JOIN secciones sec ON sec.id = m.seccion_id
		    WHERE m.id = $2::uuid AND sec.profesor_id = $1::uuid
		 ) OR EXISTS (
		    SELECT 1 FROM users u WHERE u.id = $1::uuid AND u.es_mantenedor
		 )`, userID, mensajeID).Scan(&ok)
	return ok, err
}

// Borrar elimina el mensaje (tras verificar moderación) y devuelve su sección
// para notificar el borrado a la sala. La existencia se comprueba primero:
// así un mensaje inexistente siempre responde ErrNoEncontrado (no filtra
// permisos sobre recursos que no existen).
func (s *Servicio) Borrar(ctx context.Context, userID, mensajeID string) (seccionID string, err error) {
	if err := s.Pool.QueryRow(ctx,
		`SELECT seccion_id::text FROM mensajes_chat WHERE id = $1::uuid`, mensajeID).Scan(&seccionID); err != nil {
		return "", ErrNoEncontrado
	}
	ok, err := s.PuedeModerar(ctx, userID, mensajeID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrSinPermiso
	}
	if _, err := s.Pool.Exec(ctx,
		`DELETE FROM mensajes_chat WHERE id = $1::uuid`, mensajeID); err != nil {
		return "", err
	}
	return seccionID, nil
}

var _ = fmt.Sprintf
var _ = time.Now
