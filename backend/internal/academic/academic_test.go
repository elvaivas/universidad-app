package academic

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/testdb"
)

// setup crea la cadena completa: universidad → carrera → periodo, un profesor
// y dos estudiantes. Devuelve ids listos para usar.
type entorno struct {
	ctx          context.Context
	s            *Servicio
	carreraID    string
	periodoID    string
	profesorID   string
	estudianteID string
	otroEstID    string
}

func setup(t *testing.T) *entorno {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.Nueva(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()
	e := &entorno{ctx: ctx, s: &Servicio{Pool: pool}}

	var uid string
	err = pool.QueryRow(ctx,
		`INSERT INTO universidades (nombre) VALUES ('Univ Test') RETURNING id::text`).Scan(&uid)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx,
		`INSERT INTO carreras (universidad_id, nombre) VALUES ($1::uuid, 'Ing') RETURNING id::text`, uid).Scan(&e.carreraID)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx,
		`INSERT INTO periodos (carrera_id, nombre, fecha_inicio, fecha_fin)
		 VALUES ($1::uuid, '2026-1', '2026-01-12', '2026-05-30') RETURNING id::text`, e.carreraID).Scan(&e.periodoID)
	if err != nil {
		t.Fatal(err)
	}
	e.profesorID = insertarUsuario(t, ctx, pool, "prof@uni.test", "profesor")
	e.estudianteID = insertarUsuario(t, ctx, pool, "est@uni.test", "estudiante")
	e.otroEstID = insertarUsuario(t, ctx, pool, "est2@uni.test", "estudiante")
	return e
}

