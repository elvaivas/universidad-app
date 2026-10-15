// Package dm: mensajes directos 1-a-1 con solicitud previa.
// Reglas aprobadas en diseño:
//   - profesor o mantenedor inician conversación directa con cualquier usuario activo;
//   - estudiante solo puede contactar (vía solicitud con mensaje de intención)
//     personas con quienes COMPARTE sección — así "el profesor solo recibe
//     de sus estudiantes" es estructural, no configurable;
//   - una solicitud pendiente por par; rechazada puede reintentarse;
//     aceptada crea la conversación única (par ordenado).
package dm

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoEncontrado         = errors.New("no encontrado")
	ErrSinPermiso           = errors.New("sin permiso")
	ErrAutoDM               = errors.New("no puedes enviarte un mensaje a ti mismo")
	ErrSinSeccionCompartida = errors.New("solo puedes contactar personas de tus mismas secciones")
	ErrSolicitudExiste      = errors.New("ya tienes una solicitud pendiente con esta persona")
	ErrYaConversan          = errors.New("ya existe una conversación con esta persona")
	ErrYaRespondida         = errors.New("la solicitud ya fue respondida")
	ErrCuerpoInvalido       = errors.New("el mensaje debe tener entre 1 y 2000 caracteres")
)

type Solicitud struct {
	ID             string `json:"id"`
	SolicitanteID  string `json:"solicitante_id"`
	SolicitanteNom string `json:"solicitante_nombre"`
	SolicitanteEma string `json:"solicitante_email"`
	Mensaje        string `json:"mensaje"`
	Estado         string `json:"estado"`
	CreatedAt      string `json:"created_at"`
}

type Conversacion struct {
	ID      string `json:"id"`
	OtroID  string `json:"otro_id"`
	OtroNom string `json:"otro_nombre"`
	OtroEma string `json:"otro_email"`
	CreadaEn string `json:"creada_en"`
}

type Mensaje struct {
	ID        string `json:"id"`
	AutorID   string `json:"autor_id"`
	AutorNom  string `json:"autor_nombre"`
	Cuerpo    string `json:"cuerpo"`
	CreatedAt string `json:"created_at"`
}

type Servicio struct {
	Pool *pgxpool.Pool
}

// puedeIniciar: estudiante (no profesor, no mantenedor) solo contacta
// gente de sus secciones. Profesores y mantenedores pasan siempre.
func (s *Servicio) puedeIniciar(ctx context.Context, solicitanteID, destinatarioID string) error {
	var privilegiado bool
	if err := s.Pool.QueryRow(ctx,
		`SELECT rol_base = 'profesor' OR es_mantenedor FROM users WHERE id = $1::uuid`, solicitanteID).Scan(&privilegiado); err != nil {
		return ErrNoEncontrado
	}
	if privilegiado {
		return nil
	}
	var ok bool
	if err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM inscripciones i
		    WHERE i.estudiante_id = $1::uuid AND i.estado = 'activa'
		      AND i.seccion_id IN (
		          SELECT seccion_id FROM inscripciones
		          WHERE estudiante_id = $2::uuid AND estado = 'activa'
		          UNION
		          SELECT id FROM secciones WHERE profesor_id = $2::uuid
		      )
		 )`, solicitanteID, destinatarioID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrSinSeccionCompartida
	}
	return nil
}

// CrearSolicitud crea la solicitud con su mensaje de intención.
// Si ya conversan, rechaza en origen con ErrYaConversan (sin solicitudes fantasma).
func (s *Servicio) CrearSolicitud(ctx context.Context, solicitanteID, destinatarioID, mensaje string) (*Solicitud, error) {
	if solicitanteID == destinatarioID {
		return nil, ErrAutoDM
	}
	var activo bool
	if err := s.Pool.QueryRow(ctx,
		`SELECT activo FROM users WHERE id = $1::uuid`, destinatarioID).Scan(&activo); err != nil {
		return nil, ErrNoEncontrado
	}
	if !activo {
		return nil, ErrNoEncontrado
	}
	if err := s.puedeIniciar(ctx, solicitanteID, destinatarioID); err != nil {
		return nil, err
	}
	if ok, _ := s.tienenConversacion(ctx, solicitanteID, destinatarioID); ok {
		return nil, ErrYaConversan
	}
	mensaje = strings.TrimSpace(mensaje)
	if len(mensaje) > 500 {
		mensaje = mensaje[:500]
	}
	var so Solicitud
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO solicitudes_dm (solicitante_id, destinatario_id, mensaje)
		 VALUES ($1::uuid, $2::uuid, $3)
		 RETURNING id::text, created_at::text`,
		solicitanteID, destinatarioID, mensaje).Scan(&so.ID, &so.CreatedAt)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return nil, ErrSolicitudExiste
		}
		return nil, err
	}
	so.SolicitanteID = solicitanteID
	so.Mensaje = mensaje
	so.Estado = "pendiente"
	return &so, nil
}

