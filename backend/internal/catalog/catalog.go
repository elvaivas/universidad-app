// Package catalog: CRUD del catálogo académico base (universidades, carreras,
// periodos). Lectura para cualquier usuario autenticado; escritura solo para
// mantenedores (lo impone la capa HTTP con rbac). Baja lógica vía activo=false:
// nunca se borran filas referenciadas por historia académica.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoEncontrado  = errors.New("no encontrado")
	ErrDatosInvalidos = errors.New("datos inválidos")
)

// Pagina fija la forma de todos los listados (S9: paginación obligatoria).
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

// --- tipos ---

type Universidad struct {
	ID            string `json:"id"`
	Nombre        string `json:"nombre"`
	LogoURL       string `json:"logo_url,omitempty"`
	ColorPrimario string `json:"color_primario,omitempty"`
	ColorAcento   string `json:"color_acento,omitempty"`
	NombreCorto   string `json:"nombre_corto,omitempty"`
	Activo        bool   `json:"activo"`
}

type Carrera struct {
	ID            string `json:"id"`
	UniversidadID string `json:"universidad_id"`
	Nombre        string `json:"nombre"`
	LogoURL       string `json:"logo_url,omitempty"`
	TipoPeriodo   string `json:"tipo_periodo"` // semestre | trimestre | anual
	Activo        bool   `json:"activo"`
}

type Periodo struct {
	ID          string `json:"id"`
	CarreraID   string `json:"carrera_id"`
	Nombre      string `json:"nombre"` // ej: '2026-1'
	FechaInicio string `json:"fecha_inicio"`
	FechaFin    string `json:"fecha_fin"`
	Estado      string `json:"estado"` // abierto | en_curso | cerrado
}

// --- servicio ---

type Servicio struct {
	Pool *pgxpool.Pool
}

func (s *Servicio) validarNombre(nombre string) error {
	if strings.TrimSpace(nombre) == "" {
		return fmt.Errorf("%w: nombre es obligatorio", ErrDatosInvalidos)
	}
	return nil
}

// traducirErrorPG mapea violaciones de PostgreSQL a errores de dominio
// por SQLSTATE (nunca por texto del mensaje, que es frágil).
func traducirErrorPG(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23503": // foreign_key_violation
			return ErrNoEncontrado
		case "23514": // check_violation
			return fmt.Errorf("%w: %s", ErrDatosInvalidos, pg.ConstraintName)
		}
	}
	return err
}

// --- universidades ---

func (s *Servicio) ListarUniversidades(ctx context.Context, limit, offset int) (*Pagina, error) {
	limit, offset = normalizarPaginacion(limit, offset)
	p := &Pagina{Limit: limit, Offset: offset}

	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM universidades`).Scan(&p.Total)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id::text, nombre, COALESCE(logo_url,''), COALESCE(color_primario,''),
		        COALESCE(color_acento,''), COALESCE(nombre_corto,''), activo
		 FROM universidades ORDER BY nombre LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u Universidad
		if err := rows.Scan(&u.ID, &u.Nombre, &u.LogoURL, &u.ColorPrimario,
			&u.ColorAcento, &u.NombreCorto, &u.Activo); err != nil {
			return nil, err
		}
		p.Datos = append(p.Datos, u)
	}
	return p, rows.Err()
}

func (s *Servicio) ObtenerUniversidad(ctx context.Context, id string) (*Universidad, error) {
	var u Universidad
	err := s.Pool.QueryRow(ctx,
		`SELECT id::text, nombre, COALESCE(logo_url,''), COALESCE(color_primario,''),
		        COALESCE(color_acento,''), COALESCE(nombre_corto,''), activo
		 FROM universidades WHERE id = $1::uuid`, id).Scan(
		&u.ID, &u.Nombre, &u.LogoURL, &u.ColorPrimario, &u.ColorAcento, &u.NombreCorto, &u.Activo)
	if err != nil {
		return nil, ErrNoEncontrado
	}
	return &u, nil
}

func (s *Servicio) CrearUniversidad(ctx context.Context, u *Universidad) error {
	if err := s.validarNombre(u.Nombre); err != nil {
		return err
	}
	return s.Pool.QueryRow(ctx,
		`INSERT INTO universidades (nombre, logo_url, color_primario, color_acento, nombre_corto)
		 VALUES ($1, NULLIF($2,''), NULLIF($3,''), NULLIF($4,''), NULLIF($5,''))
		 RETURNING id::text`, u.Nombre, u.LogoURL, u.ColorPrimario, u.ColorAcento, u.NombreCorto).Scan(&u.ID)
}

func (s *Servicio) ActualizarUniversidad(ctx context.Context, id string, u *Universidad) error {
	if err := s.validarNombre(u.Nombre); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx,
		`UPDATE universidades SET nombre=$2, logo_url=NULLIF($3,''), color_primario=NULLIF($4,''),
		 color_acento=NULLIF($5,''), nombre_corto=NULLIF($6,'')
		 WHERE id=$1::uuid`, id, u.Nombre, u.LogoURL, u.ColorPrimario, u.ColorAcento, u.NombreCorto)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoEncontrado
	}
	u.ID = id
	return nil
}

// DesactivarUniversidad baja lógica (activo=false). Las carreras asociadas
// permanecen para no romper historia académica.
func (s *Servicio) DesactivarUniversidad(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE universidades SET activo=false WHERE id=$1::uuid AND activo=true`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoEncontrado
	}
	return nil
}

