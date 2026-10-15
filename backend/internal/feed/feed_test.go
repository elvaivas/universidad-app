package feed

import (
	"context"
	"errors"
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
	est2ID     string
	afueraID   string
	voceroID   string
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
	if err := pool.QueryRow(ctx, `INSERT INTO universidades (nombre) VALUES ('U') RETURNING id::text`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO carreras (universidad_id, nombre) VALUES ($1::uuid,'Ing') RETURNING id::text`, uid).Scan(&cid); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO periodos (carrera_id, nombre, fecha_inicio, fecha_fin) VALUES ($1::uuid,'2026-1','2026-01-12','2026-05-30') RETURNING id::text`, cid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO materias (carrera_id, codigo, nombre) VALUES ($1::uuid,'PS-101','Prog I') RETURNING id::text`, cid).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	e.profesorID = insertarUsuario(t, ctx, pool, "prof@uni.test", "profesor")
	if err := pool.QueryRow(ctx,
		`INSERT INTO secciones (materia_id, periodo_id, profesor_id, codigo, cupo) VALUES ($1::uuid,$2::uuid,$3::uuid,'1',30) RETURNING id::text`,
		mid, pid, e.profesorID).Scan(&e.seccionID); err != nil {
		t.Fatal(err)
	}
	e.estID = insertarUsuario(t, ctx, pool, "est@uni.test", "estudiante")
	e.est2ID = insertarUsuario(t, ctx, pool, "est2@uni.test", "estudiante")
	e.afueraID = insertarUsuario(t, ctx, pool, "afuera@uni.test", "estudiante")
	e.voceroID = insertarUsuario(t, ctx, pool, "vocero@uni.test", "estudiante")
	_, _ = pool.Exec(ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`, e.seccionID, e.estID)
	_, _ = pool.Exec(ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`, e.seccionID, e.est2ID)
	_, _ = pool.Exec(ctx, `INSERT INTO vocerias (user_id, nivel, alcance_id) VALUES ($1::uuid,'clase',$2::uuid)`, e.voceroID, e.seccionID)
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

func TestPermisosSeccion(t *testing.T) {
	e := setup(t)
	casos := []struct {
		nombre   string
		userID   string
		miembro  bool
		publicar bool
	}{
		{"profesor", e.profesorID, true, true},
		{"inscrito", e.estID, true, false},
		{"vocero de clase", e.voceroID, true, true},
		{"afuera", e.afueraID, false, false},
	}
	for _, c := range casos {
		m, err := e.s.EsMiembro(e.ctx, c.userID, e.seccionID)
		if err != nil || m != c.miembro {
			t.Errorf("%s miembro=%v (esperaba %v) err=%v", c.nombre, m, c.miembro, err)
		}
		p, err := e.s.PuedePublicar(e.ctx, c.userID, e.seccionID)
		if err != nil || p != c.publicar {
			t.Errorf("%s publicar=%v (esperaba %v) err=%v", c.nombre, p, c.publicar, err)
		}
	}
}

func TestAvisosYComentarios(t *testing.T) {
	e := setup(t)
	a, err := e.s.PublicarAviso(e.ctx, e.seccionID, e.profesorID, "Parcial", "El viernes 10am")
	if err != nil {
		t.Fatalf("PublicarAviso: %v", err)
	}
	if a.Tipo != "aviso" {
		t.Errorf("tipo: %s", a.Tipo)
	}

	p, err := e.s.ListarAvisos(e.ctx, e.seccionID, 10, 0)
	if err != nil || p.Total != 1 {
		t.Fatalf("ListarAvisos: %v %v", p.Total, err)
	}

	c, err := e.s.Comentar(e.ctx, a.ID, e.estID, "¿Es en el aula 4?")
	if err != nil {
		t.Fatalf("Comentar: %v", err)
	}
	pc, _ := e.s.ListarComentarios(e.ctx, a.ID, 10, 0)
	if pc.Total != 1 || pc.Datos[0].(Comentario).ID != c.ID {
		t.Errorf("comentarios: %+v", pc)
	}

	if _, err := e.s.PublicarAviso(e.ctx, e.seccionID, e.profesorID, "  ", "x"); !errors.Is(err, ErrDatosInvalidos) {
		t.Errorf("título vacío aceptado: %v", err)
	}
}

func TestVotacionSecretoMinimo(t *testing.T) {
	e := setup(t)
	v, err := e.s.CrearVotacion(e.ctx, e.seccionID, e.profesorID,
		"Elección", "¿Aprobamos la fecha del viaje?", []string{"Sí", "No", "Abstención"}, false)
	if err != nil {
		t.Fatalf("CrearVotacion: %v", err)
	}
	if len(v.Opciones) != 3 {
		t.Fatalf("opciones: %v", v.Opciones)
	}

	if err := e.s.Votar(e.ctx, v.ID, e.estID, []int{0}); err != nil {
		t.Fatalf("Votar est: %v", err)
	}
	if err := e.s.Votar(e.ctx, v.ID, e.est2ID, []int{1}); err != nil {
		t.Fatalf("Votar est2: %v", err)
	}

	if err := e.s.Votar(e.ctx, v.ID, e.estID, []int{1}); !errors.Is(err, ErrYaVoto) {
		t.Errorf("doble voto aceptado: %v", err)
	}
	if err := e.s.Votar(e.ctx, v.ID, e.afueraID, []int{9}); !errors.Is(err, ErrOpcionInvalida) {
		t.Errorf("opción inválida aceptada: %v", err)
	}

	res, err := e.s.Resultados(e.ctx, v.ID)
	if err != nil {
		t.Fatalf("Resultados: %v", err)
	}
	if res.TotalVotos != 2 || res.Participacion != 2 {
		t.Errorf("totales: votos=%d participacion=%d", res.TotalVotos, res.Participacion)
	}
	if res.Conteo[0].Votos != 1 || res.Conteo[1].Votos != 1 {
		t.Errorf("conteo: %+v", res.Conteo)
	}

	// cierre: nuevos votos rechazados
	if err := e.s.CerrarVotacion(e.ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	tercero := insertarUsuario(t, e.ctx, e.s.Pool, "est3@uni.test", "estudiante")
	_, _ = e.s.Pool.Exec(e.ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`, e.seccionID, tercero)
	if err := e.s.Votar(e.ctx, v.ID, tercero, []int{0}); !errors.Is(err, ErrVotacionCerrada) {
		t.Errorf("votación cerrada aceptó voto: %v", err)
	}
}

