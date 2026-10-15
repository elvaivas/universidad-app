// Servicio de autenticación: login con Argon2id (con segundo factor TOTP),
// emisión de par de tokens (JWT 15 min + refresh rotativo persistido como
// SHA-256), seed del primer mantenedor, cambio de contraseña y auditoría
// de eventos sensibles. Testeable contra PostgreSQL real (internal/testdb).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elvaivas/universidad-app/backend/internal/audit"
)

var (
	ErrCredenciales      = errors.New("credenciales inválidas") // genérico: no filtra usuarios existentes
	ErrUsuarioInactivo   = errors.New("usuario inactivo")
	ErrRequiere2FA       = errors.New("requiere segundo factor")
	ErrRefreshInvalido   = errors.New("refresh token inválido o revocado")
	Err2FASinSecreto     = errors.New("2FA no iniciado: activa y confirma primero")
	ErrCodigo2FAInvalido = errors.New("código 2FA inválido")
	ErrPreTokenInvalido  = errors.New("pre-token 2FA inválido o usado")
	ErrPasswordDebil     = errors.New("la nueva contraseña debe tener al menos 10 caracteres")
)

type Usuario struct {
	ID             string
	Email          string
	Nombre         string
	RolBase        string
	EsMantenedor   bool
	PasswordHash   string
	TOTPSecret     *string
	TOTPActivado   bool
	Activo         bool
	ForcePwdChange bool
}

type ParTokens struct {
	AccessToken    string `json:"access_token"`
	RefreshToken   string `json:"refresh_token"`
	TokenType      string `json:"token_type"`
	ExpiresIn      int64  `json:"expires_in"` // segundos
	Requiere2FA    bool   `json:"requiere_2fa"`
	ForcePwdChange bool   `json:"force_password_change"`
}

// --- repositorio (consultas parametrizadas pgx; sin concatenación) ---

func buscarUsuarioPorEmail(ctx context.Context, pool *pgxpool.Pool, email string) (*Usuario, error) {
	var u Usuario
	err := pool.QueryRow(ctx,
		`SELECT id::text, lower(email), nombre, rol_base, es_mantenedor,
		        password_hash, totp_secret, totp_activado, activo, force_password_change
		 FROM users WHERE lower(email) = lower($1)`, email).Scan(
		&u.ID, &u.Email, &u.Nombre, &u.RolBase, &u.EsMantenedor,
		&u.PasswordHash, &u.TOTPSecret, &u.TOTPActivado, &u.Activo, &u.ForcePwdChange)
	if err != nil {
		return nil, err // pgx.ErrNoRows lo traduce el caller a genérico
	}
	return &u, nil
}

