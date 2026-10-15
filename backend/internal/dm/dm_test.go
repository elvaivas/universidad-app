package dm

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
	e.est2ID = insertarUsuario(t, ctx, pool, "est2@uni.test", "estudiante")
	e.afueraID = insertarUsuario(t, ctx, pool, "afuera@uni.test", "estudiante")
	_ = pool.QueryRow(ctx,
		`INSERT INTO secciones (materia_id, periodo_id, profesor_id, codigo, cupo) VALUES ($1::uuid,$2::uuid,$3::uuid,'1',30) RETURNING id::text`,
		mid, pid, e.profesorID).Scan(&e.seccionID)
	_, _ = pool.Exec(ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`, e.seccionID, e.estID)
	_, _ = pool.Exec(ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`, e.seccionID, e.est2ID)

	// sección ajena para "afuera"
	var cid2, pid2, mid2, sid2 string
	_ = pool.QueryRow(ctx, `INSERT INTO carreras (universidad_id, nombre) VALUES ($1::uuid,'Otra') RETURNING id::text`, uid).Scan(&cid2)
	_ = pool.QueryRow(ctx, `INSERT INTO periodos (carrera_id, nombre, fecha_inicio, fecha_fin) VALUES ($1::uuid,'2026-1','2026-01-12','2026-05-30') RETURNING id::text`, cid2).Scan(&pid2)
	_ = pool.QueryRow(ctx, `INSERT INTO materias (carrera_id, codigo, nombre) VALUES ($1::uuid,'PS-102','Otra') RETURNING id::text`, cid2).Scan(&mid2)
	_ = pool.QueryRow(ctx,
		`INSERT INTO secciones (materia_id, periodo_id, profesor_id, codigo, cupo) VALUES ($1::uuid,$2::uuid,$3::uuid,'1',30) RETURNING id::text`,
		mid2, pid2, e.profesorID).Scan(&sid2)
	_, _ = pool.Exec(ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`, sid2, e.afueraID)
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

func TestReglasDeInicio(t *testing.T) {
	e := setup(t)

	if _, err := e.s.CrearSolicitud(e.ctx, e.estID, e.estID, "hola"); !errors.Is(err, ErrAutoDM) {
		t.Errorf("auto-DM aceptado: %v", err)
	}
	// estudiante → profesor de SU sección: OK
	if _, err := e.s.CrearSolicitud(e.ctx, e.estID, e.profesorID, "duda sobre el parcial"); err != nil {
		t.Errorf("est→prof sección: %v", err)
	}
	// estudiante → profesor SIN sección compartida: bloqueado estructuralmente
	prof2 := insertarUsuario(t, e.ctx, e.s.Pool, "prof2@uni.test", "profesor")
	if _, err := e.s.CrearSolicitud(e.ctx, e.estID, prof2, "hola"); !errors.Is(err, ErrSinSeccionCompartida) {
		t.Errorf("est sin sección compartida: %v", err)
	}
	// estudiante → compañero de sección: OK
	if _, err := e.s.CrearSolicitud(e.ctx, e.estID, e.est2ID, "grupo de estudio"); err != nil {
		t.Errorf("est→compañero: %v", err)
	}
	// profesor → cualquiera (privilegio de inicio): OK
	if _, err := e.s.CrearSolicitud(e.ctx, e.profesorID, e.afueraID, "reunión de coordinación"); err != nil {
		t.Errorf("prof→cualquiera: %v", err)
	}
	// destinatario inexistente
	if _, err := e.s.CrearSolicitud(e.ctx, e.estID, "00000000-0000-0000-0000-000000000000", "x"); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("destinatario fantasma: %v", err)
	}
	// solicitud duplicada pendiente → 409
	if _, err := e.s.CrearSolicitud(e.ctx, e.estID, e.profesorID, "otra vez"); !errors.Is(err, ErrSolicitudExiste) {
		t.Errorf("duplicada: %v", err)
	}
}

func TestAceptarCreaConversacionUnica(t *testing.T) {
	e := setup(t)
	so, err := e.s.CrearSolicitud(e.ctx, e.estID, e.profesorID, "consulta")
	if err != nil {
		t.Fatal(err)
	}
	sos, _ := e.s.SolicitudesRecibidas(e.ctx, e.profesorID)
	if len(sos) != 1 || sos[0].Mensaje != "consulta" || sos[0].SolicitanteEma != "est@uni.test" {
		t.Fatalf("bandeja: %+v", sos)
	}

	convID, err := e.s.Aceptar(e.ctx, so.ID, e.profesorID)
	if err != nil || convID == "" {
		t.Fatalf("aceptar: %v", err)
	}
	if _, err := e.s.Aceptar(e.ctx, so.ID, e.profesorID); !errors.Is(err, ErrYaRespondida) {
		t.Errorf("doble aceptar: %v", err)
	}
	if _, err := e.s.Aceptar(e.ctx, so.ID, e.estID); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("no-destinatario respondió: %v", err)
	}
	// con conversación existente, nueva solicitud → ErrYaConversan
	if _, err := e.s.CrearSolicitud(e.ctx, e.estID, e.profesorID, "otra vez"); !errors.Is(err, ErrYaConversan) {
		t.Errorf("solicitud con conversación existente: %v", err)
	}
	convos, _ := e.s.ListarConversaciones(e.ctx, e.estID)
	if len(convos) != 1 || convos[0].OtroEma != "prof@uni.test" {
		t.Errorf("conversaciones: %+v", convos)
	}
}

func TestRechazarPermiteReintentar(t *testing.T) {
	e := setup(t)
	so, _ := e.s.CrearSolicitud(e.ctx, e.estID, e.profesorID, "intento 1")
	if err := e.s.Rechazar(e.ctx, so.ID, e.profesorID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.CrearSolicitud(e.ctx, e.estID, e.profesorID, "intento 2"); err != nil {
		t.Errorf("reintento tras rechazo: %v", err)
	}
	sos, _ := e.s.SolicitudesEnviadas(e.ctx, e.estID)
	if len(sos) != 2 || sos[0].Estado != "pendiente" || sos[1].Estado != "rechazada" {
		t.Errorf("enviadas: %+v", sos)
	}
}

func TestMensajesSoloEntreParticipantes(t *testing.T) {
	e := setup(t)
	so, _ := e.s.CrearSolicitud(e.ctx, e.estID, e.profesorID, "hola")
	convID, _ := e.s.Aceptar(e.ctx, so.ID, e.profesorID)

	m, err := e.s.GuardarMensaje(e.ctx, convID, e.estID, "buenas tardes profe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.GuardarMensaje(e.ctx, convID, e.afueraID, "intruso"); !errors.Is(err, ErrSinPermiso) {
		t.Errorf("intruso escribió: %v", err)
	}
	if _, err := e.s.GuardarMensaje(e.ctx, convID, e.estID, ""); !errors.Is(err, ErrCuerpoInvalido) {
		t.Errorf("vacío aceptado: %v", err)
	}
	datos, total, err := e.s.ListarMensajes(e.ctx, convID, e.profesorID, 50, 0)
	if err != nil || total != 1 || datos[0].ID != m.ID {
		t.Errorf("historial: %v %v", total, err)
	}
	if _, _, err := e.s.ListarMensajes(e.ctx, convID, e.afueraID, 50, 0); !errors.Is(err, ErrSinPermiso) {
		t.Errorf("intruso leyó: %v", err)
	}
	if _, err := e.s.BorrarMensaje(e.ctx, e.profesorID, m.ID); !errors.Is(err, ErrSinPermiso) {
		t.Errorf("no-autor borró en DM: %v", err)
	}
	if _, err := e.s.BorrarMensaje(e.ctx, e.estID, m.ID); err != nil {
		t.Errorf("autor no borró: %v", err)
	}
	if _, err := e.s.BorrarMensaje(e.ctx, e.estID, m.ID); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("doble borrado DM: %v", err)
	}
}