func (s *Servicio) tienenConversacion(ctx context.Context, a, b string) (bool, error) {
	menor, mayor := ordenarPar(a, b)
	var ok bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM conversaciones_dm WHERE usuario_a = $1::uuid AND usuario_b = $2::uuid)`,
		menor, mayor).Scan(&ok)
	return ok, err
}

// ordenarPar garantiza el par canónico (menor, mayor) por texto uuid.
func ordenarPar(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

// SolicitudesRecibidas: bandeja del destinatario (pendientes).
func (s *Servicio) SolicitudesRecibidas(ctx context.Context, userID string) ([]Solicitud, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT s.id::text, u.id::text, u.nombre, lower(u.email), s.mensaje, s.estado, s.created_at::text
		 FROM solicitudes_dm s JOIN users u ON u.id = s.solicitante_id
		 WHERE s.destinatario_id = $1::uuid AND s.estado = 'pendiente'
		 ORDER BY s.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Solicitud{}
	for rows.Next() {
		var so Solicitud
		if err := rows.Scan(&so.ID, &so.SolicitanteID, &so.SolicitanteNom, &so.SolicitanteEma, &so.Mensaje, &so.Estado, &so.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, so)
	}
	return out, rows.Err()
}

// SolicitudesEnviadas: historial del solicitante.
func (s *Servicio) SolicitudesEnviadas(ctx context.Context, userID string) ([]Solicitud, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT s.id::text, u.id::text, u.nombre, lower(u.email), s.mensaje, s.estado, s.created_at::text
		 FROM solicitudes_dm s JOIN users u ON u.id = s.destinatario_id
		 WHERE s.solicitante_id = $1::uuid
		 ORDER BY s.created_at DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Solicitud{}
	for rows.Next() {
		var so Solicitud
		if err := rows.Scan(&so.ID, &so.SolicitanteID, &so.SolicitanteNom, &so.SolicitanteEma, &so.Mensaje, &so.Estado, &so.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, so)
	}
	return out, rows.Err()
}

// responder marca la solicitud y, si se acepta, crea la conversación única.
func (s *Servicio) responder(ctx context.Context, solicitudID, destinatarioID, nuevoEstado string) (string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sol struct {
		solicitante string
		estado      string
	}
	err = tx.QueryRow(ctx,
		`SELECT solicitante_id::text, estado FROM solicitudes_dm
		 WHERE id = $1::uuid AND destinatario_id = $2::uuid`,
		solicitudID, destinatarioID).Scan(&sol.solicitante, &sol.estado)
	if err != nil {
		return "", ErrNoEncontrado
	}
	if sol.estado != "pendiente" {
		return "", ErrYaRespondida
	}
	if _, err := tx.Exec(ctx,
		`UPDATE solicitudes_dm SET estado = $3, responded_at = now()
		 WHERE id = $1::uuid AND destinatario_id = $2::uuid`,
		solicitudID, destinatarioID, nuevoEstado); err != nil {
		return "", err
	}
	var convID string
	if nuevoEstado == "aceptada" {
		menor, mayor := ordenarPar(sol.solicitante, destinatarioID)
		err = tx.QueryRow(ctx,
			`INSERT INTO conversaciones_dm (usuario_a, usuario_b) VALUES ($1::uuid, $2::uuid)
			 ON CONFLICT (usuario_a, usuario_b) DO NOTHING
			 RETURNING id::text`, menor, mayor).Scan(&convID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				_ = tx.QueryRow(ctx,
					`SELECT id::text FROM conversaciones_dm WHERE usuario_a = $1::uuid AND usuario_b = $2::uuid`,
					menor, mayor).Scan(&convID)
			} else {
				return "", err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return convID, nil
}

func (s *Servicio) Aceptar(ctx context.Context, solicitudID, destinatarioID string) (string, error) {
	return s.responder(ctx, solicitudID, destinatarioID, "aceptada")
}

func (s *Servicio) Rechazar(ctx context.Context, solicitudID, destinatarioID string) error {
	_, err := s.responder(ctx, solicitudID, destinatarioID, "rechazada")
	return err
}

// ListarConversaciones del usuario (con datos del otro participante).
func (s *Servicio) ListarConversaciones(ctx context.Context, userID string) ([]Conversacion, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT c.id::text,
		        CASE WHEN c.usuario_a = $1::uuid THEN c.usuario_b ELSE c.usuario_a END::text,
		        u.nombre, lower(u.email), c.creada_en::text
		 FROM conversaciones_dm c
		 JOIN users u ON u.id = CASE WHEN c.usuario_a = $1::uuid THEN c.usuario_b ELSE c.usuario_a END
		 WHERE c.usuario_a = $1::uuid OR c.usuario_b = $1::uuid
		 ORDER BY c.creada_en DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conversacion{}
	for rows.Next() {
		var c Conversacion
		if err := rows.Scan(&c.ID, &c.OtroID, &c.OtroNom, &c.OtroEma, &c.CreadaEn); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// EsParticipante: solo los dos miembros ven y escriben la conversación.
func (s *Servicio) EsParticipante(ctx context.Context, userID, convID string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM conversaciones_dm
		 WHERE id = $2::uuid AND (usuario_a = $1::uuid OR usuario_b = $1::uuid))`,
		userID, convID).Scan(&ok)
	return ok, err
}

