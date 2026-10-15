// Segundo factor TOTP (RFC 6238): generación de secreto, QR (otpauth URL +
// PNG base64) y validación con tolerancia de desfase de ±1 paso de 30 s.
package auth

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const emisor2FA = "Universidad App"

// Secreto2FA empaqueta lo que se muestra al usuario al activar 2FA.
type Secreto2FA struct {
	Secreto     string `json:"secreto"`       // base32 (se guarda en users.totp_secret)
	OtpauthURL  string `json:"otpauth_url"`   // para apps que aceptan URL directa
	QRPNGBase64 string `json:"qr_png_base64"` // QR 200x200 en base64 (<img src="data:image/png;base64,...">)
}

// GenerarSecreto2FA crea un secreto TOTP nuevo para el email dado.
func GenerarSecreto2FA(email string) (*Secreto2FA, error) {
	clave, err := totp.Generate(totp.GenerateOpts{
		Issuer:      emisor2FA,
		AccountName: email,
		Period:      30,
		SecretSize:  20,
	})
	if err != nil {
		return nil, fmt.Errorf("generar TOTP: %w", err)
	}
	img, err := clave.Image(200, 200)
	if err != nil {
		return nil, fmt.Errorf("generar QR: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("codificar QR: %w", err)
	}
	return &Secreto2FA{
		Secreto:     clave.Secret(),
		OtpauthURL:  clave.URL(),
		QRPNGBase64: base64.StdEncoding.EncodeToString(buf.Bytes()),
	}, nil
}

// ValidarCodigo2FA valida un código de 6 dígitos contra el secreto (±1 paso).
func ValidarCodigo2FA(secreto, codigo string) bool {
	ok, err := totp.ValidateCustom(codigo, secreto, time.Now().UTC(),
		totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	return err == nil && ok
}

// Codigo2FAParaTests genera el código vigente para un secreto (SOLO tests).
func Codigo2FAParaTests(secreto string) (string, error) {
	return totp.GenerateCode(secreto, time.Now().UTC())
}