// --- carreras ---

func (s *Servicio) ListarCarreras(ctx context.Context, universidadID string, limit, offset int) (*Pagina, error) {
	limit, offset = normalizarPaginacion(limit, offset)
	p := &Pagina{Limit: limit, Offset: offset}
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM carreras WHERE universidad_id=$1::uuid`, universidadID).Scan(&p.Total)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id::text, universidad_id::text, nombre, COALESCE(logo_url,''), tipo_periodo, activo
		 FROM carreras WHERE universidad_id=$1::uuid ORDER BY nombre LIMIT $2 OFFSET $3`,
		universidadID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Carrera
		if err := rows.Scan(&c.ID, &c.UniversidadID, &c.Nombre, &c.LogoURL, &c.TipoPeriodo, &c.Activo); err != nil {
			return nil, err
		}
		p.Datos = append(p.Datos, c)
	}
	return p, rows.Err()
}

func (s *Servicio) CrearCarrera(ctx context.Context, c *Carrera) error {
	if err := s.validarNombre(c.Nombre); err != nil {
		return err
	}
	if c.TipoPeriodo == "" {
		c.TipoPeriodo = "semestre"
	}
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO carreras (universidad_id, nombre, logo_url, tipo_periodo)
		 VALUES ($1::uuid, $2, NULLIF($3,''), $4) RETURNING id::text`,
		c.UniversidadID, c.Nombre, c.LogoURL, c.TipoPeriodo).Scan(&c.ID)
	if err != nil {
		return traducirErrorPG(err)
	}
	return nil
}

func (s *Servicio) ActualizarCarrera(ctx context.Context, id string, c *Carrera) error {
	if err := s.validarNombre(c.Nombre); err != nil {
		return err
	}
	if c.TipoPeriodo == "" {
		c.TipoPeriodo = "semestre"
	}
	tag, err := s.Pool.Exec(ctx,
		`UPDATE carreras SET nombre=$2, logo_url=NULLIF($3,''), tipo_periodo=$4 WHERE id=$1::uuid`,
		id, c.Nombre, c.LogoURL, c.TipoPeriodo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoEncontrado
	}
	c.ID = id
	return nil
}

func (s *Servicio) DesactivarCarrera(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE carreras SET activo=false WHERE id=$1::uuid AND activo=true`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoEncontrado
	}
	return nil
}

// --- periodos ---

func (s *Servicio) ListarPeriodos(ctx context.Context, carreraID string, limit, offset int) (*Pagina, error) {
	limit, offset = normalizarPaginacion(limit, offset)
	p := &Pagina{Limit: limit, Offset: offset}
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM periodos WHERE carrera_id=$1::uuid`, carreraID).Scan(&p.Total)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id::text, carrera_id::text, nombre, fecha_inicio::text, fecha_fin::text, estado
		 FROM periodos WHERE carrera_id=$1::uuid ORDER BY fecha_inicio DESC LIMIT $2 OFFSET $3`,
		carreraID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pe Periodo
		if err := rows.Scan(&pe.ID, &pe.CarreraID, &pe.Nombre, &pe.FechaInicio, &pe.FechaFin, &pe.Estado); err != nil {
			return nil, err
		}
		p.Datos = append(p.Datos, pe)
	}
	return p, rows.Err()
}

