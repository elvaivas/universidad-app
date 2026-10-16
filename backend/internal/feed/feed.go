// Package feed: avisos, votaciones y comentarios por sección.
// Reglas: lectura solo para miembros (inscritos activos, profesor, vocero de
// clase o mantenedor). Publicación: profesor, vocero con vocería de clase
// sobre la sección, o mantenedor.
// Votaciones con secreto mínimo viable (plano): los votos se guardan
// DESVINCULADOS de la identidad; solo se registra quién ya participó.
package feed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/elvaivas/universidad-app/backend/internal/notifs"
)

var (
	ErrNoEncontrado    = errors.New("no encontrado")
	ErrDatosInvalidos  = errors.New("datos inválidos")
	ErrYaVoto          = errors.New("ya participaste en esta votación")
	ErrVotacionCerrada = errors.New("la votación está cerrada")
	ErrOpcionInvalida  = errors.New("opción inválida")
)

type Aviso struct {
	ID        string `json:"id"`
	SeccionID string `json:"seccion_id"`
	AutorID   string `json:"autor_id"`
	Tipo      string `json:"tipo"` // aviso | votacion
	Titulo    string `json:"titulo"`
	Cuerpo    string `json:"cuerpo"`
	CreatedAt string `json:"created_at"`
	// Votacion está presente solo cuando tipo = "votacion".
	Votacion *VotacionResumen `json:"votacion,omitempty"`
}

// VotacionResumen se embebe en el aviso para que la UI lo pinte sin llamadas extra.
type VotacionResumen struct {
	ID       string   `json:"id"`
	Pregunta string   `json:"pregunta"`
	Opciones []string `json:"opciones"`
	Multiple bool     `json:"multiple"`
	Cerrada  bool     `json:"cerrada"`
}

type Votacion struct {
	ID       string   `json:"id"`
	AvisoID  string   `json:"aviso_id"`
	Pregunta string   `json:"pregunta"`
	Opciones []string `json:"opciones"`
	Multiple bool     `json:"multiple"`
	Cerrada  bool     `json:"cerrada"`
	Creada   string   `json:"creada"`
}

type Comentario struct {
	ID        string `json:"id"`
	AvisoID   string `json:"aviso_id"`
	AutorID   string `json:"autor_id"`
	Cuerpo    string `json:"cuerpo"`
	CreatedAt string `json:"created_at"`
}

type Resultado struct {
	VotacionID    string `json:"votacion_id"`
	Pregunta      string `json:"pregunta"`
	TotalVotos    int    `json:"total_votos"`
	Participacion int    `json:"participacion"`
	Conteo        []struct {
		Opcion string `json:"opcion"`
		Votos  int    `json:"votos"`
	} `json:"conteo"`
}

type Pagina struct {
	Datos  []any `json:"datos"`
	Total  int   `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

func normalizarPaginacion(limit, offset int) (int, int) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func traducirErrorPG(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23503":
			return ErrNoEncontrado
		case "23514":
			return fmt.Errorf("%w: %s", ErrDatosInvalidos, pg.ConstraintName)
		}
	}
	return err
}

type Servicio struct {
	Pool *pgxpool.Pool
}

// --- permisos ---

// EsMiembro: inscrito activo, profesor, vocero de clase de la sección o mantenedor.
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

// PuedePublicar: profesor de la sección, vocero de clase con alcance a esa
// sección, o mantenedor.
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

// --- avisos ---

func (s *Servicio) PublicarAviso(ctx context.Context, seccionID, autorID, titulo, cuerpo string) (*Aviso, error) {
	if strings.TrimSpace(titulo) == "" {
		return nil, fmt.Errorf("%w: titulo es obligatorio", ErrDatosInvalidos)
	}
	var a Aviso
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO avisos (seccion_id, autor_id, tipo, titulo, cuerpo)
		 VALUES ($1::uuid, $2::uuid, 'aviso', $3, $4)
		 RETURNING id::text, seccion_id::text, autor_id::text, tipo, titulo, cuerpo, created_at::text`,
		seccionID, autorID, titulo, cuerpo).Scan(
		&a.ID, &a.SeccionID, &a.AutorID, &a.Tipo, &a.Titulo, &a.Cuerpo, &a.CreatedAt)
	if err != nil {
		return nil, traducirErrorPG(err)
	}
	return &a, nil
}