func buscarUsuarioPorID(ctx context.Context, pool *pgxpool.Pool, id string) (*Usuario, error) {
	var u Usuario
	err := pool.QueryRow(ctx,
		`SELECT id::text, lower(email), nombre, rol_base, es_mantenedor,
		        password_hash, totp_secret, totp_activado, activo, force_password_change
		 FROM users WHERE id = $1::uuid`, id).Scan(
		&u.ID, &u.Email, &u.Nombre, &u.RolBase, &u.EsMantenedor,
		&u.PasswordHash, &u.TOTPSecret, &u.TOTPActivado, &u.Activo, &u.ForcePwdChange)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func voceriaMayorNivel(ctx context.Context, pool *pgxpool.Pool, userID string) (string, error) {
	var nivel string
	err := pool.QueryRow(ctx,
		`SELECT nivel FROM vocerias WHERE user_id = $1::uuid
		 ORDER BY CASE nivel WHEN 'universidad' THEN 3 WHEN 'carrera' THEN 2 ELSE 1 END
		 LIMIT 1`, userID).Scan(&nivel)
	if err != nil {
		return "", nil // sin vocería no es error
	}
	return nivel, nil
}

func guardarRefreshToken(ctx context.Context, pool *pgxpool.Pool, userID, hash string, expira time.Time, ip, ua string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at, ip, user_agent)
		 VALUES ($1::uuid, $2, $3, NULLIF($4,'')::inet, NULLIF($5,''))`,
		userID, hash, expira, ip, ua)
	return err
}

func buscarRefreshActivo(ctx context.Context, pool *pgxpool.Pool, hash string) (userID string, expira time.Time, revocado *time.Time, err error) {
	err = pool.QueryRow(ctx,
		`SELECT user_id::text, expires_at, revoked_at FROM refresh_tokens WHERE token_hash = $1`,
		hash).Scan(&userID, &expira, &revocado)
	return
}

func revocarRefresh(ctx context.Context, pool *pgxpool.Pool, hash string) error {
	_, err := pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, hash)
	return err
}

// --- servicio ---

type Servicio struct {
	Pool       *pgxpool.Pool
	JWTSecreto string
	AccessTTL  time.Duration
	RefreshTTL time.Duration

	muJTI     sync.Mutex
	usadosJTI map[string]time.Time
}

// Login verifica credenciales. Si el usuario tiene 2FA activado, devuelve un
// par con Requiere2FA=true y AccessToken = pre-token firmado (2 min, un solo
// uso) que se canjea en Login2FA. Si no, emite el par de tokens directo.
func (s *Servicio) Login(ctx context.Context, email, password, ip, ua string) (*ParTokens, error) {
	u, err := buscarUsuarioPorEmail(ctx, s.Pool, email)
	if err != nil {
		// gastamos el mismo costo Argon2 para no filtrar por timing (S2)
		_, _ = VerificarPassword("$argon2id$v=19$m=65536,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", password)
		audit.Registrar(ctx, s.Pool, "", "auth.login_fallido", "user", "",
			nil, map[string]any{"email": email}, ip)
		return nil, ErrCredenciales
	}
	ok, err := VerificarPassword(u.PasswordHash, password)
	if err != nil || !ok {
		audit.Registrar(ctx, s.Pool, u.ID, "auth.login_fallido", "user", u.ID,
			nil, map[string]any{"email": email}, ip)
		return nil, ErrCredenciales
	}
	if !u.Activo {
		return nil, ErrUsuarioInactivo
	}
	if u.TOTPActivado && u.TOTPSecret != nil && *u.TOTPSecret != "" {
		pre, err := EmitirToken(u.ID, Claims{RolBase: u.RolBase, Pre2FA: true},
			s.JWTSecreto, 2*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("firmar pre-token 2FA: %w", err)
		}
		return &ParTokens{Requiere2FA: true, AccessToken: pre,
			TokenType: "Pre2FA", ExpiresIn: 120}, nil
	}
	audit.Registrar(ctx, s.Pool, u.ID, "auth.login", "user", u.ID,
		nil, map[string]any{"email": u.Email}, ip)
	return s.emitirPar(ctx, u, ip, ua)
}

// Refresh rota el token: revoca el presentado y emite par nuevo. Reutilizar un
// token ya rotado (replay) es rechazado — señal de robo: revoca toda la cadena.
func (s *Servicio) Refresh(ctx context.Context, refreshToken, ip, ua string) (*ParTokens, error) {
	suma := sha256.Sum256([]byte(refreshToken))
	hash := base64.RawURLEncoding.EncodeToString(suma[:])

	userID, expira, revocado, err := buscarRefreshActivo(ctx, s.Pool, hash)
	if err != nil {
		return nil, ErrRefreshInvalido
	}
	ahora := time.Now()
	if revocado != nil || ahora.After(expira) {
		if revocado != nil {
			// replay detectado: cadena comprometida, se revoca TODO lo del usuario
			_, _ = s.Pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now()
				WHERE user_id = $1::uuid AND revoked_at IS NULL`, userID)
			audit.Registrar(ctx, s.Pool, userID, "auth.refresh_replay", "user", userID, nil, nil, ip)
		}
		return nil, ErrRefreshInvalido
	}

	u, err := buscarUsuarioPorID(ctx, s.Pool, userID)
	if err != nil {
		return nil, ErrRefreshInvalido
	}
	if err := revocarRefresh(ctx, s.Pool, hash); err != nil {
		return nil, fmt.Errorf("revocar refresh: %w", err)
	}
	audit.Registrar(ctx, s.Pool, u.ID, "auth.refresh", "user", u.ID, nil, nil, ip)
	return s.emitirPar(ctx, u, ip, ua)
}

