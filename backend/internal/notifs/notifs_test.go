package notifs

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/testdb"
)

func setup(t *testing.T) (context.Context, *pgxpool.Pool, string, string) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.Nueva(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()
	hash, _ := auth.HashPassword("Clave#2026X")
	var a, b string
	_ = pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, nombre, rol_base) VALUES ('a@uni.test',$1,'A','estudiante') RETURNING id::text`, hash).Scan(&a)
	_ = pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, nombre, rol_base) VALUES ('b@uni.test',$1,'B','profesor') RETURNING id::text`, hash).Scan(&b)
	return ctx, pool, a, b
}

func TestPanelMarcadoYRetencion(t *testing.T) {
	ctx, pool, a, b := setup(t)

	if err := Crear(ctx, pool, a, TipoDMSolicitud, "ref-1", "solicitud de prueba"); err != nil {
		t.Fatal(err)
	}
	if err := Crear(ctx, pool, a, TipoAnuncioGeneral, "ref-2", "Universidad: feriado"); err != nil {
		t.Fatal(err)
	}
	if err := Crear(ctx, pool, b, TipoDMSolicitud, "ref-x", "ajena"); err != nil {
		t.Fatal(err)
	}

	p, err := Listar(ctx, pool, a)
	if err != nil {
		t.Fatal(err)
	}
	if p.TotalNoLeidas != 2 || len(p.NoLeidas) != 2 {
		t.Fatalf("no leídas: %d", p.TotalNoLeidas)
	}

	// marcar por destino (auto-al-abrir)
	if err := MarcarLeidaPorRef(ctx, pool, a, TipoDMSolicitud, "ref-1"); err != nil {
		t.Fatal(err)
	}
	p, _ = Listar(ctx, pool, a)
	if p.TotalNoLeidas != 1 {
		t.Fatalf("tras marcar: %d", p.TotalNoLeidas)
	}
	if len(p.Recientes) != 1 || p.Recientes[0].Titulo != "solicitud de prueba" {
		t.Errorf("historial: %+v", p.Recientes)
	}

	// marcar una concreta por id
	if err := MarcarLeida(ctx, pool, a, p.NoLeidas[0].ID); err != nil {
		t.Fatal(err)
	}
	p, _ = Listar(ctx, pool, a)
	if p.TotalNoLeidas != 0 {
		t.Fatalf("todas leídas: %d", p.TotalNoLeidas)
	}
}

func TestFanOutMiembrosSeccionYNoAutonotificacion(t *testing.T) {
	ctx, pool, a, b := setup(t)

	var uid, cid, pid, mid, sid string
	_ = pool.QueryRow(ctx, `INSERT INTO universidades (nombre) VALUES ('U') RETURNING id::text`).Scan(&uid)
	_ = pool.QueryRow(ctx, `INSERT INTO carreras (universidad_id, nombre) VALUES ($1::uuid,'Ing') RETURNING id::text`, uid).Scan(&cid)
	_ = pool.QueryRow(ctx, `INSERT INTO periodos (carrera_id, nombre, fecha_inicio, fecha_fin) VALUES ($1::uuid,'2026-1','2026-01-12','2026-05-30') RETURNING id::text`, cid).Scan(&pid)
	_ = pool.QueryRow(ctx, `INSERT INTO materias (carrera_id, codigo, nombre) VALUES ($1::uuid,'PS-101','Prog') RETURNING id::text`, cid).Scan(&mid)
	_ = pool.QueryRow(ctx, `INSERT INTO secciones (materia_id, periodo_id, profesor_id, codigo, cupo) VALUES ($1::uuid,$2::uuid,$3::uuid,'1',30) RETURNING id::text`, mid, pid, b).Scan(&sid)
	_, _ = pool.Exec(ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`, sid, a)

	// el profesor (b) publica: se notifica al estudiante (a), NO a sí mismo
	if err := CrearParaMiembrosSeccion(ctx, pool, sid, b, TipoChatAnuncio, sid, "Anuncio"); err != nil {
		t.Fatal(err)
	}
	pa, _ := Listar(ctx, pool, a)
	if pa.TotalNoLeidas != 1 {
		t.Errorf("estudiante no notificado: %d", pa.TotalNoLeidas)
	}
	pb, _ := Listar(ctx, pool, b)
	if pb.TotalNoLeidas != 0 {
		t.Errorf("autor se autonotificó: %d", pb.TotalNoLeidas)
	}
}
