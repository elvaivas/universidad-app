package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/catalog"
)

// servidorConCatalogo: mismos servicio/pool de servidorPrueba, con catálogo montado.
func servidorConCatalogo(t *testing.T) (*httptest.Server, *auth.Servicio) {
	t.Helper()
	srvViejo, svc := servidorPrueba(t)
	srvViejo.Close() // el server del helper base no expone el catálogo

	httpSrv := NuevoServer(svc, svc.JWTSecreto, 50, 5)
	httpSrv.MontarCatalogo(&catalog.Servicio{Pool: svc.Pool})
	srv := httptest.NewServer(httpSrv.Handler())
	t.Cleanup(srv.Close)
	return srv, svc
}

func tokenLogin(t *testing.T, srv *httptest.Server, email, clave string) string {
	t.Helper()
	code, out := postJSON(t, srv.URL+"/api/auth/login",
		fmt.Sprintf(`{"email":%q,"password":%q}`, email, clave))
	if code != http.StatusOK {
		t.Fatalf("login %s: %d %v", email, code, out)
	}
	return out["data"].(map[string]any)["access_token"].(string)
}

func postConToken(t *testing.T, url, token, cuerpo string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(cuerpo))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func delConToken(t *testing.T, url, token string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestCatalogoFlujoMantenedor(t *testing.T) {
	srv, svc := servidorConCatalogo(t)
	if _, err := svc.CrearMantenedorInicial(context.Background(), "boss@uni.test", "Boss#2026Clave", "Boss"); err != nil {
		t.Fatal(err)
	}
	tok := tokenLogin(t, srv, "boss@uni.test", "Boss#2026Clave")

	code, out := postConToken(t, srv.URL+"/api/universidades", tok,
		`{"nombre":"Universidad Central","color_primario":"#003"}`)
	if code != http.StatusCreated {
		t.Fatalf("crear universidad: %d %v", code, out)
	}
	uid := out["data"].(map[string]any)["id"].(string)

	code, out = getConToken(t, srv.URL+"/api/universidades?limit=10", tok)
	if code != http.StatusOK || out["data"].(map[string]any)["total"].(float64) != 1 {
		t.Errorf("listar: %d %v", code, out)
	}

	code, out = postConToken(t, srv.URL+"/api/universidades/"+uid+"/carreras", tok,
		`{"nombre":"Ingeniería Informática","tipo_periodo":"semestre"}`)
	if code != http.StatusCreated {
		t.Fatalf("crear carrera: %d %v", code, out)
	}
	cid := out["data"].(map[string]any)["id"].(string)

	code, out = postConToken(t, srv.URL+"/api/carreras/"+cid+"/periodos", tok,
		`{"nombre":"2026-1","fecha_inicio":"2026-01-12","fecha_fin":"2026-05-30"}`)
	if code != http.StatusCreated {
		t.Fatalf("crear periodo: %d %v", code, out)
	}

	code, out = getConToken(t, srv.URL+"/api/carreras/"+cid+"/periodos", tok)
	if code != http.StatusOK || out["data"].(map[string]any)["total"].(float64) != 1 {
		t.Errorf("listar periodos: %d %v", code, out)
	}

	code, out = delConToken(t, srv.URL+"/api/universidades/"+uid, tok)
	if code != http.StatusOK {
		t.Errorf("desactivar: %d %v", code, out)
	}
}

func TestCatalogoEscrituraRechazadaParaNoMantenedor(t *testing.T) {
	srv, _ := servidorConCatalogo(t)
	tok := tokenLogin(t, srv, "luis@uni.test", "clav3-Valida!") // estudiante

	code, _ := postConToken(t, srv.URL+"/api/universidades", tok, `{"nombre":"X"}`)
	if code != http.StatusForbidden {
		t.Errorf("estudiante creando universidad: esperaba 403, obtuvo %d", code)
	}
}

func TestCatalogoLecturaRechazadaSinToken(t *testing.T) {
	srv, _ := servidorConCatalogo(t)
	code, _ := getConToken(t, srv.URL+"/api/universidades", "")
	if code != http.StatusUnauthorized {
		t.Errorf("listar sin token: esperaba 401, obtuvo %d", code)
	}
}

func TestCatalogo404Y400(t *testing.T) {
	srv, svc := servidorConCatalogo(t)
	if _, err := svc.CrearMantenedorInicial(context.Background(), "boss@uni.test", "Boss#2026Clave", "B"); err != nil {
		t.Fatal(err)
	}
	tok := tokenLogin(t, srv, "boss@uni.test", "Boss#2026Clave")

	code, _ := getConToken(t, srv.URL+"/api/universidades/00000000-0000-0000-0000-000000000000", tok)
	if code != http.StatusNotFound {
		t.Errorf("get inexistente: esperaba 404, obtuvo %d", code)
	}

	code, _ = postConToken(t, srv.URL+"/api/universidades", tok, `{"nombre":""}`)
	if code != http.StatusBadRequest {
		t.Errorf("nombre vacío: esperaba 400, obtuvo %d", code)
	}
}
