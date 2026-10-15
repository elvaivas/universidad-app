// Rutas de mensajes directos: solicitudes (crear/listar/aceptar/rechazar),
// conversaciones (listar/historial/borrar) y el socket /ws/dm/{cid}, que
// reutiliza el mismo hub del chat con salas "dm:{cid}" — el motor no se toca.
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
	"github.com/elvaivas/universidad-app/backend/internal/dm"
	"github.com/elvaivas/universidad-app/backend/internal/rbac"
)

func (s *Server) MontarDM(svc *dm.Servicio, hub *chat.Hub) {
	s.router.Group(func(r chi.Router) {
		r.Use(rbac.RequireAuth(s.jwtSecreto))

		r.Post("/api/dm/solicitudes", crearSolicitud(svc))
		r.Get("/api/dm/solicitudes", solicitudesRecibidas(svc))
		r.Get("/api/dm/solicitudes/enviadas", solicitudesEnviadas(svc))
		r.Post("/api/dm/solicitudes/{id}/aceptar", aceptarSolicitud(svc))
		r.Post("/api/dm/solicitudes/{id}/rechazar", rechazarSolicitud(svc))

		r.Get("/api/dm/conversaciones", listarConversaciones(svc))
		r.Get("/api/dm/conversaciones/{cid}/mensajes", listarMensajesDM(svc))
		r.Delete("/api/dm/mensajes/{id}", borrarMensajeDM(svc, hub))
	})

	s.router.Get("/ws/dm/{cid}", wsDM(svc, hub, s.jwtSecreto))
}

func responderErrorDM(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dm.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "usuario o solicitud no encontrado")
	case errors.Is(err, dm.ErrSinPermiso):
		escribirError(w, http.StatusForbidden, "sin permiso")
	case errors.Is(err, dm.ErrAutoDM), errors.Is(err, dm.ErrCuerpoInvalido):
		escribirError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, dm.ErrSinSeccionCompartida):
		escribirError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, dm.ErrSolicitudExiste), errors.Is(err, dm.ErrYaConversan), errors.Is(err, dm.ErrYaRespondida):
		escribirError(w, http.StatusConflict, err.Error())
	default:
		escribirError(w, http.StatusInternalServerError, "error interno")
	}
}

type solicitudReq struct {
	Destinatario string `json:"destinatario"` // email
	Mensaje      string `json:"mensaje"`      // intención
}

func crearSolicitud(svc *dm.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req solicitudReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		var destID string
		err := svc.Pool.QueryRow(r.Context(),
			`SELECT id::text FROM users WHERE lower(email) = lower($1) AND activo`,
			strings.TrimSpace(req.Destinatario)).Scan(&destID)
		if err != nil {
			escribirError(w, http.StatusNotFound, "usuario no encontrado")
			return
		}
		so, err := svc.CrearSolicitud(r.Context(), actorDe(r), destID, req.Mensaje)
		if err != nil {
			responderErrorDM(w, err)
			return
		}
		audit.Registrar(r.Context(), svc.Pool, actorDe(r), "dm.solicitud_enviar", "solicitud_dm", so.ID,
			nil, map[string]any{"destinatario": destID}, ipDe(r))
		escribirJSON(w, http.StatusCreated, map[string]any{"data": so})
	}
}

func solicitudesRecibidas(svc *dm.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sos, err := svc.SolicitudesRecibidas(r.Context(), actorDe(r))
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": sos})
	}
}

func solicitudesEnviadas(svc *dm.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sos, err := svc.SolicitudesEnviadas(r.Context(), actorDe(r))
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": sos})
	}
}

func aceptarSolicitud(svc *dm.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		convID, err := svc.Aceptar(r.Context(), chi.URLParam(r, "id"), actorDe(r))
		if err != nil {
			responderErrorDM(w, err)
			return
		}
		audit.Registrar(r.Context(), svc.Pool, actorDe(r), "dm.solicitud_aceptar", "solicitud_dm",
			chi.URLParam(r, "id"), nil, map[string]any{"conversacion": convID}, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"conversacion_id": convID}})
	}
}

func rechazarSolicitud(svc *dm.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Rechazar(r.Context(), chi.URLParam(r, "id"), actorDe(r)); err != nil {
			responderErrorDM(w, err)
			return
		}
		audit.Registrar(r.Context(), svc.Pool, actorDe(r), "dm.solicitud_rechazar", "solicitud_dm",
			chi.URLParam(r, "id"), nil, nil, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"rechazada": true}})
	}
}

func listarConversaciones(svc *dm.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		convos, err := svc.ListarConversaciones(r.Context(), actorDe(r))
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": convos})
	}
}

func listarMensajesDM(svc *dm.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid := chi.URLParam(r, "cid")
		lim, off := paginacionDe(r)
		datos, total, err := svc.ListarMensajes(r.Context(), cid, actorDe(r), lim, off)
		if err != nil {
			responderErrorDM(w, err)
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"datos": datos, "total": total, "limit": lim, "offset": off,
		}})
	}
}

func borrarMensajeDM(svc *dm.Servicio, hub *chat.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		cid, err := svc.BorrarMensaje(r.Context(), actorDe(r), id)
		if err != nil {
			responderErrorDM(w, err)
			return
		}
		audit.Registrar(r.Context(), svc.Pool, actorDe(r), "dm.borrar_mensaje", "mensaje_dm", id, nil, nil, ipDe(r))
		b, _ := json.Marshal(map[string]string{"id": id})
		hub.Difundir("dm:"+cid, chat.Evento{Tipo: "borrado", Data: b})
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"borrado": true}})
	}
}

// wsDM: sala "dm:{cid}" en el mismo hub; solo participantes de la conversación.
func wsDM(svc *dm.Servicio, hub *chat.Hub, jwtSecreto string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid := chi.URLParam(r, "cid")
		claims, err := auth.ValidarToken(strings.TrimSpace(r.URL.Query().Get("token")), jwtSecreto)
		if err != nil {
			http.Error(w, `{"error":"token inválido o expirado"}`, http.StatusUnauthorized)
			return
		}
		ok, err := svc.EsParticipante(r.Context(), claims.Subject, cid)
		if err != nil || !ok {
			http.Error(w, `{"error":"no participas de esta conversación"}`, http.StatusForbidden)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "adiós")

		sala := "dm:" + cid
		hub.Atender(r.Context(), sala, conn, func(cuerpo, _ string) error {
			m, err := svc.GuardarMensaje(r.Context(), cid, claims.Subject, cuerpo)
			if err != nil {
				return err
			}
			nombre := claims.RolBase
			_ = svc.Pool.QueryRow(r.Context(), `SELECT nombre FROM users WHERE id = $1::uuid`, claims.Subject).Scan(&nombre)
			m.AutorNom = nombre
			b, _ := json.Marshal(m)
			hub.Difundir(sala, chat.Evento{Tipo: "mensaje", Data: b})
			return nil
		})
	}
}
