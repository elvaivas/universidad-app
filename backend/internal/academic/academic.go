// Package academic: núcleo académico — materias, secciones e inscripciones.
// Reglas de negocio: cupo atómico (nunca sobre-inscripción), inscripción
// única por estudiante/sección, baja que devuelve cupo. Los permisos finos
// (quién crea, quién ve la lista) los impone la capa HTTP con rbac.
package academic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoEncontrado   = errors.New("no encontrado")
	ErrDatosInvalidos = errors.New("datos inválidos")
	ErrCupoLleno      = errors.New("cupo de la sección completo")
	ErrYaInscrito     = errors.New("el estudiante ya está inscrito en esta sección")
	ErrNoInscrito     = errors.New("el estudiante no tiene inscripción activa en esta sección")
)

type Materia struct {
	ID        string `json:"id"`
	CarreraID string `json:"carrera_id"`
	Codigo    string `json:"codigo"`
	Nombre    string `json:"nombre"`
	Activo    bool   `json:"activo"`
}

type Seccion struct {
	ID         string `json:"id"`
	MateriaID  string `json:"materia_id"`
	PeriodoID  string `json:"periodo_id"`
	ProfesorID string `json:"profesor_id"`
	Codigo     string `json:"codigo"`
	Cupo       int    `json:"cupo"`
	Inscritos  int    `json:"inscritos"`
	Activo     bool   `json:"activo"`
}

type Inscripcion struct {
	ID           string `json:"id"`
	SeccionID    string `json:"seccion_id"`
	EstudianteID string `json:"estudiante_id"`
	Estado       string `json:"estado"`
	Fecha        string `json:"fecha"`
}

type Inscrito struct {
	EstudianteID string `json:"estudiante_id"`
	Email        string `json:"email"`
	Nombre       string `json:"nombre"`
	Fecha        string `json:"fecha"`
}

// Pagina iguala la forma del catálogo (S9).
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
		case "23503": // FK inexistente
			return ErrNoEncontrado
		case "23505": // UNIQUE duplicado
			if strings.Contains(pg.ConstraintName, "inscripciones") {
				return ErrYaInscrito
			}
			return fmt.Errorf("%w: código duplicado en el mismo nivel", ErrDatosInvalidos)
		case "23514": // CHECK
			return fmt.Errorf("%w: %s", ErrDatosInvalidos, pg.ConstraintName)
		}
	}
	return err
}

type Servicio struct {
	Pool *pgxpool.Pool
}

func (s *Servicio) validar(codigo, nombre string) error {
	if strings.TrimSpace(codigo) == "" || strings.TrimSpace(nombre) == "" {
		return fmt.Errorf("%w: codigo y nombre son obligatorios", ErrDatosInvalidos)
	}
	return nil
}

// --- materias ---

func (s *Servicio) CrearMateria(ctx context.Context, m *Materia) error {
	if err := s.validar(m.Codigo, m.Nombre); err != nil {
		return err
	}
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO materias (carrera_id, codigo, nombre) VALUES ($1::uuid, $2, $3) RETURNING id::text`,
		m.CarreraID, m.Codigo, m.Nombre).Scan(&m.ID)
	if err != nil {
		return traducirErrorPG(err)
	}
	return nil
}

func (s *Servicio) ListarMaterias(ctx context.Context, carreraID string, limit, offset int) (*Pagina, error) {
	limit, offset = normalizarPaginacion(limit, offset)
	p := &Pagina{Limit: limit, Offset: offset}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM materias WHERE carrera_id=$1::uuid`, carreraID).Scan(&p.Total); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id::text, carrera_id::text, codigo, nombre, activo FROM materias
		 WHERE carrera_id=$1::uuid ORDER BY codigo LIMIT $2 OFFSET $3`, carreraID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Materia
		if err := rows.Scan(&m.ID, &m.CarreraID, &m.Codigo, &m.Nombre, &m.Activo); err != nil {
			return nil, err
		}
		p.Datos = append(p.Datos, m)
	}
	return p, rows.Err()
}

// --- secciones ---

func (s *Servicio) CrearSeccion(ctx context.Context, sec *Seccion) error {
	if err := s.validar(sec.Codigo, "seccion"); err != nil {
		return err
	}
	if sec.PeriodoID == "" {
		return fmt.Errorf("%w: periodo_id es obligatorio", ErrDatosInvalidos)
	}
	if sec.Cupo <= 0 {
		sec.Cupo = 30
	}
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO secciones (materia_id, periodo_id, profesor_id, codigo, cupo)
		 VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5) RETURNING id::text`,
		sec.MateriaID, sec.PeriodoID, sec.ProfesorID, sec.Codigo, sec.Cupo).Scan(&sec.ID)
	if err != nil {
		return traducirErrorPG(err)
	}
	sec.Inscritos = 0
	sec.Activo = true
	return nil
}

func (s *Servicio) ListarSecciones(ctx context.Context, materiaID string, limit, offset int) (*Pagina, error) {
	limit, offset = normalizarPaginacion(limit, offset)
	p := &Pagina{Limit: limit, Offset: offset}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM secciones WHERE materia_id=$1::uuid`, materiaID).Scan(&p.Total); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id::text, materia_id::text, periodo_id::text, profesor_id::text, codigo, cupo, inscritos, activo
		 FROM secciones WHERE materia_id=$1::uuid ORDER BY codigo LIMIT $2 OFFSET $3`, materiaID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sec Seccion
		if err := rows.Scan(&sec.ID, &sec.MateriaID, &sec.PeriodoID, &sec.ProfesorID,
			&sec.Codigo, &sec.Cupo, &sec.Inscritos, &sec.Activo); err != nil {
			return nil, err
		}
		p.Datos = append(p.Datos, sec)
	}
	return p, rows.Err()
}

