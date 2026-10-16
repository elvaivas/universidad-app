package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/testdb"
)

func setup(t *testing.T) (context.Context, *Servicio) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.Nueva(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return context.Background(), &Servicio{Pool: pool}
}

func crearUniversidad(t *testing.T, s *Servicio, ctx context.Context, nombre string) *Universidad {
	t.Helper()
	u := &Universidad{Nombre: nombre, ColorPrimario: "#005"}
	if err := s.CrearUniversidad(ctx, u); err != nil {
		t.Fatalf("CrearUniversidad: %v", err)
	}
	if u.ID == "" {
		t.Fatal("ID no asignado")
	}
	return u
}

func TestUniversidadCRUDYPaginacion(t *testing.T) {
	ctx, s := setup(t)
	for _, n := range []string{"Alpha", "Beta", "Gamma"} {
		crearUniversidad(t, s, ctx, n)
	}

	p, err := s.ListarUniversidades(ctx, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 3 || len(p.Datos) != 2 {
		t.Errorf("página 1: total=%d datos=%d", p.Total, len(p.Datos))
	}
	p2, _ := s.ListarUniversidades(ctx, 2, 2)
	if len(p2.Datos) != 1 {
		t.Errorf("página 2: datos=%d", len(p2.Datos))
	}
	p3, _ := s.ListarUniversidades(ctx, 0, 0) // límite inválido → default 20
	if p3.Limit != 20 {
		t.Errorf("default limit: %d", p3.Limit)
	}

	u := p.Datos[0].(Universidad)
	got, err := s.ObtenerUniversidad(ctx, u.ID)
	if err != nil || got.Nombre != u.Nombre {
		t.Errorf("ObtenerUniversidad: %v %v", got, err)
	}

	got.Nombre = "Alpha Actualizada"
	if err := s.ActualizarUniversidad(ctx, got.ID, got); err != nil {
		t.Fatalf("ActualizarUniversidad: %v", err)
	}
	otra, _ := s.ObtenerUniversidad(ctx, got.ID)
	if otra.Nombre != "Alpha Actualizada" {
		t.Errorf("update no aplicado: %s", otra.Nombre)
	}

	if err := s.DesactivarUniversidad(ctx, got.ID); err != nil {
		t.Fatalf("DesactivarUniversidad: %v", err)
	}
	inactiva, _ := s.ObtenerUniversidad(ctx, got.ID)
	if inactiva.Activo {
		t.Error("baja lógica no aplicada")
	}

	if _, err := s.ObtenerUniversidad(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("esperaba ErrNoEncontrado, obtuvo %v", err)
	}
}

func TestUniversidadNombreVacio(t *testing.T) {
	ctx, s := setup(t)
	if err := s.CrearUniversidad(ctx, &Universidad{Nombre: "  "}); !errors.Is(err, ErrDatosInvalidos) {
		t.Errorf("esperaba ErrDatosInvalidos, obtuvo %v", err)
	}
}

func TestCadenaCarreraPeriodo(t *testing.T) {
	ctx, s := setup(t)
	u := crearUniversidad(t, s, ctx, "Univ Cadena")

	c := &Carrera{UniversidadID: u.ID, Nombre: "Ingeniería", TipoPeriodo: "semestre"}
	if err := s.CrearCarrera(ctx, c); err != nil {
		t.Fatalf("CrearCarrera: %v", err)
	}

	pe := &Periodo{CarreraID: c.ID, Nombre: "2026-1", FechaInicio: "2026-01-12", FechaFin: "2026-05-30"}
	if err := s.CrearPeriodo(ctx, pe); err != nil {
		t.Fatalf("CrearPeriodo: %v", err)
	}

	pc, err := s.ListarCarreras(ctx, u.ID, 10, 0)
	if err != nil || pc.Total != 1 {
		t.Errorf("ListarCarreras: total=%d err=%v", pc.Total, err)
	}
	pp, err := s.ListarPeriodos(ctx, c.ID, 10, 0)
	if err != nil || pp.Total != 1 {
		t.Errorf("ListarPeriodos: total=%d err=%v", pp.Total, err)
	}

	// fechas invertidas violan el CHECK → 400 de dominio
	malo := &Periodo{CarreraID: c.ID, Nombre: "2026-2", FechaInicio: "2026-06-01", FechaFin: "2026-01-01"}
	if err := s.CrearPeriodo(ctx, malo); !errors.Is(err, ErrDatosInvalidos) {
		t.Errorf("fechas invertidas aceptadas: %v", err)
	}
}

func TestCarreraUniversidadInexistente(t *testing.T) {
	ctx, s := setup(t)
	c := &Carrera{UniversidadID: "00000000-0000-0000-0000-000000000000", Nombre: "Huérfana"}
	if err := s.CrearCarrera(ctx, c); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("FK violada no traducida: %v", err)
	}
}

func TestAnuncioGeneralEditarYEliminar(t *testing.T) {
	ctx, s := setup(t)
	var autorID string
	hash, _ := auth.HashPassword("Clave#2026X")
	if err := s.Pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, nombre, rol_base)
		 VALUES ('mant@uni.test', $1, 'Mant', 'profesor') RETURNING id::text`, hash).Scan(&autorID); err != nil {
		t.Fatal(err)
	}

	a, err := s.CrearAnuncioGeneral(ctx, autorID, "Feriado", "No hay clases el viernes")
	if err != nil {
		t.Fatalf("CrearAnuncioGeneral: %v", err)
	}
	editado, err := s.ActualizarAnuncioGeneral(ctx, a.ID, "Feriado confirmado", "Se movió al lunes")
	if err != nil {
		t.Fatalf("ActualizarAnuncioGeneral: %v", err)
	}
	if editado.Titulo != "Feriado confirmado" || editado.Cuerpo != "Se movió al lunes" {
		t.Errorf("editado: %+v", editado)
	}
	if _, err := s.ActualizarAnuncioGeneral(ctx, a.ID, "  ", "x"); !errors.Is(err, ErrDatosInvalidos) {
		t.Errorf("título vacío: %v", err)
	}
	lista, _ := s.ListarAnunciosGenerales(ctx, 10)
	if len(lista) != 1 || lista[0].Titulo != "Feriado confirmado" {
		t.Errorf("lista: %+v", lista)
	}
	if err := s.EliminarAnuncioGeneral(ctx, a.ID); err != nil {
		t.Fatalf("EliminarAnuncioGeneral: %v", err)
	}
	if err := s.EliminarAnuncioGeneral(ctx, a.ID); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("doble eliminación: %v", err)
	}
	vacia, _ := s.ListarAnunciosGenerales(ctx, 10)
	if len(vacia) != 0 {
		t.Errorf("no se eliminó: %d", len(vacia))
	}
}
