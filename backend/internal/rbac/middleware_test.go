package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
)

const secreto = "0123456789abcdef0123456789abcdef"

func tokenDe(t *testing.T, c auth.Claims) string {
	t.Helper()
	tok, err := auth.EmitirToken(c, secreto, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// handler que devuelve 200 y las claims que ve en el contexto (si hay).
func handlerEspia(w http.ResponseWriter, r *http.Request) {
	c := ClaimsDe(r)
	if c == nil {
		w.WriteHeader(http.StatusTeapot) // 418 = no inyectó claims
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(c.RolBase + "/" + c.Voceria))
}

func TestRequireAuthTabla(t *testing.T) {
	valido := tokenDe(t, auth.Claims{RolBase: "estudiante", Voceria: "clase"})
	expirado, _ := auth.EmitirToken(auth.Claims{RolBase: "estudiante"}, secreto, -time.Minute)
	otroSecreto, _ := auth.EmitirToken(auth.Claims{RolBase: "x"}, "otro-secreto-0123456789abcdef0123", time.Minute)

	casos := []struct {
		nombre string
		header string
		status int
		cuerpo string
	}{
		{"sin header", "", 401, ""},
		{"esquema basico", "Basic abc", 401, ""},
		{"token basura", "Bearer abc.def.ghi", 401, ""},
		{"token expirado", "Bearer " + expirado, 401, ""},
		{"token de otro secreto", "Bearer " + otroSecreto, 401, ""},
		{"token valido", "Bearer " + valido, 200, "estudiante/clase"},
	}
	for _, c := range casos {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		rec := httptest.NewRecorder()
		RequireAuth(secreto)(http.HandlerFunc(handlerEspia)).ServeHTTP(rec, req)
		if rec.Code != c.status {
			t.Errorf("%s: esperaba %d, obtuvo %d", c.nombre, c.status, rec.Code)
		}
		if c.cuerpo != "" && rec.Body.String() != c.cuerpo {
			t.Errorf("%s: cuerpo %q, esperaba %q", c.nombre, rec.Body.String(), c.cuerpo)
		}
	}
}

func TestRequireMantenedor(t *testing.T) {
	si := tokenDe(t, auth.Claims{RolBase: "profesor", EsMantenedor: true})
	no := tokenDe(t, auth.Claims{RolBase: "estudiante"})

	for nombre, tok := range map[string]string{"mantenedor": si, "estudiante": no} {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		RequireAuth(secreto)(RequireMantenedor(http.HandlerFunc(handlerEspia))).ServeHTTP(rec, req)
		esperado := http.StatusForbidden
		if nombre == "mantenedor" {
			esperado = http.StatusOK
		}
		if rec.Code != esperado {
			t.Errorf("%s: esperaba %d, obtuvo %d", nombre, esperado, rec.Code)
		}
	}
}

func TestRequireRol(t *testing.T) {
	profesor := tokenDe(t, auth.Claims{RolBase: "profesor"})
	estudiante := tokenDe(t, auth.Claims{RolBase: "estudiante"})

	casos := []struct {
		nombre string
		tok    string
		roles  []string
		status int
	}{
		{"profesor permitido", profesor, []string{"profesor", "estudiante"}, 200},
		{"estudiante en lista", estudiante, []string{"profesor", "estudiante"}, 200},
		{"estudiante fuera de lista", estudiante, []string{"profesor"}, 403},
	}
	for _, c := range casos {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Authorization", "Bearer "+c.tok)
		rec := httptest.NewRecorder()
		RequireAuth(secreto)(RequireRol(c.roles...)(http.HandlerFunc(handlerEspia))).ServeHTTP(rec, req)
		if rec.Code != c.status {
			t.Errorf("%s: esperaba %d, obtuvo %d", c.nombre, c.status, rec.Code)
		}
	}
}

func TestClaimsDeSinContexto(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if ClaimsDe(req) != nil {
		t.Error("ClaimsDe sin middleware debe devolver nil")
	}
}