// GuardarMensaje valida membresía y persiste.
func (s *Servicio) GuardarMensaje(ctx context.Context, convID, autorID, cuerpo string) (*Mensaje, error) {
	ok, err := s.EsParticipante(ctx, autorID, convID)
	if err != nil || !ok {
		return nil, ErrSinPermiso
	}
	n := len(strings.TrimSpace(cuerpo))
	if n < 1 || n > 2000 {
		return nil, ErrCuerpoInvalido
	}
	var m Mensaje
	err = s.Pool.QueryRow(ctx,
		`INSERT INTO mensajes_dm (conversacion_id, autor_id, cuerpo)
		 VALUES ($1::uuid, $2::uuid, $3)
		 RETURNING id::text, autor_id::text, cuerpo, created_at::text`,
		convID, autorID, strings.TrimSpace(cuerpo)).Scan(&m.ID, &m.AutorID, &m.Cuerpo, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	m.AutorNom = "(tú)"
	return &m, nil
}

func (s *Servicio) ListarMensajes(ctx context.Context, convID, userID string, limit, offset int) ([]Mensaje, int, error) {
	ok, err := s.EsParticipante(ctx, userID, convID)
	if err != nil || !ok {
		return nil, 0, ErrSinPermiso
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var total int
	if err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM mensajes_dm WHERE conversacion_id = $1::uuid`, convID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT m.id::text, m.autor_id::text, u.nombre, m.cuerpo, m.created_at::text
		 FROM mensajes_dm m JOIN users u ON u.id = m.autor_id
		 WHERE m.conversacion_id = $1::uuid
		 ORDER BY m.created_at ASC, m.id ASC LIMIT $2 OFFSET $3`, convID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Mensaje{}
	for rows.Next() {
		var m Mensaje
		if err := rows.Scan(&m.ID, &m.AutorID, &m.AutorNom, &m.Cuerpo, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// BorrarMensaje: el autor o un mantenedor. Existencia primero.
func (s *Servicio) BorrarMensaje(ctx context.Context, userID, mensajeID string) (convID string, err error) {
	if err := s.Pool.QueryRow(ctx,
		`SELECT conversacion_id::text FROM mensajes_dm WHERE id = $1::uuid`, mensajeID).Scan(&convID); err != nil {
		return "", ErrNoEncontrado
	}
	var puede bool
	if err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM mensajes_dm WHERE id = $1::uuid AND autor_id = $2::uuid)
		     OR EXISTS (SELECT 1 FROM users WHERE id = $2::uuid AND es_mantenedor)`,
		mensajeID, userID).Scan(&puede); err != nil {
		return "", err
	}
	if !puede {
		return "", ErrSinPermiso
	}
	if _, err := s.Pool.Exec(ctx,
		`DELETE FROM mensajes_dm WHERE id = $1::uuid`, mensajeID); err != nil {
		return "", err
	}
	return convID, nil
}
