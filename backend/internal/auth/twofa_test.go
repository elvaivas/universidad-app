package auth

import (
	"context"
	"errors"
	"testing"
)

func TestFlujo2FACompleto(t *testing.T) {
	s := setupServicio(t)
	crearUsuarioPrueba(t, s, "ana@uni.test", "clav3-Valida!")
	ctx := context.Background()

	// 1) login directo antes de activar
	if _, err := s.Login(ctx, "ana@uni.test", "clav3-Valida!", "", ""); err != nil {
		t.Fatalf("login sin 2FA: %v", err)
	}

	// 2) activar: secreto pendiente, el login sigue siendo directo
	sec, err := s.Activar2FA(ctx, idDe(t, s, "ana@uni.test"))
	if err != nil {
		t.Fatalf("Activar2FA: %v", err)
	}
	if sec.Secreto == "" || sec.QRPNGBase64 == "" || sec.OtpauthURL == "" {
		t.Fatalf("secreto incompleto: %+v", sec)
	}
	par, err := s.Login(ctx, "ana@uni.test", "clav3-Valida!", "", "")
	if err != nil || par.Requiere2FA {
		t.Fatalf("login bloqueado con 2FA pendiente: %v %+v", err, par)
	}

	// 3) confirmar: código malo rechazado, código bueno activa
	if err := s.Confirmar2FA(ctx, idDe(t, s, "ana@uni.test"), "000000"); !errors.Is(err, ErrCodigo2FAInvalido) {
		t.Errorf("código malo aceptado: %v", err)
	}
	codigo, err := Codigo2FAParaTests(sec.Secreto)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Confirmar2FA(ctx, idDe(t, s, "ana@uni.test"), codigo); err != nil {
		t.Fatalf("Confirmar2FA: %v", err)
	}

	// 4) ahora el login exige segundo factor y entrega pre-token
	pre, err := s.Login(ctx, "ana@uni.test", "clav3-Valida!", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !pre.Requiere2FA || pre.AccessToken == "" || pre.TokenType != "Pre2FA" {
		t.Fatalf("pre-token incompleto: %+v", pre)
	}

	// 5) segundo paso: código malo 401; código bueno → par real
	if _, err := s.Login2FA(ctx, pre.AccessToken, "000000", "", ""); !errors.Is(err, ErrCodigo2FAInvalido) {
		t.Errorf("código 2FA malo aceptado: %v", err)
	}
	codigo2, _ := Codigo2FAParaTests(sec.Secreto)
	par2, err := s.Login2FA(ctx, pre.AccessToken, codigo2, "", "")
	if err != nil {
		t.Fatalf("Login2FA: %v", err)
	}
	if par2.Requiere2FA || par2.RefreshToken == "" {
		t.Fatalf("par real incompleto: %+v", par2)
	}

	// 6) reuso del pre-token → rechazo (un solo uso)
	codigo3, _ := Codigo2FAParaTests(sec.Secreto)
	if _, err := s.Login2FA(ctx, pre.AccessToken, codigo3, "", ""); !errors.Is(err, ErrPreTokenInvalido) {
		t.Errorf("reuso del pre-token aceptado: %v", err)
	}

	// 7) desactivar con código bueno → login directo otra vez
	codigo4, _ := Codigo2FAParaTests(sec.Secreto)
	if err := s.Desactivar2FA(ctx, idDe(t, s, "ana@uni.test"), codigo4); err != nil {
		t.Fatalf("Desactivar2FA: %v", err)
	}
	if _, err := s.Login(ctx, "ana@uni.test", "clav3-Valida!", "", ""); err != nil {
		t.Errorf("login directo tras desactivar: %v", err)
	}
}

func TestCambiarPassword(t *testing.T) {
	s := setupServicio(t)
	crearUsuarioPrueba(t, s, "ana@uni.test", "clav3-Valida!")
	ctx := context.Background()
	uid := idDe(t, s, "ana@uni.test")

	if err := s.CambiarPassword(ctx, uid, "mala", "Nueva-Clave-2026", ""); !errors.Is(err, ErrCredenciales) {
		t.Errorf("clave actual no validada: %v", err)
	}
	if err := s.CambiarPassword(ctx, uid, "clav3-Valida!", "corta", ""); !errors.Is(err, ErrPasswordDebil) {
		t.Errorf("clave débil aceptada: %v", err)
	}
	par, err := s.Login(ctx, "ana@uni.test", "clav3-Valida!", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CambiarPassword(ctx, uid, "clav3-Valida!", "Nueva-Clave-2026", ""); err != nil {
		t.Fatalf("CambiarPassword: %v", err)
	}
	if _, err := s.Login(ctx, "ana@uni.test", "clav3-Valida!", "", ""); !errors.Is(err, ErrCredenciales) {
		t.Errorf("login con clave vieja aceptado: %v", err)
	}
	if _, err := s.Refresh(ctx, par.RefreshToken, "", ""); !errors.Is(err, ErrRefreshInvalido) {
		t.Errorf("refresh previo no revocado tras cambio: %v", err)
	}
	if _, err := s.Login(ctx, "ana@uni.test", "Nueva-Clave-2026", "", ""); err != nil {
		t.Errorf("login con clave nueva: %v", err)
	}
}

func TestCambiarPasswordLimpiaForceChange(t *testing.T) {
	s := setupServicio(t)
	ctx := context.Background()
	if _, err := s.CrearMantenedorInicial(ctx, "boss@uni.test", "Boss#2026Clave", "B"); err != nil {
		t.Fatal(err)
	}
	uid := idDe(t, s, "boss@uni.test")
	par, err := s.Login(ctx, "boss@uni.test", "Boss#2026Clave", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !par.ForcePwdChange {
		t.Fatal("mantenedor debería exigir cambio")
	}
	if err := s.CambiarPassword(ctx, uid, "Boss#2026Clave", "Boss-Nueva#2026", ""); err != nil {
		t.Fatal(err)
	}
	par2, err := s.Login(ctx, "boss@uni.test", "Boss-Nueva#2026", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if par2.ForcePwdChange {
		t.Error("force_password_change no se limpió tras el cambio")
	}
}

func idDe(t *testing.T, s *Servicio, email string) string {
	t.Helper()
	u, err := buscarUsuarioPorEmail(context.Background(), s.Pool, email)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestSeguridadDevuelveEstado(t *testing.T) {
	s := setupServicio(t)
	crearUsuarioPrueba(t, s, "ana@uni.test", "clav3-Valida!")
	ctx := context.Background()
	uid := idDe(t, s, "ana@uni.test")

	sec, err := s.Seguridad(ctx, uid)
	if err != nil {
		t.Fatalf("Seguridad: %v", err)
	}
	if sec.Email != "ana@uni.test" || sec.TOTPActivado || sec.EsMantenedor {
		t.Errorf("estado inicial: %+v", sec)
	}

	secAct, err := s.Activar2FA(ctx, uid)
	if err != nil {
		t.Fatalf("Activar2FA: %v", err)
	}
	sec, _ = s.Seguridad(ctx, uid)
	if sec.TOTPActivado {
		t.Error("con 2FA pendiente no debe marcar activado")
	}

	codigo, _ := Codigo2FAParaTests(secAct.Secreto)
	if err := s.Confirmar2FA(ctx, uid, codigo); err != nil {
		t.Fatal(err)
	}
	sec, _ = s.Seguridad(ctx, uid)
	if !sec.TOTPActivado {
		t.Error("tras confirmar debe marcar activado")
	}
}
