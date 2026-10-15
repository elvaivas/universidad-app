// Centro de atención: campanita (panel + marcar leída), inicio jerárquico
// (anuncios de la universidad + importantes de mis secciones), anuncios
// generales (solo mantenedor) y flag importante de avisos.
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elvaivas/universidad-app/backend/internal/audit"
	"github.com/elvaivas/universidad-app/backend/internal/catalog"
	"github.com/elvaivas/universidad-app/backend/internal/feed"
	"github.com/elvaivas/universidad-app/backend/internal/notifs"
	"github.com/elvaivas/universidad-app/backend/internal/rbac"
)

// MontarAtencion registra las rutas del centro de atención (llamar desde main).
func (s *Server) MontarAtencion(cat *catalog.Servicio, feedSvc *feed.Servicio, pool *pgxpool.Pool) {
	s.router.Group(func(r chi.Router) {
		r.Use(rbac.RequireAuth(s.jwtSecreto))
		r.Get("/api/notificaciones", panelNotifs(pool))
		r.Post("/api/notificaciones/leer", marcarLeida(pool))
		r.Get("/api/inicio", inicio(cat, feedSvc, pool))
	})
	s.router.With(rbac.RequireAuth(s.jwtSecreto), rbac.RequireMantenedor).
		Post("/api/anuncios-generales", crearAnuncioGeneral(cat, pool))
	s.router.With(rbac.RequireAuth(s.jwtSecreto)).
		Post("/api/avisos/{aid}/importante", marcarImportante(feedSvc))
}

func panelNotifs(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := notifs.Listar(r.Context(), pool, actorDe(r))
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": p})
	}
}

type leerReq struct {
	ID    string `json:"id,omitempty"`   // marcar una notificación concreta
	Tipo  string `json:"tipo,omitempty"` // o por destino (auto-al-abrir)
	RefID string `json:"ref_id,omitempty"`
}

func marcarLeida(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req leerReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		var err error
		if req.ID != "" {
			err = notifs.MarcarLeida(r.Context(), pool, actorDe(r), req.ID)
		} else {
			err = notifs.MarcarLeidaPorRef(r.Context(), pool, actorDe(r), req.Tipo, req.RefID)
		}
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"leida": true}})
	}
}

func inicio(cat *catalog.Servicio, feedSvc *feed.Servicio, pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		generales, err := cat.ListarAnunciosGenerales(r.Context(), 10)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		importantes, err := feedSvc.ListarImportantesDe(r.Context(), actorDe(r), 10)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		panel, _ := notifs.Listar(r.Context(), pool, actorDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"universidad": generales,
			"importantes": importantes,
			"no_leidas":   panel.TotalNoLeidas,
		}})
	}
}

func crearAnuncioGeneral(cat *catalog.Servicio, pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Titulo string `json:"titulo"`
			Cuerpo string `json:"cuerpo"`
		}
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		a, err := cat.CrearAnuncioGeneral(r.Context(), actorDe(r), req.Titulo, req.Cuerpo)
		if err != nil {
			escribirError(w, http.StatusBadRequest, err.Error())
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "atencion.anuncio_general", "anuncio_general", a.ID, nil, a, ipDe(r))
		_ = notifs.CrearParaTodos(r.Context(), pool, actorDe(r), notifs.TipoAnuncioGeneral, a.ID, "Universidad: "+a.Titulo)
		escribirJSON(w, http.StatusCreated, map[string]any{"data": a})
	}
}

func marcarImportante(feedSvc *feed.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Importante bool `json:"importante"`
		}
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		aid := chi.URLParam(r, "aid")
		if err := feedSvc.MarcarImportante(r.Context(), aid, actorDe(r), req.Importante); err != nil {
			escribirError(w, http.StatusNotFound, "aviso no encontrado")
			return
		}
		audit.Registrar(r.Context(), feedSvc.Pool, actorDe(r), "atencion.aviso_importante", "aviso", aid,
			nil, map[string]any{"importante": req.Importante}, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"importante": req.Importante}})
	}
}
