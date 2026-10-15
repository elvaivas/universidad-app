package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/feed"
)

func servidorConFeed(t *testing.T) (*httptest.Server, *auth.Servicio) {
	t.Helper()
	srvViejo, svc := servidorPrueba(t)
	srvViejo.Close()

	httpSrv := NuevoServer(svc, svc.JWTSecreto, 50, 5)
	httpSrv.MontarFeed(&feed.Servicio{Pool: svc.Pool})
	srv := httptest.NewServer(httpSrv.Handler())
	t.Cleanup(srv.Close)
	return srv, svc
}

// preparaSeccionFeed crea la cadena + profesor + 2 inscritos + vocero + afuera.
func preparaSeccionFeed(t *testing.T, srv *httptest.Server, svc *auth.Servicio) (seccionID string, tokens map[string]string) {
	t.Helper()
	ctx := context.Background()
	var uid, cid, pid, mid string
	_ = svc.Pool.QueryRow(ctx, `INSERT INTO universidades (nombre) VALUES ('U') RETURNING id::text`).Scan(&uid)
	_ = svc.Pool.QueryRow(ctx, `INSERT INTO carreras (universidad_id, nombre) VALUES ($1::uuid,'Ing') RETURNING id::text`, uid).Scan(&cid)
	_ = svc.Pool.QueryRow(ctx, `INSERT INTO periodos (carrera_id, nombre, fecha_inicio, fecha_fin) VALUES ($1::uuid,'2026-1','2026-01-12','2026-05-30') RETURNING id::text`, cid).Scan(&pid)
	_ = svc.Pool.QueryRow(ctx, `INSERT INTO materias (carrera_id, codigo, nombre) VALUES ($1::uuid,'PS-101','Prog I') RETURNING id::text`, cid).Scan(&mid)
	profID := crearUsuarioRol(t, svc, "prof@uni.test", "profesor")
	if err := svc.Pool.QueryRow(ctx,
		`INSERT INTO secciones (materia_id, periodo_id, profesor_id, codigo, cupo) VALUES ($1::uuid,$2::uuid,$3::uuid,'1',30) RETURNING id::text`,
		mid, pid, profID).Scan(&seccionID); err != nil {
		t.Fatal(err)
	}

	tokens = map[string]string{"prof@uni.test": tokenLogin(t, srv, "prof@uni.test", "Clave#2026X")}
	for _, spec := range []struct {
		email string
		rol   string
	}{
		{"est@uni.test", "estudiante"},
		{"est2@uni.test", "estudiante"},
		{"vocero@uni.test", "estudiante"},
		{"afuera@uni.test", "estudiante"},
	} {
		crearUsuarioRol(t, svc, spec.email, spec.rol)
		tokens[spec.email] = tokenLogin(t, srv, spec.email, "Clave#2026X")
	}
	_, _ = svc.Pool.Exec(ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`,
		seccionID, uidDePool(t, svc, "est@uni.test"))
	_, _ = svc.Pool.Exec(ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`,
		seccionID, uidDePool(t, svc, "est2@uni.test"))
	_, _ = svc.Pool.Exec(ctx, `INSERT INTO vocerias (user_id, nivel, alcance_id) VALUES ($1::uuid,'clase',$2::uuid)`,
		uidDePool(t, svc, "vocero@uni.test"), seccionID)
	return
}

