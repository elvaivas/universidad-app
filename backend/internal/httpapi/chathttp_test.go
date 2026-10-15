package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/chat"
)

// servidorConChatModulo: servidor de pruebas con el módulo chat montado.
func servidorConChatModulo(t *testing.T) (*httptest.Server, *auth.Servicio) {
	t.Helper()
	srvViejo, svc := servidorPrueba(t)
	srvViejo.Close()
	httpSrv := NuevoServer(svc, svc.JWTSecreto, 50, 5)
	httpSrv.MontarChat(&chat.Servicio{Pool: svc.Pool}, chat.NuevoHub())
	srv := httptest.NewServer(httpSrv.Handler())
	t.Cleanup(srv.Close)
	return srv, svc
}

// tokenDeID emite un JWT de cortesía para un usuario conocido por id.
func tokenDeID(t *testing.T, svc *auth.Servicio, userID string) string {
	t.Helper()
	tok, err := auth.EmitirToken(userID, auth.Claims{RolBase: "estudiante"}, svc.JWTSecreto, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// wsChatSetup: servidor real con sección + miembro + no-miembro.
func wsChatSetup(t *testing.T) (srv *httptest.Server, svc *auth.Servicio, seccionID, tokMiembro, tokAfuera string) {
	t.Helper()
	srvBase, svcBase := servidorConChatModulo(t)
	svc = svcBase

	ctx := context.Background()
	var uid, cid, pid, mid, profID, estID, afueraID string
	_ = svc.Pool.QueryRow(ctx, `INSERT INTO universidades (nombre) VALUES ('U') RETURNING id::text`).Scan(&uid)
	_ = svc.Pool.QueryRow(ctx, `INSERT INTO carreras (universidad_id, nombre) VALUES ($1::uuid,'Ing') RETURNING id::text`, uid).Scan(&cid)
	_ = svc.Pool.QueryRow(ctx, `INSERT INTO periodos (carrera_id, nombre, fecha_inicio, fecha_fin) VALUES ($1::uuid,'2026-1','2026-01-12','2026-05-30') RETURNING id::text`, cid).Scan(&pid)
	_ = svc.Pool.QueryRow(ctx, `INSERT INTO materias (carrera_id, codigo, nombre) VALUES ($1::uuid,'PS-101','Prog') RETURNING id::text`, cid).Scan(&mid)
	profID = crearUsuarioRol(t, svc, "prof@uni.test", "profesor")
	estID = crearUsuarioRol(t, svc, "est@uni.test", "estudiante")
	afueraID = crearUsuarioRol(t, svc, "afuera@uni.test", "estudiante")
	_ = svc.Pool.QueryRow(ctx,
		`INSERT INTO secciones (materia_id, periodo_id, profesor_id, codigo, cupo) VALUES ($1::uuid,$2::uuid,$3::uuid,'1',30) RETURNING id::text`,
		mid, pid, profID).Scan(&seccionID)
	_, _ = svc.Pool.Exec(ctx, `INSERT INTO inscripciones (seccion_id, estudiante_id) VALUES ($1::uuid,$2::uuid)`, seccionID, estID)

	tokMiembro = tokenDeID(t, svc, estID)
	tokAfuera = tokenDeID(t, svc, afueraID)
	return srvBase, svc, seccionID, tokMiembro, tokAfuera
}

func TestWSRechazaNoMiembroYAceptaMiembro(t *testing.T) {
	srv, _, sid, tokMiembro, tokAfuera := wsChatSetup(t)
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	// no miembro: el upgrade se rechaza
	_, _, err := websocket.Dial(context.Background(), wsURL+"/ws/secciones/"+sid+"?token="+tokAfuera, nil)
	if err == nil {
		t.Fatal("no-miembro conectó al WS")
	}

	// miembro: conecta, envía y recibe su eco
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL+"/ws/secciones/"+sid+"?token="+tokMiembro, nil)
	if err != nil {
		t.Fatalf("miembro no pudo conectar: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "fin")

	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"tipo":"mensaje","cuerpo":"hola sala"}`)); err != nil {
		t.Fatal(err)
	}
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("sin eco de mensaje: %v", err)
	}
	var ev struct {
		Tipo string          `json:"tipo"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &ev); err != nil || ev.Tipo != "mensaje" {
		t.Fatalf("evento inesperado: %s (%v)", data, err)
	}
	var m chat.Mensaje
	if err := json.Unmarshal(ev.Data, &m); err != nil || m.Cuerpo != "hola sala" {
		t.Fatalf("mensaje malformado: %s (%v)", ev.Data, err)
	}
}

func TestWSDifundeAOtrosClientes(t *testing.T) {
	srv, _, sid, tokA, _ := wsChatSetup(t)
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// mismo usuario, dos conexiones (simula dos pestañas)
	connA, _, err := websocket.Dial(ctx, wsURL+"/ws/secciones/"+sid+"?token="+tokA, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connA.Close(websocket.StatusNormalClosure, "a")
	connB, _, err := websocket.Dial(ctx, wsURL+"/ws/secciones/"+sid+"?token="+tokA, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connB.Close(websocket.StatusNormalClosure, "b")

	if err := connA.Write(ctx, websocket.MessageText, []byte(`{"tipo":"mensaje","cuerpo":"a otro cliente"}`)); err != nil {
		t.Fatal(err)
	}

	_, dataA, err := connA.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, dataB, err := connB.Read(ctx)
	if err != nil {
		t.Fatalf("B no recibió el broadcast: %v", err)
	}
	var evA, evB struct {
		Tipo string          `json:"tipo"`
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(dataA, &evA)
	_ = json.Unmarshal(dataB, &evB)
	if evA.Tipo != "mensaje" || evB.Tipo != "mensaje" {
		t.Errorf("tipos: %s / %s", evA.Tipo, evB.Tipo)
	}
	if string(evA.Data) != string(evB.Data) {
		t.Errorf("broadcast distinto al eco: %s vs %s", evA.Data, evB.Data)
	}
}