func insertarUsuario(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email, rol string) string {
	t.Helper()
	hash, _ := auth.HashPassword("Clave#2026X")
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, nombre, rol_base) VALUES ($1, $2, $3, $4) RETURNING id::text`,
		email, hash, email, rol).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *entorno) materia(t *testing.T) *Materia {
	t.Helper()
	m := &Materia{CarreraID: e.carreraID, Codigo: "PS-101", Nombre: "Programación I"}
	if err := e.s.CrearMateria(e.ctx, m); err != nil {
		t.Fatalf("CrearMateria: %v", err)
	}
	return m
}

func TestCadenaMateriaSeccionInscripcion(t *testing.T) {
	e := setup(t)
	m := e.materia(t)

	sec := &Seccion{MateriaID: m.ID, PeriodoID: e.periodoID, ProfesorID: e.profesorID, Codigo: "1", Cupo: 2}
	if err := e.s.CrearSeccion(e.ctx, sec); err != nil {
		t.Fatalf("CrearSeccion: %v", err)
	}
	if sec.Cupo != 2 || sec.Inscritos != 0 {
		t.Errorf("sección mal inicializada: %+v", sec)
	}

	// inscripción normal
	ins, err := e.s.Inscribir(e.ctx, sec.ID, e.estudianteID)
	if err != nil {
		t.Fatalf("Inscribir: %v", err)
	}
	if ins.Estado != "activa" {
		t.Errorf("estado: %s", ins.Estado)
	}

	// duplicada → rechazo
	if _, err := e.s.Inscribir(e.ctx, sec.ID, e.estudianteID); !errors.Is(err, ErrYaInscrito) {
		t.Errorf("inscripción duplicada aceptada: %v", err)
	}

	// segundo estudiante llena el cupo (cupo=2)
	if _, err := e.s.Inscribir(e.ctx, sec.ID, e.otroEstID); err != nil {
		t.Fatalf("Inscribir 2: %v", err)
	}

	// tercero → cupo lleno
	tercero := insertarUsuario(t, e.ctx, e.s.Pool, "est3@uni.test", "estudiante")
	if _, err := e.s.Inscribir(e.ctx, sec.ID, tercero); !errors.Is(err, ErrCupoLleno) {
		t.Errorf("sobrecupo aceptado: %v", err)
	}

	// la sección refleja 2 inscritos
	got, _ := e.s.ObtenerSeccion(e.ctx, sec.ID)
	if got.Inscritos != 2 {
		t.Errorf("inscritos=%d, esperaba 2", got.Inscritos)
	}

	// baja libera cupo
	if err := e.s.Bajar(e.ctx, sec.ID, e.estudianteID); err != nil {
		t.Fatalf("Bajar: %v", err)
	}
	if err := e.s.Bajar(e.ctx, sec.ID, e.estudianteID); !errors.Is(err, ErrNoInscrito) {
		t.Errorf("doble baja aceptada: %v", err)
	}
	got, _ = e.s.ObtenerSeccion(e.ctx, sec.ID)
	if got.Inscritos != 1 {
		t.Errorf("cupo no liberado: %d", got.Inscritos)
	}

	// y el tercero ahora sí entra
	if _, err := e.s.Inscribir(e.ctx, sec.ID, tercero); err != nil {
		t.Errorf("tras baja debería haber cupo: %v", err)
	}
}

func TestListarInscritosYSeccionesDeUsuario(t *testing.T) {
	e := setup(t)
	m := e.materia(t)
	sec := &Seccion{MateriaID: m.ID, PeriodoID: e.periodoID, ProfesorID: e.profesorID, Codigo: "1", Cupo: 10}
	if err := e.s.CrearSeccion(e.ctx, sec); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Inscribir(e.ctx, sec.ID, e.estudianteID); err != nil {
		t.Fatal(err)
	}

	p, err := e.s.ListarInscritos(e.ctx, sec.ID, 20, 0)
	if err != nil || p.Total != 1 {
		t.Fatalf("ListarInscritos: total=%d err=%v", p.Total, err)
	}
	in := p.Datos[0].(Inscrito)
	if in.Email != "est@uni.test" {
		t.Errorf("inscrito equivocado: %s", in.Email)
	}

	// claim seccion_ids: estudiante ve la sección, profesor la ve, tercero no
	ids, _ := e.s.SeccionesDeUsuario(e.ctx, e.estudianteID)
	if len(ids) != 1 || ids[0] != sec.ID {
		t.Errorf("estudiante seccion_ids=%v", ids)
	}
	ids, _ = e.s.SeccionesDeUsuario(e.ctx, e.profesorID)
	if len(ids) != 1 {
		t.Errorf("profesor seccion_ids=%v", ids)
	}
	ids, _ = e.s.SeccionesDeUsuario(e.ctx, e.otroEstID)
	if len(ids) != 0 {
		t.Errorf("tercero seccion_ids=%v", ids)
	}
}

func TestSeccionDuplicadaYMateriaInexistente(t *testing.T) {
	e := setup(t)
	m := e.materia(t)
	sec := &Seccion{MateriaID: m.ID, PeriodoID: e.periodoID, ProfesorID: e.profesorID, Codigo: "1"}
	if err := e.s.CrearSeccion(e.ctx, sec); err != nil {
		t.Fatal(err)
	}
	dup := &Seccion{MateriaID: m.ID, PeriodoID: e.periodoID, ProfesorID: e.profesorID, Codigo: "1"}
	if err := e.s.CrearSeccion(e.ctx, dup); !errors.Is(err, ErrDatosInvalidos) {
		t.Errorf("sección duplicada aceptada: %v", err)
	}

	malo := &Materia{CarreraID: "00000000-0000-0000-0000-000000000000", Codigo: "X", Nombre: "Y"}
	if err := e.s.CrearMateria(e.ctx, malo); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("materia huérfana aceptada: %v", err)
	}
}

func TestListarSeccionesDeUsuarioResumen(t *testing.T) {
	e := setup(t)
	m := e.materia(t)
	sec := &Seccion{MateriaID: m.ID, PeriodoID: e.periodoID, ProfesorID: e.profesorID, Codigo: "3", Cupo: 5}
	if err := e.s.CrearSeccion(e.ctx, sec); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Inscribir(e.ctx, sec.ID, e.estudianteID); err != nil {
		t.Fatal(err)
	}

	res, err := e.s.ListarSeccionesDeUsuario(e.ctx, e.profesorID)
	if err != nil || len(res) != 1 || !res[0].EsProfesor {
		t.Fatalf("profesor: %v %v", res, err)
	}
	if res[0].MateriaNombre != "Programación I" || res[0].PeriodoNombre != "2026-1" {
		t.Errorf("resumen: %+v", res[0])
	}

	res, err = e.s.ListarSeccionesDeUsuario(e.ctx, e.estudianteID)
	if err != nil || len(res) != 1 || res[0].EsProfesor {
		t.Errorf("estudiante: %v %v", res, err)
	}
	res, _ = e.s.ListarSeccionesDeUsuario(e.ctx, e.otroEstID)
	if len(res) != 0 {
		t.Errorf("afuera debería tener 0: %v", res)
	}
}
