package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elvaivas/universidad-app/backend/internal/academic"
	"github.com/elvaivas/universidad-app/backend/internal/auth"
)

// servidorConAcademico: helpers del paquete + módulo académico montado.
func servidorConAcademico(t *testing.T) (*httptest.Server, *auth.Servicio) {
	t.Helper()
	srvViejo, svc := servidorPrueba(t)
	srvViejo.Close()

	httpSrv := NuevoServer(svc, svc.JWTSecreto, 50, 5)
	httpSrv.MontarAcademico(&academic.Servicio{Pool: svc.Pool})
	srv := httptest.NewServer(httpSrv.Handler())
	t.Cleanup(srv.Close)
	return srv, svc
}

// preparaCadena crea universidad→carrera→periodo→materia vía SQL directo.
func preparaCadena(t *testing.T, svc *auth.Servicio) (carreraID, periodoID, materiaID string) {
	t.Helper()
	ctx := context.Background()
	var uid string
	if err := svc.Pool.QueryRow(ctx, `INSERT INTO universidades (nombre) VALUES ('Univ HTTP') RETURNING id::text`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := svc.Pool.QueryRow(ctx, `INSERT INTO carreras (universidad_id, nombre) VALUES ($1::uuid,'Ing') RETURNING id::text`, uid).Scan(&carreraID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Pool.QueryRow(ctx, `INSERT INTO periodos (carrera_id, nombre, fecha_inicio, fecha_fin) VALUES ($1::uuid,'2026-1','2026-01-12','2026-05-30') RETURNING id::text`, carreraID).Scan(&periodoID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Pool.QueryRow(ctx, `INSERT INTO materias (carrera_id, codigo, nombre) VALUES ($1::uuid,'PS-101','Programación I') RETURNING id::text`, carreraID).Scan(&materiaID); err != nil {
		t.Fatal(err)
	}
	return
}

func crearUsuarioRol(t *testing.T, svc *auth.Servicio, email, rol string) string {
	t.Helper()
	hash, _ := auth.HashPassword("Clave#2026X")
	var id string
	if err := svc.Pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, nombre, rol_base) VALUES ($1,$2,$3,$4) RETURNING id::text`,
		email, hash, email, rol).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAcademicoFlujoCompleto(t *testing.T) {
	srv, svc := servidorConAcademico(t)
	carreraID, periodoID, materiaID := preparaCadena(t, svc)

	if _, err := svc.CrearMantenedorInicial(context.Background(), "boss@uni.test", "Boss#2026Clave", "Boss"); err != nil {
		t.Fatal(err)
	}
	tokBoss := tokenLogin(t, srv, "boss@uni.test", "Boss#2026Clave")
	crearUsuarioRol(t, svc, "prof@uni.test", "profesor")
	tokProf := tokenLogin(t, srv, "prof@uni.test", "Clave#2026X")
	_ = crearUsuarioRol(t, svc, "est@uni.test", "estudiante")
	tokEst := tokenLogin(t, srv, "est@uni.test", "Clave#2026X")

	// materia: solo mantenedor
	code, _ := postConToken(t, srv.URL+"/api/carreras/"+carreraID+"/materias", tokProf,
		`{"codigo":"PS-102","nombre":"X"}`)
	if code != http.StatusForbidden {
		t.Errorf("profesor creando materia: esperaba 403, obtuvo %d", code)
	}
	code, out := postConToken(t, srv.URL+"/api/carreras/"+carreraID+"/materias", tokBoss,
		`{"codigo":"PS-102","nombre":"Programación II"}`)
	if code != http.StatusCreated {
		t.Fatalf("crear materia: %d %v", code, out)
	}

	// sección: estudiante no puede; profesor sí (queda como dueño)
	code, _ = postConToken(t, srv.URL+"/api/materias/"+materiaID+"/secciones", tokEst,
		fmt.Sprintf(`{"periodo_id":%q,"codigo":"1","cupo":2}`, periodoID))
	if code != http.StatusForbidden {
		t.Errorf("estudiante creando sección: esperaba 403, obtuvo %d", code)
	}
	code, out = postConToken(t, srv.URL+"/api/materias/"+materiaID+"/secciones", tokProf,
		fmt.Sprintf(`{"periodo_id":%q,"codigo":"1","cupo":2}`, periodoID))
	if code != http.StatusCreated {
		t.Fatalf("crear sección: %d %v", code, out)
	}
	sid := out["data"].(map[string]any)["id"].(string)

	// inscripción del estudiante
	code, out = postConToken(t, srv.URL+"/api/secciones/"+sid+"/inscripcion", tokEst, `{}`)
	if code != http.StatusCreated {
		t.Fatalf("inscribir: %d %v", code, out)
	}
	code, _ = postConToken(t, srv.URL+"/api/secciones/"+sid+"/inscripcion", tokEst, `{}`)
	if code != http.StatusConflict {
		t.Errorf("inscripción duplicada: esperaba 409, obtuvo %d", code)
	}

	// cupo=2: segundo entra, tercero no
	crearUsuarioRol(t, svc, "est2@uni.test", "estudiante")
	tokEst2 := tokenLogin(t, srv, "est2@uni.test", "Clave#2026X")
	if code, _ := postConToken(t, srv.URL+"/api/secciones/"+sid+"/inscripcion", tokEst2, `{}`); code != http.StatusCreated {
		t.Fatalf("inscripción 2: %d", code)
	}
	crearUsuarioRol(t, svc, "est3@uni.test", "estudiante")
	tokEst3 := tokenLogin(t, srv, "est3@uni.test", "Clave#2026X")
	if code, _ := postConToken(t, srv.URL+"/api/secciones/"+sid+"/inscripcion", tokEst3, `{}`); code != http.StatusConflict {
		t.Errorf("sobrecupo: esperaba 409, obtuvo %d", code)
	}

	// lista de inscritos: externo → 403; profesor dueño → 200
	code, _ = getConToken(t, srv.URL+"/api/secciones/"+sid+"/inscritos", tokEst3)
	if code != http.StatusForbidden {
		t.Errorf("lista por no dueño: esperaba 403, obtuvo %d", code)
	}
	code, out = getConToken(t, srv.URL+"/api/secciones/"+sid+"/inscritos", tokProf)
	if code != http.StatusOK || out["data"].(map[string]any)["total"].(float64) != 2 {
		t.Errorf("lista por profesor: %d %v", code, out)
	}

	// baja libera cupo
	code, _ = delConToken(t, srv.URL+"/api/secciones/"+sid+"/inscripcion", tokEst2)
	if code != http.StatusOK {
		t.Errorf("baja: %d", code)
	}
	if code, _ := postConToken(t, srv.URL+"/api/secciones/"+sid+"/inscripcion", tokEst3, `{}`); code != http.StatusCreated {
		t.Errorf("tras baja debería haber cupo: %d", code)
	}

	// claim seccion_ids del estudiante recién inscrito
	tokEst3b := tokenLogin(t, srv, "est3@uni.test", "Clave#2026X")
	code, out = getConToken(t, srv.URL+"/api/auth/me", tokEst3b)
	me := out["data"].(map[string]any)
	ids := me["seccion_ids"].([]any)
	if len(ids) != 1 {
		t.Errorf("seccion_ids del inscrito: %v", ids)
	}
}