func (s *Servicio) emitirPar(ctx context.Context, u *Usuario, ip, ua string) (*ParTokens, error) {
	voceria, _ := voceriaMayorNivel(ctx, s.Pool, u.ID)

	access, err := EmitirToken(u.ID, Claims{
		RolBase: u.RolBase, Voceria: voceria, EsMantenedor: u.EsMantenedor,
	}, s.JWTSecreto, s.AccessTTL)
	if err != nil {
		return nil, fmt.Errorf("firmar access token: %w", err)
	}

	refreshBytes := make([]byte, 48) // 384 bits de entropía
	if _, err := rand.Read(refreshBytes); err != nil {
		return nil, fmt.Errorf("generar refresh: %w", err)
	}
	refresh := base64.RawURLEncoding.EncodeToString(refreshBytes)
	suma := sha256.Sum256([]byte(refresh))
	if err := guardarRefreshToken(ctx, s.Pool, u.ID,
		base64.RawURLEncoding.EncodeToString(suma[:]), time.Now().Add(s.RefreshTTL), ip, ua); err != nil {
		return nil, fmt.Errorf("persistir refresh: %w", err)
	}

	return &ParTokens{
		AccessToken: access, RefreshToken: refresh, TokenType: "Bearer",
		ExpiresIn:      int64(s.AccessTTL.Seconds()),
		ForcePwdChange: u.ForcePwdChange,
	}, nil
}