// ListarAvisos trae aviso + votación embebida (LEFT JOIN: un solo query).
func (s *Servicio) ListarAvisos(ctx context.Context, seccionID string, limit, offset int) (*Pagina, error) {
	limit, offset = normalizarPaginacion(limit, offset)
	p := &Pagina{Limit: limit, Offset: offset}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM avisos WHERE seccion_id=$1::uuid`, seccionID).Scan(&p.Total); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT a.id::text, a.seccion_id::text, a.autor_id::text, a.tipo, a.titulo, a.cuerpo, a.created_at::text,
		        v.id::text, v.pregunta, v.opciones::text, v.multiple, v.cerrada
		 FROM avisos a
		 LEFT JOIN votaciones v ON v.aviso_id = a.id
		 WHERE a.seccion_id=$1::uuid
		 ORDER BY a.created_at DESC LIMIT $2 OFFSET $3`, seccionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a Aviso
		var vID, vPregunta, vOpciones *string
		var vMultiple, vCerrada *bool
		if err := rows.Scan(&a.ID, &a.SeccionID, &a.AutorID, &a.Tipo, &a.Titulo, &a.Cuerpo, &a.CreatedAt,
			&vID, &vPregunta, &vOpciones, &vMultiple, &vCerrada); err != nil {
			return nil, err
		}
		if vID != nil {
			v := &VotacionResumen{ID: *vID, Pregunta: *vPregunta, Multiple: *vMultiple, Cerrada: *vCerrada}
			if vOpciones != nil {
				_ = json.Unmarshal([]byte(*vOpciones), &v.Opciones)
			}
			a.Votacion = v
		}
		p.Datos = append(p.Datos, a)
	}
	return p, rows.Err()
}

// --- votaciones ---

// CrearVotacion crea el aviso (tipo votacion) + la votación en una transacción.
func (s *Servicio) CrearVotacion(ctx context.Context, seccionID, autorID, titulo, pregunta string, opciones []string, multiple bool) (*Votacion, error) {
	if strings.TrimSpace(pregunta) == "" || len(opciones) < 2 {
		return nil, fmt.Errorf("%w: pregunta y al menos 2 opciones son obligatorias", ErrDatosInvalidos)
	}
	for _, op := range opciones {
		if strings.TrimSpace(op) == "" {
			return nil, fmt.Errorf("%w: opciones vacías", ErrDatosInvalidos)
		}
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var v Votacion
	err = tx.QueryRow(ctx,
		`INSERT INTO avisos (seccion_id, autor_id, tipo, titulo, cuerpo)
		 VALUES ($1::uuid, $2::uuid, 'votacion', $3, $4)
		 RETURNING id::text, created_at::text`,
		seccionID, autorID, titulo, pregunta).Scan(&v.AvisoID, &v.Creada)
	if err != nil {
		return nil, traducirErrorPG(err)
	}

	opcionesJSON, _ := json.Marshal(opciones)
	err = tx.QueryRow(ctx,
		`INSERT INTO votaciones (aviso_id, pregunta, opciones, multiple)
		 VALUES ($1::uuid, $2, $3, $4)
		 RETURNING id::text, pregunta, opciones::text, multiple, cerrada`,
		v.AvisoID, pregunta, opcionesJSON, multiple).Scan(
		&v.ID, &v.Pregunta, &opcionesJSON, &v.Multiple, &v.Cerrada)
	if err != nil {
		return nil, traducirErrorPG(err)
	}
	if err := json.Unmarshal(opcionesJSON, &v.Opciones); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Servicio) ObtenerVotacion(ctx context.Context, id string) (*Votacion, error) {
	var v Votacion
	var opcionesJSON []byte
	err := s.Pool.QueryRow(ctx,
		`SELECT id::text, aviso_id::text, pregunta, opciones::text, multiple, cerrada, creada::text
		 FROM votaciones WHERE id = $1::uuid`, id).Scan(
		&v.ID, &v.AvisoID, &v.Pregunta, &opcionesJSON, &v.Multiple, &v.Cerrada, &v.Creada)
	if err != nil {
		return nil, ErrNoEncontrado
	}
	if err := json.Unmarshal(opcionesJSON, &v.Opciones); err != nil {
		return nil, err
	}
	return &v, nil
}

// Votar registra participación y votos DESVINCULADOS en transacción:
// si ya votó, el INSERT en votantes falla (PK) y nada se guarda.
func (s *Servicio) Votar(ctx context.Context, votacionID, estudianteID string, opciones []int) error {
	v, err := s.ObtenerVotacion(ctx, votacionID)
	if err != nil {
		return err
	}
	if v.Cerrada {
		return ErrVotacionCerrada
	}
	if len(opciones) == 0 || (!v.Multiple && len(opciones) > 1) {
		return ErrOpcionInvalida
	}
	for _, idx := range opciones {
		if idx < 0 || idx >= len(v.Opciones) {
			return ErrOpcionInvalida
		}
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`INSERT INTO votantes (votacion_id, estudiante_id) VALUES ($1::uuid, $2::uuid)`,
		votacionID, estudianteID)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return ErrYaVoto
		}
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrYaVoto
	}
	for _, idx := range opciones {
		if _, err := tx.Exec(ctx,
			`INSERT INTO votos (votacion_id, opcion_idx) VALUES ($1::uuid, $2)`,
			votacionID, idx); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Resultados agrega los votos (conteo por opción) y la participación.
// Por diseño NO se puede saber quién votó qué.
func (s *Servicio) Resultados(ctx context.Context, votacionID string) (*Resultado, error) {
	v, err := s.ObtenerVotacion(ctx, votacionID)
	if err != nil {
		return nil, err
	}
	res := &Resultado{VotacionID: v.ID, Pregunta: v.Pregunta}
	for _, op := range v.Opciones {
		res.Conteo = append(res.Conteo, struct {
			Opcion string `json:"opcion"`
			Votos  int    `json:"votos"`
		}{Opcion: op})
	}
	if err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM votos WHERE votacion_id=$1::uuid`, votacionID).Scan(&res.TotalVotos); err != nil {
		return nil, err
	}
	if err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM votantes WHERE votacion_id=$1::uuid`, votacionID).Scan(&res.Participacion); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT opcion_idx, count(*) FROM votos WHERE votacion_id=$1::uuid GROUP BY opcion_idx`, votacionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var idx, n int
		if err := rows.Scan(&idx, &n); err != nil {
			return nil, err
		}
		if idx >= 0 && idx < len(res.Conteo) {
			res.Conteo[idx].Votos = n
		}
	}
	return res, rows.Err()
}

func (s *Servicio) CerrarVotacion(ctx context.Context, votacionID string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE votaciones SET cerrada=true WHERE id=$1::uuid`, votacionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNoEncontrado
	}
	return nil
}