func (s *Servicio) CrearPeriodo(ctx context.Context, pe *Periodo) error {
	if err := s.validarNombre(pe.Nombre); err != nil {
		return err
	}
	if pe.FechaInicio == "" || pe.FechaFin == "" {
		return fmt.Errorf("%w: fechas obligatorias", ErrDatosInvalidos)
	}
	if pe.Estado == "" {
		pe.Estado = "abierto"
	}
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO periodos (carrera_id, nombre, fecha_inicio, fecha_fin, estado)
		 VALUES ($1::uuid, $2, $3::date, $4::date, $5) RETURNING id::text`,
		pe.CarreraID, pe.Nombre, pe.FechaInicio, pe.FechaFin, pe.Estado).Scan(&pe.ID)
	if err != nil {
		return traducirErrorPG(err)
	}
	return nil
}

func (s *Servicio) ActualizarPeriodo(ctx context.Context, id string, pe *Periodo) error {
	if err := s.validarNombre(pe.Nombre); err != nil {
		return err
	}
	if pe.Estado == "" {
		pe.Estado = "abierto"
	}
	tag, err := s.Pool.Exec(ctx,
		`UPDATE periodos SET nombre=$2, fecha_inicio=$3::date, fecha_fin=$4::date, estado=$5 WHERE id=$1::uuid`,
		id, pe.Nombre, pe.FechaInicio, pe.FechaFin, pe.Estado)
	if err != nil {
		return traducirErrorPG(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoEncontrado
	}
	pe.ID = id
	return nil
}

// --- anuncios generales de la universidad (solo mantenedor publica) ---

type AnuncioGeneral struct {
	ID        string `json:"id"`
	AutorID   string `json:"autor_id"`
	Titulo    string `json:"titulo"`
	Cuerpo    string `json:"cuerpo"`
	CreatedAt string `json:"created_at"`
}

func (s *Servicio) CrearAnuncioGeneral(ctx context.Context, autorID, titulo, cuerpo string) (*AnuncioGeneral, error) {
	if strings.TrimSpace(titulo) == "" {
		return nil, ErrDatosInvalidos
	}
	var a AnuncioGeneral
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO anuncios_generales (autor_id, titulo, cuerpo)
		 VALUES ($1::uuid, $2, $3)
		 RETURNING id::text, autor_id::text, titulo, cuerpo, created_at::text`,
		autorID, titulo, cuerpo).Scan(&a.ID, &a.AutorID, &a.Titulo, &a.Cuerpo, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Servicio) ListarAnunciosGenerales(ctx context.Context, limit int) ([]AnuncioGeneral, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id::text, autor_id::text, titulo, cuerpo, created_at::text
		 FROM anuncios_generales ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AnuncioGeneral{}
	for rows.Next() {
		var a AnuncioGeneral
		if err := rows.Scan(&a.ID, &a.AutorID, &a.Titulo, &a.Cuerpo, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ActualizarAnuncioGeneral edita título/cuerpo (el permiso lo pone la capa HTTP).
func (s *Servicio) ActualizarAnuncioGeneral(ctx context.Context, id, titulo, cuerpo string) (*AnuncioGeneral, error) {
	if strings.TrimSpace(titulo) == "" {
		return nil, ErrDatosInvalidos
	}
	var a AnuncioGeneral
	err := s.Pool.QueryRow(ctx,
		`UPDATE anuncios_generales SET titulo = $2, cuerpo = $3 WHERE id = $1::uuid
		 RETURNING id::text, autor_id::text, titulo, cuerpo, created_at::text`,
		id, titulo, cuerpo).Scan(&a.ID, &a.AutorID, &a.Titulo, &a.Cuerpo, &a.CreatedAt)
	if err != nil {
		return nil, ErrNoEncontrado
	}
	return &a, nil
}

// EliminarAnuncioGeneral borra el anuncio (existencia primero, como en todo el sistema).
func (s *Servicio) EliminarAnuncioGeneral(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM anuncios_generales WHERE id = $1::uuid`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNoEncontrado
	}
	return nil
}