// CrearMantenedorInicial crea el primer usuario administrador (comando
// seed-admin). Error claro si el email ya existe — nunca sobrescribe.
func (s *Servicio) CrearMantenedorInicial(ctx context.Context, email, password, nombre string) (*Usuario, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	var id string
	err = s.Pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, nombre, rol_base, es_mantenedor, force_password_change)
		 VALUES ($1, $2, $3, 'profesor', true, true)
		 ON CONFLICT (lower(email)) DO NOTHING
		 RETURNING id::text`, email, hash, nombre).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("el email %s ya está registrado", email)
	}
	if err != nil {
		return nil, fmt.Errorf("insertar mantenedor: %w", err)
	}
	audit.Registrar(ctx, s.Pool, id, "auth.seed_admin", "user", id,
		nil, map[string]any{"email": email}, "")
	return &Usuario{ID: id, Email: email, Nombre: nombre, RolBase: "profesor",
		EsMantenedor: true, ForcePwdChange: true}, nil
}

// --- 2FA TOTP ---

// Activar2FA genera un secreto y lo guarda PENDIENTE (no exige código aún).
// El usuario lo confirma escaneando el QR e ingresando el primer código.
func (s *Servicio) Activar2FA(ctx context.Context, userID string) (*Secreto2FA, error) {
	u, err := buscarUsuarioPorID(ctx, s.Pool, userID)
	if err != nil {
		return nil, ErrCredenciales
	}
	sec, err := GenerarSecreto2FA(u.Email)
	if err != nil {
		return nil, err
	}
	_, err = s.Pool.Exec(ctx,
		`UPDATE users SET totp_secret = $2, totp_activado = false WHERE id = $1::uuid`,
		userID, sec.Secreto)
	if err != nil {
		return nil, fmt.Errorf("guardar secreto 2FA: %w", err)
	}
	audit.Registrar(ctx, s.Pool, userID, "auth.2fa_activar", "user", userID,
		nil, map[string]any{"estado": "pendiente"}, "")
	return sec, nil
}

// Confirmar2FA valida el primer código y activa el segundo factor.
func (s *Servicio) Confirmar2FA(ctx context.Context, userID, codigo string) error {
	u, err := buscarUsuarioPorID(ctx, s.Pool, userID)
	if err != nil || u.TOTPSecret == nil || *u.TOTPSecret == "" {
		return Err2FASinSecreto
	}
	if !ValidarCodigo2FA(*u.TOTPSecret, codigo) {
		return ErrCodigo2FAInvalido
	}
	_, err = s.Pool.Exec(ctx,
		`UPDATE users SET totp_activado = true WHERE id = $1::uuid`, userID)
	if err != nil {
		return fmt.Errorf("activar 2FA: %w", err)
	}
	audit.Registrar(ctx, s.Pool, userID, "auth.2fa_confirmar", "user", userID,
		nil, map[string]any{"estado": "activo"}, "")
	return nil
}

// Desactivar2FA exige un código válido para apagar el segundo factor.
func (s *Servicio) Desactivar2FA(ctx context.Context, userID, codigo string) error {
	u, err := buscarUsuarioPorID(ctx, s.Pool, userID)
	if err != nil || u.TOTPSecret == nil {
		return Err2FASinSecreto
	}
	if !ValidarCodigo2FA(*u.TOTPSecret, codigo) {
		return ErrCodigo2FAInvalido
	}
	_, err = s.Pool.Exec(ctx,
		`UPDATE users SET totp_secret = NULL, totp_activado = false WHERE id = $1::uuid`, userID)
	if err != nil {
		return fmt.Errorf("desactivar 2FA: %w", err)
	}
	audit.Registrar(ctx, s.Pool, userID, "auth.2fa_desactivar", "user", userID, nil, nil, "")
	return nil
}

// Login2FA completa el login del segundo paso: valida el pre-token (un solo
// uso, 2 min) y el código TOTP, y emite el par de tokens real.
func (s *Servicio) Login2FA(ctx context.Context, preToken, codigo, ip, ua string) (*ParTokens, error) {
	c, err := ValidarToken(preToken, s.JWTSecreto)
	if err != nil || !c.Pre2FA {
		return nil, ErrPreTokenInvalido
	}
	if s.jtiUsado(c.ID) {
		return nil, ErrPreTokenInvalido // reuso: posible robo
	}

	u, err := buscarUsuarioPorID(ctx, s.Pool, c.Subject)
	if err != nil || u.TOTPSecret == nil || !u.TOTPActivado {
		return nil, ErrPreTokenInvalido
	}
	if !ValidarCodigo2FA(*u.TOTPSecret, codigo) {
		return nil, ErrCodigo2FAInvalido
	}
	s.marcarJTIUsado(c.ID)

	audit.Registrar(ctx, s.Pool, u.ID, "auth.login_2fa", "user", u.ID,
		nil, map[string]any{"email": u.Email}, ip)
	return s.emitirPar(ctx, u, ip, ua)
}

// --- cambio de contraseña ---

// CambiarPassword exige la clave actual, valida la nueva, marca
// force_password_change=false y REVOCA todas las sesiones (refresh tokens):
// un cambio de contraseña debe cerrar todo lo abierto.
func (s *Servicio) CambiarPassword(ctx context.Context, userID, actual, nueva, ip string) error {
	u, err := buscarUsuarioPorID(ctx, s.Pool, userID)
	if err != nil {
		return ErrCredenciales
	}
	ok, err := VerificarPassword(u.PasswordHash, actual)
	if err != nil || !ok {
		return ErrCredenciales
	}
	if len(nueva) < 10 {
		return ErrPasswordDebil
	}
	hash, err := HashPassword(nueva)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`UPDATE users SET password_hash = $2, force_password_change = false WHERE id = $1::uuid`,
		userID, hash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL`,
		userID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	audit.Registrar(ctx, s.Pool, userID, "auth.cambio_password", "user", userID, nil, nil, ip)
	return nil
}

// --- pre-tokens de un solo uso (memoria; 1 réplica según plano) ---

func (s *Servicio) jtiUsado(jti string) bool {
	s.muJTI.Lock()
	defer s.muJTI.Unlock()
	_, usado := s.usadosJTI[jti]
	return usado
}

func (s *Servicio) marcarJTIUsado(jti string) {
	s.muJTI.Lock()
	defer s.muJTI.Unlock()
	if s.usadosJTI == nil {
		s.usadosJTI = map[string]time.Time{}
	}
	s.usadosJTI[jti] = time.Now()
}