func (s *Servicio) ObtenerSeccion(ctx context.Context, id string) (*Seccion, error) {
	var sec Seccion
	err := s.Pool.QueryRow(ctx,
		`SELECT id::text, materia_id::text, periodo_id::text, profesor_id::text, codigo, cupo, inscritos, activo
		 FROM secciones WHERE id = $1::uuid`, id).Scan(
		&sec.ID, &sec.MateriaID, &sec.PeriodoID, &sec.ProfesorID,
		&sec.Codigo, &sec.Cupo, &sec.Inscritos, &sec.Activo)
	if err != nil {
		return nil, ErrNoEncontrado
	}
	return &sec, nil
}

// --- inscripciones ---

// Inscribir matricula a un estudiante con cupo atómico: el UPDATE condicional
// garantiza inscritos <= cupo aunque haya carreras concurrentes.
func (s *Servicio) Inscribir(ctx context.Context, seccionID, estudianteID string) (*Inscripcion, error) {
	sec, err := s.ObtenerSeccion(ctx, seccionID)
	if err != nil {
		return nil, err
	}
	if !sec.Activo {
		return nil, fmt.Errorf("%w: la sección está desactivada", ErrDatosInvalidos)
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid, $2::uuid)`,
		seccionID, estudianteID)
	if err != nil {
		return nil, traducirErrorPG(err) // UNIQUE → ErrYaInscrito
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrYaInscrito
	}
	tag, err = tx.Exec(ctx,
		`UPDATE secciones SET inscritos = inscritos + 1
		 WHERE id = $1::uuid AND inscritos < cupo AND activo`, seccionID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrCupoLleno // rollback libera la inscripción
	}

	var ins Inscripcion
	err = tx.QueryRow(ctx,
		`SELECT id::text, seccion_id::text, estudiante_id::text, estado, fecha::text
		 FROM inscripciones WHERE seccion_id=$1::uuid AND estudiante_id=$2::uuid`,
		seccionID, estudianteID).Scan(&ins.ID, &ins.SeccionID, &ins.EstudianteID, &ins.Estado, &ins.Fecha)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ins, nil
}

// Bajar revoca la inscripción activa y devuelve el cupo.
func (s *Servicio) Bajar(ctx context.Context, seccionID, estudianteID string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE inscripciones SET estado='retirada'
		 WHERE seccion_id=$1::uuid AND estudiante_id=$2::uuid AND estado='activa'`,
		seccionID, estudianteID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNoInscrito
	}
	if _, err := tx.Exec(ctx,
		`UPDATE secciones SET inscritos = GREATEST(inscritos - 1, 0) WHERE id=$1::uuid`, seccionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListarInscritos devuelve las inscripciones activas con datos del estudiante.
func (s *Servicio) ListarInscritos(ctx context.Context, seccionID string, limit, offset int) (*Pagina, error) {
	limit, offset = normalizarPaginacion(limit, offset)
	p := &Pagina{Limit: limit, Offset: offset}
	if err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM inscripciones WHERE seccion_id=$1::uuid AND estado='activa'`,
		seccionID).Scan(&p.Total); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT u.id::text, lower(u.email), u.nombre, i.fecha::text
		 FROM inscripciones i JOIN users u ON u.id = i.estudiante_id
		 WHERE i.seccion_id=$1::uuid AND i.estado='activa'
		 ORDER BY u.nombre LIMIT $2 OFFSET $3`, seccionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var in Inscrito
		if err := rows.Scan(&in.EstudianteID, &in.Email, &in.Nombre, &in.Fecha); err != nil {
			return nil, err
		}
		p.Datos = append(p.Datos, in)
	}
	return p, rows.Err()
}

// SeccionesDeUsuario alimenta el claim seccion_ids del JWT (Flujo 1):
// secciones donde es profesor + secciones donde está inscrito activo.
func (s *Servicio) SeccionesDeUsuario(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id::text FROM secciones WHERE profesor_id=$1::uuid AND activo
		 UNION
		 SELECT i.seccion_id::text FROM inscripciones i
		 WHERE i.estudiante_id=$1::uuid AND i.estado='activa'`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, rows.Err()
}

// --- resumen para la UI ---

// SeccionResumen es lo que la app muestra en "mis secciones".
type SeccionResumen struct {
	ID            string `json:"id"`
	Codigo        string `json:"codigo"`
	MateriaNombre string `json:"materia_nombre"`
	PeriodoNombre string `json:"periodo_nombre"`
	EsProfesor    bool   `json:"es_profesor"`
}

// ListarSeccionesDeUsuario resume las secciones donde la persona es profesor
// o está inscrita activa: alimenta la home de la PWA.
func (s *Servicio) ListarSeccionesDeUsuario(ctx context.Context, userID string) ([]SeccionResumen, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT sec.id::text, sec.codigo, m.nombre, p.nombre, true
		 FROM secciones sec JOIN materias m ON m.id = sec.materia_id
		 JOIN periodos p ON p.id = sec.periodo_id
		 WHERE sec.profesor_id = $1::uuid AND sec.activo
		 UNION
		 SELECT sec.id::text, sec.codigo, m.nombre, p.nombre, false
		 FROM inscripciones i
		 JOIN secciones sec ON sec.id = i.seccion_id
		 JOIN materias m ON m.id = sec.materia_id
		 JOIN periodos p ON p.id = sec.periodo_id
		 WHERE i.estudiante_id = $1::uuid AND i.estado = 'activa' AND sec.activo
		 ORDER BY 3, 2`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SeccionResumen{}
	for rows.Next() {
		var r SeccionResumen
		if err := rows.Scan(&r.ID, &r.Codigo, &r.MateriaNombre, &r.PeriodoNombre, &r.EsProfesor); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
