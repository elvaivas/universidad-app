// Hub: registro de conexiones WebSocket agrupadas por sección, con broadcast.
// 1 réplica (plano): memoria es suficiente; Redis pub/sub queda para cuando
// haya >1 réplica del servidor.
package chat

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Evento es el sobre JSON que viaja por el socket.
type Evento struct {
	Tipo string          `json:"tipo"` // mensaje | borrado | error | estado
	Data json.RawMessage `json:"data,omitempty"`
}

type cliente struct {
	conn   *websocket.Conn
	enviar chan Evento
	// anti-spam: ventana deslizante simple
	mu      sync.Mutex
	ventana []time.Time
}

func (c *cliente) permite() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	ahora := time.Now()
	corte := ahora.Add(-10 * time.Second)
	viva := c.ventana[:0]
	for _, t := range c.ventana {
		if t.After(corte) {
			viva = append(viva, t)
		}
	}
	c.ventana = viva
	if len(c.ventana) >= 5 { // máx 5 mensajes cada 10 s por conexión
		return false
	}
	c.ventana = append(c.ventana, ahora)
	return true
}

type Hub struct {
	mu    sync.Mutex
	salas map[string]map[*cliente]bool
}

func NuevoHub() *Hub {
	return &Hub{salas: map[string]map[*cliente]bool{}}
}

func (h *Hub) unir(seccionID string, c *cliente) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.salas[seccionID] == nil {
		h.salas[seccionID] = map[*cliente]bool{}
	}
	h.salas[seccionID][c] = true
}

func (h *Hub) salir(seccionID string, c *cliente) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.salas[seccionID] != nil {
		delete(h.salas[seccionID], c)
		if len(h.salas[seccionID]) == 0 {
			delete(h.salas, seccionID)
		}
	}
}

// Difundir envía el evento a todos los clientes de la sala. Los envíos son
// asíncronos (canales con buffer); un cliente lento no frena a la sala.
func (h *Hub) Difundir(seccionID string, ev Evento) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.salas[seccionID] {
		select {
		case c.enviar <- ev:
		default: // canal lleno: se descarta para este cliente
		}
	}
}

// Atender registra la conexión y bombea eventos hasta que el cliente se va.
// onMensaje se invoca por cada mensaje entrante válido (ya con anti-spam).
func (h *Hub) Atender(ctx context.Context, seccionID string, conn *websocket.Conn, onMensaje func(cuerpo string) error) {
	c := &cliente{conn: conn, enviar: make(chan Evento, 16)}
	h.unir(seccionID, c)
	defer h.salir(seccionID, c)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// bombeo de salida
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range c.enviar {
			b, _ := json.Marshal(ev)
			if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
				return
			}
		}
	}()

	// lectura (solo se aceptan mensajes de tipo "mensaje")
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		var entrada struct {
			Tipo   string `json:"tipo"`
			Cuerpo string `json:"cuerpo"`
		}
		if json.Unmarshal(data, &entrada) != nil || entrada.Tipo != "mensaje" {
			continue
		}
		if !c.permite() {
			b, _ := json.Marshal(Evento{Tipo: "error", Data: json.RawMessage(`{"error":"demasiados mensajes; espera unos segundos"}`)})
			_ = conn.Write(ctx, websocket.MessageText, b)
			continue
		}
		if err := onMensaje(entrada.Cuerpo); err != nil {
			slog.Debug("chat: mensaje rechazado", "error", err)
		}
	}
	cancel()
	<-done
}
