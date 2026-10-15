package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
)

func servidorCon2FA(t *testing.T) (*httptest.Server, *auth.Servicio) {
	t.Helper()
	srvViejo, svc := servidorPrueba(t)
	srvViejo.Close()

	httpSrv := NuevoServer(svc, svc.JWTSecreto, 50, 5)
	httpSrv.Montar2FA(svc)
	srv := httptest.NewServer(httpSrv.Handler())
	t.Cleanup(srv.Close)
	return srv, svc
}

func Test2FAPorHTTP(t *testing.T) {
	srv, _ := servidorCon2FA(t)
	tok := tokenLogin(t, srv, "luis@uni.test", "clav3-Valida!")

	code, out := postConToken(t, srv.URL+"/api/auth/2fa/activar", tok, `{}`)
	if code != http.StatusOK {
		t.Fatalf("activar: %d %v", code, out)
	}
	sec := out["data"].(map[string]any)["secreto"].(string)

	codigo, err := auth.Codigo2FAParaTests(sec)
	if err != nil {
		t.Fatal(err)
	}
	code, out = postConToken(t, srv.URL+"/api/auth/2fa/confirmar", tok,
		`{"codigo":"`+codigo+`"}`)
	if code != http.StatusOK {
		t.Fatalf("confirmar: %d %v", code, out)
	}

	code, out = postJSON(t, srv.URL+"/api/auth/login",
		`{"email":"luis@uni.test","password":"clav3-Valida!"}`)
	if code != http.StatusOK {
		t.Fatalf("login: %d", code)
	}
	d := out["data"].(map[string]any)
	if d["requiere_2fa"] != true || d["pre_2fa_token"] == nil {
		t.Fatalf("sin pre-token: %v", d)
	}

	codigo2, _ := auth.Codigo2FAParaTests(sec)
	code, out = postJSON(t, srv.URL+"/api/auth/2fa/login",
		`{"pre_2fa_token":"`+d["pre_2fa_token"].(string)+`","codigo":"`+codigo2+`"}`)
	if code != http.StatusOK {
		t.Fatalf("login2fa: %d %v", code, out)
	}
	if out["data"].(map[string]any)["refresh_token"] == "" {
		t.Errorf("par real incompleto: %v", out)
	}
}

func TestPasswordEndpointRechazos(t *testing.T) {
	srv, _ := servidorCon2FA(t)
	tok := tokenLogin(t, srv, "luis@uni.test", "clav3-Valida!")

	code, _ := postConToken(t, srv.URL+"/api/auth/password", tok,
		`{"password_actual":"mala","password_nueva":"Nueva-Clave-2026"}`)
	if code != http.StatusUnauthorized {
		t.Errorf("clave actual mala: esperaba 401, obtuvo %d", code)
	}
	code, _ = postConToken(t, srv.URL+"/api/auth/password", tok,
		`{"password_actual":"clav3-Valida!","password_nueva":"corta"}`)
	if code != http.StatusBadRequest {
		t.Errorf("clave débil: esperaba 400, obtuvo %d", code)
	}

	code, out := postConToken(t, srv.URL+"/api/auth/password", tok,
		`{"password_actual":"clav3-Valida!","password_nueva":"Nueva-Clave-2026"}`)
	if code != http.StatusOK {
		t.Fatalf("cambio válido: %d %v", code, out)
	}
	if _, out := postJSON(t, srv.URL+"/api/auth/login",
		`{"email":"luis@uni.test","password":"Nueva-Clave-2026"}`); out["data"] == nil {
		t.Errorf("login con clave nueva falló: %v", out)
	}
}

func Test2FARutasRequierenAuth(t *testing.T) {
	srv, _ := servidorCon2FA(t)
	code, _ := postConToken(t, srv.URL+"/api/auth/2fa/activar", "", `{}`)
	if code != http.StatusUnauthorized {
		t.Errorf("activar sin token: esperaba 401, obtuvo %d", code)
	}
	code, _ = postConToken(t, srv.URL+"/api/auth/password", "",
		`{"password_actual":"x","password_nueva":"y"}`)
	if code != http.StatusUnauthorized {
		t.Errorf("password sin token: esperaba 401, obtuvo %d", code)
	}
}
