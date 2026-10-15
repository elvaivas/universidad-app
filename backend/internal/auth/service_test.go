package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elvaivas/universidad-app/backend/internal/testdb"
)

// setupServicio crea una base efímera migrada por test (aislamiento total).
func setupServicio(t *testing.T) *Servicio {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.Nueva(t))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return &Servicio{
		Pool: pool, JWTSecreto: "0123456789abcdef0123456789abcdef",
		AccessTTL: 15 * time.Minute, RefreshTTL: time.Hour,
	}
}

func crearUsuarioPrueba(t *testing.T, s *Servicio, email, clave string) {
	t.Helper()
	hash, err := HashPassword(clave)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Pool.Exec(context.Background(),
		`INSERT INTO users (email, password_hash, nombre, rol_base)
		 VALUES ($1, $2, 'Prueba', 'estudiante')`, email, hash)
	if err != nil {
		t.Fatalf("insertar usuario: %v", err)
	}
}

func TestLoginOkEmiteParConClaims(t *testing.T) {
	s := setupServicio(t)
	crearUsuarioPrueba(t, s, "ana@uni.test", "clav3-Valida!")
	ctx := context.Background()

	par, err := s.Login(ctx, "ANA@uni.test", "clav3-Valida!", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if par.RefreshToken == "" || par.TokenType != "Bearer" || par.ExpiresIn != 900 {
		t.Errorf("par incompleto: %+v", par)
	}
	c, err := ValidarToken(par.AccessToken, s.JWTSecreto)
	if err != nil {
		t.Fatalf("access token no válido: %v", err)
	}
	if c.RolBase != "estudiante" || c.EsMantenedor {
		t.Errorf("claims incorrectos: %+v", c)
	}
}

func TestLoginClaveIncorrectaNoFiltraUsuario(t *testing.T) {
	s := setupServicio(t)
	crearUsuarioPrueba(t, s, "ana@uni.test", "clav3-Valida!")

	_, err := s.Login(context.Background(), "ana@uni.test", "mala", "", "")
	if !errors.Is(err, ErrCredenciales) {
		t.Errorf("esperaba ErrCredenciales, obtuvo %v", err)
	}
	// mismo error para email inexistente: no debe distinguirse
	_, err = s.Login(context.Background(), "fantasma@uni.test", "mala", "", "")
	if !errors.Is(err, ErrCredenciales) {
		t.Errorf("email inexistente revelado: %v", err)
	}
}

func TestLoginUsuarioInactivo(t *testing.T) {
	s := setupServicio(t)
	crearUsuarioPrueba(t, s, "ana@uni.test", "clav3-Valida!")
	_, err := s.Pool.Exec(context.Background(), `UPDATE users SET activo = false WHERE email = $1`, "ana@uni.test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Login(context.Background(), "ana@uni.test", "clav3-Valida!", "", "")
	if !errors.Is(err, ErrUsuarioInactivo) {
		t.Errorf("esperaba ErrUsuarioInactivo, obtuvo %v", err)
	}
}

func TestRefreshRotaYRechazaReplay(t *testing.T) {
	s := setupServicio(t)
	crearUsuarioPrueba(t, s, "ana@uni.test", "clav3-Valida!")
	ctx := context.Background()

	par1, err := s.Login(ctx, "ana@uni.test", "clav3-Valida!", "", "")
	if err != nil {
		t.Fatal(err)
	}
	par2, err := s.Refresh(ctx, par1.RefreshToken, "", "")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if par2.RefreshToken == par1.RefreshToken {
		t.Error("rotación no emitió refresh nuevo")
	}
	// replay del token ya rotado → rechazo + cadena revocada
	if _, err := s.Refresh(ctx, par1.RefreshToken, "", ""); !errors.Is(err, ErrRefreshInvalido) {
		t.Errorf("replay aceptado: %v", err)
	}
	// la cadena comprometida: incluso el token vigente queda revocado
	if _, err := s.Refresh(ctx, par2.RefreshToken, "", ""); !errors.Is(err, ErrRefreshInvalido) {
		t.Errorf("cadena no fue revocada tras replay: %v", err)
	}
}

func TestRefreshBasura(t *testing.T) {
	s := setupServicio(t)
	if _, err := s.Refresh(context.Background(), "token-inventado", "", ""); !errors.Is(err, ErrRefreshInvalido) {
		t.Errorf("esperaba ErrRefreshInvalido, obtuvo %v", err)
	}
}

func TestSeedAdminCreaMantenedorConForceChange(t *testing.T) {
	s := setupServicio(t)
	ctx := context.Background()

	u, err := s.CrearMantenedorInicial(ctx, "admin@uni.test", "Admin#2026Seguro", "Admin Central")
	if err != nil {
		t.Fatalf("CrearMantenedorInicial: %v", err)
	}
	if !u.EsMantenedor || !u.ForcePwdChange || u.RolBase != "profesor" {
		t.Errorf("mantenedor mal configurado: %+v", u)
	}
	par, err := s.Login(ctx, "admin@uni.test", "Admin#2026Seguro", "", "")
	if err != nil {
		t.Fatalf("login del mantenedor: %v", err)
	}
	c, _ := ValidarToken(par.AccessToken, s.JWTSecreto)
	if !c.EsMantenedor {
		t.Error("claim es_mantenedor ausente en el JWT")
	}
	if !par.ForcePwdChange {
		t.Error("force_password_change no propagado")
	}
	// no sobrescribe un email existente
	if _, err := s.CrearMantenedorInicial(ctx, "admin@uni.test", "Otra#Clave9", "X"); err == nil {
		t.Error("seed sobrescribió un mantenedor existente sin error")
	}
}
