package audit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elvaivas/universidad-app/backend/internal/testdb"
)

func TestRegistraEventoConActorYJSON(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testdb.Nueva(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx := context.Background()

	actorID := crearActor(t, ctx, pool)
	Registrar(ctx, pool, actorID, "permisos.otorgar_voceria", "voceria", "uuid-alcance",
		map[string]any{"nivel": ""}, map[string]any{"nivel": "carrera"}, "127.0.0.1")

	var accion, objetoTipo string
	var actor *string
	var despues []byte
	err = pool.QueryRow(ctx,
		`SELECT accion, objeto_tipo, actor_id::text, despues_json FROM audit_log`).Scan(
		&accion, &objetoTipo, &actor, &despues)
	if err != nil {
		t.Fatalf("leer audit_log: %v", err)
	}
	if accion != "permisos.otorgar_voceria" || objetoTipo != "voceria" {
		t.Errorf("evento mal registrado: %s %s", accion, objetoTipo)
	}
	if actor == nil || *actor != actorID {
		t.Errorf("actor no registrado: %v", actor)
	}
	// jsonb reformatea el texto: se compara semánticamente, no como string
	var estado map[string]any
	if err := json.Unmarshal(despues, &estado); err != nil {
		t.Fatalf("despues_json ilegible: %v", err)
	}
	if estado["nivel"] != "carrera" {
		t.Errorf("despues_json: %s", despues)
	}
}

func TestRegistraSinActorNiJSON(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testdb.Nueva(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx := context.Background()

	// evento anónimo (ej: login fallido, no sabemos quién es): no debe fallar
	Registrar(ctx, pool, "", "auth.login_fallido", "user", "", nil, nil, "")

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE accion='auth.login_fallido'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("evento anónimo no registrado: n=%d err=%v", n, err)
	}
}

func crearActor(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, nombre, rol_base)
		 VALUES ('actor@uni.test', 'x', 'Actor', 'profesor') RETURNING id::text`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
