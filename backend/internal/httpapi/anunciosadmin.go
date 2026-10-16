// Administración de anuncios generales: editar y eliminar (solo mantenedor).
// Archivo separado para no tocar notifshttp.go en las entregas.
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/elvaivas/universidad-app/backend/internal/audit"
	"github.com/elvaivas/universidad-app/backend/internal/catalog"
	"github.com/elvaivas/universidad-app/backend/internal/rbac"
)

// MontarAnunciosAdmin registra PATCH y DELETE sobre /api/anuncios-generales/{id}.
func (s *Server) MontarAnunciosAdmin(cat *catalog.Servicio) {
	s.router.With(rbac.RequireAuth(s.jwtSecreto), rbac.RequireMantenedor).
		Patch("/api/anuncios-generales/{id}", editarAnuncioGeneral(cat))
	s.router.With(rbac.RequireAuth(s.jwtSecreto), rbac.RequireMantenedor).
		Delete("/api/anuncios-generales/{id}", eliminarAnuncioGeneral(cat))
}

type anuncioGeneralReq struct {
	Titulo string `json:"titulo"`
	Cuerpo string `json:"cuerpo"`
}

func editarAnuncioGeneral(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req anuncioGeneralReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		a, err := cat.ActualizarAnuncioGeneral(r.Context(), chi.URLParam(r, "id"), req.Titulo, req.Cuerpo)
		if err != nil {
			escribirError(w, http.StatusNotFound, "anuncio no encontrado")
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "atencion.anuncio_general_editar", "anuncio_general", a.ID, nil, a, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": a})
	}
}

func eliminarAnuncioGeneral(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := cat.EliminarAnuncioGeneral(r.Context(), id); err != nil {
			escribirError(w, http.StatusNotFound, "anuncio no encontrado")
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "atencion.anuncio_general_eliminar", "anuncio_general", id, nil, nil, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"eliminado": true}})
	}
}
