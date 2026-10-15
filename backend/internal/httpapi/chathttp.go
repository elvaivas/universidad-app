// Rutas del chat: historial paginado, borrado con moderación y el socket
// WebSocket /ws/secciones/{sid}. El navegador no puede enviar headers en el
// upgrade WS, así que el JWT viaja como ?token= (uso aceptado: WSS en prod,
// sesión corta de 15 min y validación de membresía al conectar).
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"github.com/elvaivas/universidad-app/backend/internal/audit"
	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/chat"
	"github.com/elvaivas/universidad-app/backend/internal/rbac"
)

func (s *Server) MontarChat(svc *chat.Servicio, hub *chat.Hub) {
	// historial y moderación (HTTP)
	s.router.With(rbac.RequireAuth(s.jwtSecreto)).
		Get("/api/secciones/{sid}/mensajes", listarMensajes(svc))
	s.router.With(rbac.RequireAuth(s.jwtSecreto)).
		Delete("/api/mensajes/{id}", borrarMensaje(svc, hub))

	// websocket: el token viaja por query (?token=)
	s.router.Get("/ws/secciones/{sid}", wsChat(svc, hub, s.jwtSecreto))
}

func miembroChatO403(w http.ResponseWriter, r *http.Request, svc *chat.Servicio, seccionID string) bool {
	c := rbac.ClaimsDe(r)
	ok, err := svc.EsMiembro(r.Context(), c.Subject, seccionID)
	if err != nil || !ok {
		escribirError(w, http.StatusForbidden, "no eres miembro de esta sección")
		return false
	}
	return true
}

func listarMensajes(svc *chat.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := chi.URLParam(r, "sid")
		if !miembroChatO403(w, r, svc, sid) {
			return
		}
		lim, off := paginacionDe(r)
		datos, total, err := svc.Listar(r.Context(), sid, lim, off)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"datos": datos, "total": total, "limit": lim, "offset": off,
		}})
	}
}

func borrarMensaje(svc *chat.Servicio, hub *chat.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		sid, err := svc.Borrar(r.Context(), actorDe(r), id)
		switch {
		case errors.Is(err, chat.ErrSinPermiso):
			escribirError(w, http.StatusForbidden, "solo el autor, el profesor de la sección o un mantenedor pueden borrar")
			return
		case errors.Is(err, chat.ErrNoEncontrado):
			escribirError(w, http.StatusNotFound, "mensaje no encontrado")
			return
		case err != nil:
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		audit.Registrar(r.Context(), svc.Pool, actorDe(r), "chat.borrar_mensaje", "mensaje", id, nil, nil, ipDe(r))
		b, _ := json.Marshal(map[string]string{"id": id})
		hub.Difundir(sid, chat.Evento{Tipo: "borrado", Data: b})
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"borrado": true}})
	}
}

// wsChat maneja el upgrade: valida token (query) + membresía y delega al hub.
func wsChat(svc *chat.Servicio, hub *chat.Hub, jwtSecreto string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := chi.URLParam(r, "sid")
		token := r.URL.Query().Get("token")
		claims, err := auth.ValidarToken(strings.TrimSpace(token), jwtSecreto)
		if err != nil {
			http.Error(w, `{"error":"token inválido o expirado"}`, http.StatusUnauthorized)
			return
		}
		miembro, err := svc.EsMiembro(r.Context(), claims.Subject, sid)
		if err != nil || !miembro {
			http.Error(w, `{"error":"no eres miembro de esta sección"}`, http.StatusForbidden)
			return
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"}, // dev: cualquier origen (el token es la guarda)
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "adiós")

		autorID := claims.Subject
		hub.Atender(r.Context(), sid, conn, func(cuerpo string) error {
			m, err := svc.Guardar(r.Context(), sid, autorID, cuerpo)
			if err != nil {
				return err
			}
			nombre := claims.RolBase
			_ = svc.Pool.QueryRow(r.Context(), `SELECT nombre FROM users WHERE id = $1::uuid`, autorID).Scan(&nombre)
			m.AutorNom = nombre
			b, _ := json.Marshal(m)
			hub.Difundir(sid, chat.Evento{Tipo: "mensaje", Data: b})
			return nil
		})
	}
}
