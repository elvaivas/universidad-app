package auth

import (
	"strings"
	"testing"
)

func TestHashYVerify(t *testing.T) {
	hash, err := HashPassword("clav3-Segura!2026")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Errorf("formato PHC inesperado: %q", hash[:40])
	}
	ok, err := VerificarPassword(hash, "clav3-Segura!2026")
	if err != nil || !ok {
		t.Errorf("verificación de clave correcta falló: ok=%v err=%v", ok, err)
	}
	ok, err = VerificarPassword(hash, "otra-clave")
	if err != nil || ok {
		t.Errorf("clave incorrecta aceptada: ok=%v err=%v", ok, err)
	}
}

func TestHashesDistintosMismaClave(t *testing.T) {
	h1, _ := HashPassword("misma")
	h2, _ := HashPassword("misma")
	if h1 == h2 {
		t.Error("dos hashes de la misma clave deben diferir (sal aleatoria)")
	}
}

func TestVerificarRechazaFormatoBasura(t *testing.T) {
	for _, basura := range []string{"", "abc", "$argon2id$v=19$m=1$", "$md5$v=1$m=1,t=1,p=1$AAAA$BBBB"} {
		if _, err := VerificarPassword(basura, "x"); err == nil {
			t.Errorf("formato inválido aceptado: %q", basura)
		}
	}
}

// TestVectorPHCExterno fija un vector generado por el propio código para
// detectar cambios accidentales de parámetros (regresión silenciosa).
func TestVectorPHCExterno(t *testing.T) {
	ok, err := VerificarPassword(
		"$argon2id$v=19$m=65536,t=3,p=4$A3+2uecnxCMZB0s62q7BCQ$GsqGCl6WEYfdMkDQcP6sEKTL3zzBbjS1+dWVnoYokCw",
		"vector-de-prueba")
	if err != nil || !ok {
		t.Errorf("vector PHC conocido no verifica: ok=%v err=%v", ok, err)
	}
}