func uidDePool(t *testing.T, svc *auth.Servicio, email string) string {
	t.Helper()
	var id string
	if err := svc.Pool.QueryRow(context.Background(),
		`SELECT id::text FROM users WHERE email=$1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestFeedFlujoCompleto(t *testing.T) {
	srv, svc := servidorConFeed(t)
	seccionID, tokens := preparaSeccionFeed(t, srv, svc)
	tProf, tEst, tEst2 := tokens["prof@uni.test"], tokens["est@uni.test"], tokens["est2@uni.test"]
	tVoc, tAfuera := tokens["vocero@uni.test"], tokens["afuera@uni.test"]

	// publicar: afuera 403, estudiante 403, vocero 201, profesor 201
	code, _ := postConToken(t, srv.URL+"/api/secciones/"+seccionID+"/avisos", tAfuera, `{"titulo":"X","cuerpo":"Y"}`)
	if code != http.StatusForbidden {
		t.Errorf("afuera publica: %d", code)
	}
	code, _ = postConToken(t, srv.URL+"/api/secciones/"+seccionID+"/avisos", tEst, `{"titulo":"X","cuerpo":"Y"}`)
	if code != http.StatusForbidden {
		t.Errorf("estudiante publica: %d", code)
	}
	code, out := postConToken(t, srv.URL+"/api/secciones/"+seccionID+"/avisos", tVoc, `{"titulo":"Reunión","cuerpo":"Mañana 8am"}`)
	if code != http.StatusCreated {
		t.Fatalf("vocero publica: %d %v", code, out)
	}
	code, out = postConToken(t, srv.URL+"/api/secciones/"+seccionID+"/avisos", tProf, `{"titulo":"Parcial","cuerpo":"Viernes"}`)
	if code != http.StatusCreated {
		t.Fatalf("profesor publica: %d %v", code, out)
	}

	// leer feed: miembro 200 con 2 avisos; afuera 403
	code, out = getConToken(t, srv.URL+"/api/secciones/"+seccionID+"/avisos", tEst)
	if code != http.StatusOK || out["data"].(map[string]any)["total"].(float64) != 2 {
		t.Errorf("feed miembro: %d %v", code, out)
	}
	code, _ = getConToken(t, srv.URL+"/api/secciones/"+seccionID+"/avisos", tAfuera)
	if code != http.StatusForbidden {
		t.Errorf("feed afuera: %d", code)
	}

	// votación por el profesor
	code, out = postConToken(t, srv.URL+"/api/secciones/"+seccionID+"/votaciones", tProf,
		`{"titulo":"Viaje","pregunta":"¿Vamos al museo?","opciones":["Sí","No"],"multiple":false}`)
	if code != http.StatusCreated {
		t.Fatalf("crear votación: %d %v", code, out)
	}
	vid := out["data"].(map[string]any)["id"].(string)
	avisoID := out["data"].(map[string]any)["aviso_id"].(string)

	// comentar: miembro sí, afuera no
	code, _ = postConToken(t, srv.URL+"/api/avisos/"+avisoID+"/comentarios", tEst, `{"cuerpo":"+1"}`)
	if code != http.StatusCreated {
		t.Errorf("comentar miembro: %d", code)
	}
	code, _ = postConToken(t, srv.URL+"/api/avisos/"+avisoID+"/comentarios", tAfuera, `{"cuerpo":"spam"}`)
	if code != http.StatusForbidden {
		t.Errorf("comentar afuera: %d", code)
	}

	// votar: dos miembros; doble voto 409; afuera 403
	code, _ = postConToken(t, srv.URL+"/api/votaciones/"+vid+"/votar", tEst, `{"opciones":[0]}`)
	if code != http.StatusOK {
		t.Errorf("votar est: %d", code)
	}
	code, _ = postConToken(t, srv.URL+"/api/votaciones/"+vid+"/votar", tEst2, `{"opciones":[1]}`)
	if code != http.StatusOK {
		t.Errorf("votar est2: %d", code)
	}
	code, _ = postConToken(t, srv.URL+"/api/votaciones/"+vid+"/votar", tEst, `{"opciones":[1]}`)
	if code != http.StatusConflict {
		t.Errorf("doble voto: %d", code)
	}
	code, _ = postConToken(t, srv.URL+"/api/votaciones/"+vid+"/votar", tAfuera, `{"opciones":[0]}`)
	if code != http.StatusForbidden {
		t.Errorf("votar afuera: %d", code)
	}

	// resultados: conteo 1-1, participación 2
	code, out = getConToken(t, srv.URL+"/api/votaciones/"+vid+"/resultados", tEst)
	if code != http.StatusOK {
		t.Fatalf("resultados: %d", code)
	}
	res := out["data"].(map[string]any)
	if res["total_votos"].(float64) != 2 || res["participacion"].(float64) != 2 {
		t.Errorf("resultados: %v", res)
	}
	conteo := res["conteo"].([]any)
	if conteo[0].(map[string]any)["votos"].(float64) != 1 || conteo[1].(map[string]any)["votos"].(float64) != 1 {
		t.Errorf("conteo: %v", conteo)
	}

	// cerrar (profesor) y rechazar voto nuevo
	code, _ = postConToken(t, srv.URL+"/api/votaciones/"+vid+"/cerrar", tProf, `{}`)
	if code != http.StatusOK {
		t.Errorf("cerrar: %d", code)
	}
	code, _ = postConToken(t, srv.URL+"/api/votaciones/"+vid+"/votar", tVoc, `{"opciones":[0]}`)
	if code != http.StatusConflict {
		t.Errorf("votar cerrada: %d", code)
	}
	_ = fmt.Sprintf
}