func TestVotacionMultipleYValidaciones(t *testing.T) {
	e := setup(t)
	if _, err := e.s.CrearVotacion(e.ctx, e.seccionID, e.profesorID, "T", "P?", []string{"solo"}, false); !errors.Is(err, ErrDatosInvalidos) {
		t.Errorf("votación de 1 opción aceptada: %v", err)
	}
	v, err := e.s.CrearVotacion(e.ctx, e.seccionID, e.voceroID,
		"Preferencias", "¿Qué temas repasamos?", []string{"A", "B", "C"}, true)
	if err != nil {
		t.Fatalf("votación múltiple por vocero: %v", err)
	}
	if err := e.s.Votar(e.ctx, v.ID, e.estID, []int{0, 2}); err != nil {
		t.Errorf("voto múltiple válido rechazado: %v", err)
	}
	res, _ := e.s.Resultados(e.ctx, v.ID)
	if res.TotalVotos != 2 || res.Participacion != 1 {
		t.Errorf("múltiple: votos=%d participacion=%d", res.TotalVotos, res.Participacion)
	}
}

func TestListarAvisosConVotacionEmbebida(t *testing.T) {
	e := setup(t)
	if _, err := e.s.PublicarAviso(e.ctx, e.seccionID, e.profesorID, "Aviso simple", "sin votación"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.CrearVotacion(e.ctx, e.seccionID, e.profesorID, "Votación", "¿Pregunta?", []string{"Sí", "No"}, false); err != nil {
		t.Fatal(err)
	}

	p, err := e.s.ListarAvisos(e.ctx, e.seccionID, 10, 0)
	if err != nil || p.Total != 2 {
		t.Fatalf("listar: %v %v", p.Total, err)
	}
	vot := p.Datos[0].(Aviso)
	if vot.Tipo != "votacion" || vot.Votacion == nil {
		t.Fatalf("votación no embebida: %+v", vot)
	}
	if len(vot.Votacion.Opciones) != 2 || vot.Votacion.Cerrada {
		t.Errorf("votación resumen: %+v", vot.Votacion)
	}
	aviso := p.Datos[1].(Aviso)
	if aviso.Votacion != nil {
		t.Errorf("aviso simple no debe traer votación: %+v", aviso.Votacion)
	}
}
