package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/testdb"
)

type entorno struct {
	ctx        context.Context
	s          *Servicio
	seccionID  string
	profesorID string
	estID      string
	afueraID   string
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

	var uid, cid, pid, mid string
	_ = pool.QueryRow(ctx, `INSERT INTO universidades (nombre) VALUES ('U') RETURNING id::text`).Scan(&uid)
	_ = pool.QueryRow(ctx, `INSERT INTO carreras (universidad_id, nombre) VALUES ($1::uuid,'Ing') RETURNING id::text`, uid).Scan(&cid)
	_ = pool.QueryRow(ctx, `INSERT INTO periodos (carrera_id, nombre, fecha_inicio, fecha_fin) VALUES ($1::uuid,'2026-1','2026-01-12','2026-05-30') RETURNING id::text`, cid).Scan(&pid)
	_ = pool.QueryRow(ctx, `INSERT INTO materias (carrera_id, codigo, nombre) VALUES ($1::uuid,'PS-101','Prog') RETURNING id::text`, cid).Scan(&mid)
	e.profesorID = insertarUsuario(t, ctx, pool, "prof@uni.test", "profesor")
	e.estID = insertarUsuario(t, ctx, pool, "est@uni.test", "estudiante")
	e.afueraID = insertarUsuario(t, ctx, pool, "afuera@uni.test", "estudiante")
	_ = pool.QueryRow(ctx,
		`INSERT INTO secciones (materia_id, periodo_id, profesor_id, codigo, cupo) VALUES ($1::uuid,$2::uuid,$3::uuid,'1',30) RETURNING id::text`,
		mid, pid, e.profesorID).Scan(&e.seccionID)
	_, _ = pool.Exec(ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`, e.seccionID, e.estID)
	return e
}

func insertarUsuario(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email, rol string) string {
	t.Helper()
	hash, _ := auth.HashPassword("Clave#2026X")
	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, nombre, rol_base) VALUES ($1,$2,$3,$4) RETURNING id::text`,
		email, hash, email, rol).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGuardarYListarCronologico(t *testing.T) {
	e := setup(t)

	m1, err := e.s.Guardar(e.ctx, e.seccionID, e.estID, "  Hola a todos  ")
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	if m1.Cuerpo != "Hola a todos" {
		t.Errorf("trim no aplicado: %q", m1.Cuerpo)
	}
	if _, err := e.s.Guardar(e.ctx, e.seccionID, e.estID, ""); !errors.Is(err, ErrCuerpoInvalido) {
		t.Errorf("cuerpo vacío aceptado: %v", err)
	}
	if _, err := e.s.Guardar(e.ctx, e.seccionID, e.estID, strings.Repeat("x", 2001)); !errors.Is(err, ErrCuerpoInvalido) {
		t.Errorf("cuerpo gigante aceptado: %v", err)
	}

	if _, err := e.s.Guardar(e.ctx, e.seccionID, e.profesorID, "Bienvenidos"); err != nil {
		t.Fatal(err)
	}

	datos, total, err := e.s.Listar(e.ctx, e.seccionID, 50, 0)
	if err != nil || total != 2 {
		t.Fatalf("Listar: %v %v", total, err)
	}
	if datos[0].Cuerpo != "Hola a todos" || datos[1].Cuerpo != "Bienvenidos" {
		t.Errorf("orden: %+v", datos)
	}
	if datos[0].AutorNom == "" {
		t.Error("autor sin nombre")
	}
}

func TestMembresiaChat(t *testing.T) {
	e := setup(t)
	for nombre, id := range map[string]string{
		"profesor": e.profesorID, "inscrito": e.estID, "afuera": e.afueraID,
	} {
		ok, err := e.s.EsMiembro(e.ctx, id, e.seccionID)
		if err != nil {
			t.Fatal(err)
		}
		esperado := nombre != "afuera"
		if ok != esperado {
			t.Errorf("%s: miembro=%v esperaba %v", nombre, ok, esperado)
		}
	}
}

func TestModeracionBorrado(t *testing.T) {
	e := setup(t)
	m, err := e.s.Guardar(e.ctx, e.seccionID, e.estID, "mensaje del estudiante")
	if err != nil {
		t.Fatal(err)
	}

	// el autor puede borrar lo suyo
	if _, err := e.s.Borrar(e.ctx, e.estID, m.ID); err != nil {
		t.Fatalf("autor borra lo suyo: %v", err)
	}
	m, _ = e.s.Guardar(e.ctx, e.seccionID, e.estID, "otro mensaje")

	// un extraño NO puede
	if _, err := e.s.Borrar(e.ctx, e.afueraID, m.ID); !errors.Is(err, ErrSinPermiso) {
		t.Errorf("afuera borró: %v", err)
	}
	// el profesor sí puede borrar lo ajeno
	if _, err := e.s.Borrar(e.ctx, e.profesorID, m.ID); err != nil {
		t.Errorf("profesor no pudo moderar: %v", err)
	}
	// doble borrado → no encontrado (existencia primero)
	if _, err := e.s.Borrar(e.ctx, e.profesorID, m.ID); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("doble borrado: %v", err)
	}
}