// --- comentarios ---

func (s *Servicio) Comentar(ctx context.Context, avisoID, autorID, cuerpo string) (*Comentario, error) {
	if strings.TrimSpace(cuerpo) == "" {
		return nil, fmt.Errorf("%w: cuerpo vacío", ErrDatosInvalidos)
	}
	var c Comentario
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO comentarios (aviso_id, autor_id, cuerpo)
		 VALUES ($1::uuid, $2::uuid, $3)
		 RETURNING id::text, aviso_id::text, autor_id::text, cuerpo, created_at::text`,
		avisoID, autorID, cuerpo).Scan(&c.ID, &c.AvisoID, &c.AutorID, &c.Cuerpo, &c.CreatedAt)
	if err != nil {
		return nil, traducirErrorPG(err)
	}
	return &c, nil
}

func (s *Servicio) ListarComentarios(ctx context.Context, avisoID string, limit, offset int) (*Pagina, error) {
	limit, offset = normalizarPaginacion(limit, offset)
	p := &Pagina{Limit: limit, Offset: offset}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM comentarios WHERE aviso_id=$1::uuid`, avisoID).Scan(&p.Total); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id::text, aviso_id::text, autor_id::text, cuerpo, created_at::text
		 FROM comentarios WHERE aviso_id=$1::uuid
		 ORDER BY created_at LIMIT $2 OFFSET $3`, avisoID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Comentario
		if err := rows.Scan(&c.ID, &c.AvisoID, &c.AutorID, &c.Cuerpo, &c.CreatedAt); err != nil {
			return nil, err
		}
		p.Datos = append(p.Datos, c)
	}
	return p, rows.Err()
}

// SeccionDeAviso resuelve la sección de un aviso (para chequear membresía).
func (s *Servicio) SeccionDeAviso(ctx context.Context, avisoID string) (string, error) {
	var sid string
	err := s.Pool.QueryRow(ctx, `SELECT seccion_id::text FROM avisos WHERE id=$1::uuid`, avisoID).Scan(&sid)
	if err != nil {
		return "", ErrNoEncontrado
	}
	return sid, nil
}

// MarcarImportante activa/desactiva el flag importante del aviso. Al activarlo,
// notifica a los miembros de la sección (aparece en su inicio).
func (s *Servicio) MarcarImportante(ctx context.Context, avisoID, actorID string, valor bool) error {
	var seccionID, titulo string
	if err := s.Pool.QueryRow(ctx,
		`UPDATE avisos SET importante = $2 WHERE id = $1::uuid RETURNING seccion_id::text, titulo`,
		avisoID, valor).Scan(&seccionID, &titulo); err != nil {
		return ErrNoEncontrado
	}
	if valor {
		var nombre string
		_ = s.Pool.QueryRow(ctx, `SELECT nombre FROM users WHERE id = $1::uuid`, actorID).Scan(&nombre)
		_ = notifs.CrearParaMiembrosSeccion(ctx, s.Pool, seccionID, actorID,
			notifs.TipoAvisoImportante, avisoID, "Importante: "+titulo)
	}
	return nil
}

// ListarImportantesDe devuelve los avisos importantes de las secciones del
// usuario (para el inicio).
func (s *Servicio) ListarImportantesDe(ctx context.Context, userID string, limit int) ([]Aviso, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT a.id::text, a.seccion_id::text, a.autor_id::text, a.tipo, a.titulo, a.cuerpo, a.created_at::text
		 FROM avisos a
		 WHERE a.importante AND (
		    EXISTS (SELECT 1 FROM inscripciones i WHERE i.seccion_id = a.seccion_id
		            AND i.estudiante_id = $1::uuid AND i.estado = 'activa')
		    OR EXISTS (SELECT 1 FROM secciones sec WHERE sec.id = a.seccion_id AND sec.profesor_id = $1::uuid)
		 )
		 ORDER BY a.created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Aviso{}
	for rows.Next() {
		var a Aviso
		if err := rows.Scan(&a.ID, &a.SeccionID, &a.AutorID, &a.Tipo, &a.Titulo, &a.Cuerpo, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
