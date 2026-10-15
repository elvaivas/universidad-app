package auth

import (
	"strings"
	"testing"
	"time"
)

const secretoPrueba = "0123456789abcdef0123456789abcdef"

func TestEmitirYValidar(t *testing.T) {
	token, err := EmitirToken("user-123", Claims{
		RolBase: "estudiante", Voceria: "carrera", EsMantenedor: false,
		SeccionIDs: []string{"a", "b"},
	}, secretoPrueba, 15*time.Minute)
	if err != nil {
		t.Fatalf("EmitirToken: %v", err)
	}
	c, err := ValidarToken(token, secretoPrueba)
	if err != nil {
		t.Fatalf("ValidarToken: %v", err)
	}
	if c.Subject != "user-123" {
		t.Errorf("sub ausente: %q", c.Subject)
	}
	if c.RolBase != "estudiante" || c.Voceria != "carrera" || c.EsMantenedor || len(c.SeccionIDs) != 2 {
		t.Errorf("claims no redondean: %+v", c)
	}
}

func TestTokenExpirado(t *testing.T) {
	token, _ := EmitirToken("user-123", Claims{RolBase: "profesor"}, secretoPrueba, -time.Minute)
	if _, err := ValidarToken(token, secretoPrueba); err == nil {
		t.Fatal("token expirado fue aceptado")
	}
}

func TestTokenConSecretoDistinto(t *testing.T) {
	token, _ := EmitirToken("user-123", Claims{RolBase: "profesor"}, secretoPrueba, time.Minute)
	if _, err := ValidarToken(token, "otro-secreto-0123456789abcdef012345"); err == nil {
		t.Fatal("token firmado con otro secreto fue aceptado")
	}
}

func TestTokenManipulado(t *testing.T) {
	token, _ := EmitirToken("user-123", Claims{RolBase: "estudiante", EsMantenedor: false}, secretoPrueba, time.Minute)
	parte := strings.Split(token, ".")
	parte[1] = "AAAA"
	if _, err := ValidarToken(strings.Join(parte, "."), secretoPrueba); err == nil {
		t.Fatal("token manipulado fue aceptado")
	}
}

func TestAlgNoneRechazado(t *testing.T) {
	if _, err := ValidarToken("eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJ4In0.", secretoPrueba); err == nil {
		t.Fatal("token alg=none aceptado")
	}
}
