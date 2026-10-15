package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/testdb"
)

// servidorPrueba real: DB efímera + auth + http. Se salta sin TEST_DB_URL.
func servidorPrueba(t *testing.T) (*httptest.Server, *auth.Servicio) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.Nueva(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	svc := &auth.Servicio{
		Pool: pool, JWTSecreto: "0123456789abcdef0123456789abcdef",
		AccessTTL: 15 * time.Minute, RefreshTTL: time.Hour,
	}
	hash, _ := auth.HashPassword("clav3-Valida!")
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (email, password_hash, nombre, rol_base) VALUES ('luis@uni.test', $1, 'Luis', 'estudiante')`, hash); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NuevoServer(svc, svc.JWTSecreto, 50, 5).Handler())
	t.Cleanup(srv.Close)
	return srv, svc
}

func postJSON(t *testing.T, url string, cuerpo string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(cuerpo))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func getConToken(t *testing.T, url, token string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
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

func TestHealthz(t *testing.T) {
	srv, _ := servidorPrueba(t)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %d", resp.StatusCode)
	}
	if out["data"].(map[string]any)["status"] != "ok" {
		t.Errorf("healthz sin data.status: %v", out)
	}
}

func TestLoginFlujoCompleto(t *testing.T) {
	srv, svc := servidorPrueba(t)

	code, out := postJSON(t, srv.URL+"/api/auth/login",
		`{"email":"luis@uni.test","password":"clav3-Valida!"}`)
	if code != http.StatusOK {
		t.Fatalf("login: %d %v", code, out)
	}
	data := out["data"].(map[string]any)
	if data["access_token"] == "" || data["refresh_token"] == "" {
		t.Fatalf("par incompleto: %v", data)
	}
	if _, err := auth.ValidarToken(data["access_token"].(string), svc.JWTSecreto); err != nil {
		t.Errorf("access token inválido: %v", err)
	}
	code, out = postJSON(t, srv.URL+"/api/auth/refresh",
		`{"refresh_token":"`+data["refresh_token"].(string)+`"}`)
	if code != http.StatusOK {
		t.Fatalf("refresh: %d %v", code, out)
	}
	code, _ = postJSON(t, srv.URL+"/api/auth/refresh",
		`{"refresh_token":"`+data["refresh_token"].(string)+`"}`)
	if code != http.StatusUnauthorized {
		t.Errorf("replay no dio 401: %d", code)
	}
}

func TestLoginErroresJSONUniformes(t *testing.T) {
	srv, _ := servidorPrueba(t)
	casos := []struct {
		nombre string
		cuerpo string
		status int
	}{
		{"clave mala", `{"email":"luis@uni.test","password":"nope"}`, 401},
		{"email inexistente", `{"email":"nadie@uni.test","password":"nope"}`, 401},
		{"json roto", `{"email":`, 400},
		{"campos vacíos", `{"email":"","password":""}`, 400},
	}
	for _, c := range casos {
		code, out := postJSON(t, srv.URL+"/api/auth/login", c.cuerpo)
		if code != c.status {
			t.Errorf("%s: esperaba %d, obtuvo %d (%v)", c.nombre, c.status, code, out)
		}
		if _, ok := out["error"].(string); !ok {
			t.Errorf("%s: respuesta sin campo error uniforme: %v", c.nombre, out)
		}
	}
}

func TestRateLimitDevuelve429(t *testing.T) {
	// limiter muy estricto: ráfaga 2, sin recarga apreciable; no toca DB
	srv := httptest.NewServer(NuevoServer(servicioFalso{}, "x", 0.001, 2).Handler())
	defer srv.Close()
	var ultimo int
	for i := 0; i < 5; i++ {
		code, _ := postJSON(t, srv.URL+"/api/auth/login", `{"email":"a@b.c","password":"x"}`)
		ultimo = code
		if code == http.StatusTooManyRequests {
			break
		}
	}
	if ultimo != http.StatusTooManyRequests {
		t.Errorf("esperaba 429 tras agotar ráfaga, obtuvo %d", ultimo)
	}
}

func TestMeDevuelveClaimsDelToken(t *testing.T) {
	srv, _ := servidorPrueba(t)
	_, login := postJSON(t, srv.URL+"/api/auth/login",
		`{"email":"luis@uni.test","password":"clav3-Valida!"}`)
	tok := login["data"].(map[string]any)["access_token"].(string)

	code, out := getConToken(t, srv.URL+"/api/auth/me", tok)
	if code != http.StatusOK {
		t.Fatalf("/me: %d %v", code, out)
	}
	me := out["data"].(map[string]any)
	if me["rol_base"] != "estudiante" {
		t.Errorf("claims inesperadas: %v", me)
	}

	code, _ = getConToken(t, srv.URL+"/api/auth/me", "")
	if code != http.StatusUnauthorized {
		t.Errorf("/me sin token: esperaba 401, obtuvo %d", code)
	}
}

func TestAdminPingSoloMantenedor(t *testing.T) {
	srv, svc := servidorPrueba(t)
	// estudiante logueado → 403 en ruta admin
	_, login := postJSON(t, srv.URL+"/api/auth/login",
		`{"email":"luis@uni.test","password":"clav3-Valida!"}`)
	tokEst := login["data"].(map[string]any)["access_token"].(string)
	code, _ := getConToken(t, srv.URL+"/api/admin/ping", tokEst)
	if code != http.StatusForbidden {
		t.Errorf("estudiante en /admin/ping: esperaba 403, obtuvo %d", code)
	}

	// mantenedor creado en la misma base → 200
	if _, err := svc.CrearMantenedorInicial(context.Background(), "boss@uni.test", "Boss#2026Clave", "Boss"); err != nil {
		t.Fatal(err)
	}
	par, err := svc.Login(context.Background(), "boss@uni.test", "Boss#2026Clave", "", "")
	if err != nil {
		t.Fatal(err)
	}
	code, out := getConToken(t, srv.URL+"/api/admin/ping", par.AccessToken)
	if code != http.StatusOK {
		t.Errorf("mantenedor en /admin/ping: esperaba 200, obtuvo %d (%v)", code, out)
	}
}

type servicioFalso struct{}

func (servicioFalso) Login(_ context.Context, _, _, _, _ string) (*auth.ParTokens, error) {
	return nil, auth.ErrCredenciales
}
func (servicioFalso) Refresh(_ context.Context, _, _, _ string) (*auth.ParTokens, error) {
	return nil, auth.ErrRefreshInvalido
}
