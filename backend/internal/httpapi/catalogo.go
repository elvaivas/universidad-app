// Handlers del catálogo académico. Lectura: cualquier usuario autenticado.
// Escritura (POST/PUT/DELETE): solo mantenedores. Baja = desactivar (activo=false).
package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/elvaivas/universidad-app/backend/internal/audit"
	"github.com/elvaivas/universidad-app/backend/internal/catalog"
	"github.com/elvaivas/universidad-app/backend/internal/rbac"
)

// MontarCatalogo agrega las rutas del catálogo al router. Se llama desde
// main.go después de NuevoServer (NuevoServer conserva el secreto JWT).
func (s *Server) MontarCatalogo(cat *catalog.Servicio) {
	// lectura: autenticado
	s.router.Group(func(r chi.Router) {
		r.Use(rbac.RequireAuth(s.jwtSecreto))
		r.Get("/api/universidades", listarUniversidades(cat))
		r.Get("/api/universidades/{id}", obtenerUniversidad(cat))
		r.Get("/api/universidades/{uid}/carreras", listarCarreras(cat))
		r.Get("/api/carreras/{cid}/periodos", listarPeriodos(cat))
	})
	// escritura: mantenedor
	s.router.Group(func(r chi.Router) {
		r.Use(rbac.RequireAuth(s.jwtSecreto), rbac.RequireMantenedor)
		r.Post("/api/universidades", crearUniversidad(cat))
		r.Put("/api/universidades/{id}", actualizarUniversidad(cat))
		r.Delete("/api/universidades/{id}", desactivarUniversidad(cat))
		r.Post("/api/universidades/{uid}/carreras", crearCarrera(cat))
		r.Put("/api/carreras/{id}", actualizarCarrera(cat))
		r.Delete("/api/carreras/{id}", desactivarCarrera(cat))
		r.Post("/api/carreras/{cid}/periodos", crearPeriodo(cat))
		r.Put("/api/periodos/{id}", actualizarPeriodo(cat))
	})
}

// --- helpers de request/response ---

func paginacionDe(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	return
}

func responderErrorCatalogo(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalog.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "recurso no encontrado")
	case errors.Is(err, catalog.ErrDatosInvalidos):
		escribirError(w, http.StatusBadRequest, err.Error())
	default:
		escribirError(w, http.StatusInternalServerError, "error interno")
	}
}

func actorDe(r *http.Request) string {
	if c := rbac.ClaimsDe(r); c != nil {
		return c.Subject
	}
	return ""
}

// --- universidades ---

func listarUniversidades(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lim, off := paginacionDe(r)
		p, err := cat.ListarUniversidades(r.Context(), lim, off)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": p})
	}
}

func obtenerUniversidad(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := cat.ObtenerUniversidad(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			responderErrorCatalogo(w, err)
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": u})
	}
}

func crearUniversidad(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u catalog.Universidad
		if err := decodificarJSON(w, r, &u); err != nil {
			return
		}
		if err := cat.CrearUniversidad(r.Context(), &u); err != nil {
			responderErrorCatalogo(w, err)
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "catalogo.crear_universidad",
			"universidad", u.ID, nil, u, ipDe(r))
		escribirJSON(w, http.StatusCreated, map[string]any{"data": u})
	}
}

func actualizarUniversidad(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u catalog.Universidad
		if err := decodificarJSON(w, r, &u); err != nil {
			return
		}
		id := chi.URLParam(r, "id")
		if err := cat.ActualizarUniversidad(r.Context(), id, &u); err != nil {
			responderErrorCatalogo(w, err)
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "catalogo.actualizar_universidad",
			"universidad", id, nil, u, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": u})
	}
}

func desactivarUniversidad(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := cat.DesactivarUniversidad(r.Context(), id); err != nil {
			responderErrorCatalogo(w, err)
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "catalogo.desactivar_universidad",
			"universidad", id, nil, map[string]any{"activo": false}, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": id, "activo": "false"}})
	}
}

// --- carreras ---

func listarCarreras(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lim, off := paginacionDe(r)
		p, err := cat.ListarCarreras(r.Context(), chi.URLParam(r, "uid"), lim, off)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": p})
	}
}

func crearCarrera(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var c catalog.Carrera
		if err := decodificarJSON(w, r, &c); err != nil {
			return
		}
		c.UniversidadID = chi.URLParam(r, "uid")
		if err := cat.CrearCarrera(r.Context(), &c); err != nil {
			responderErrorCatalogo(w, err)
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "catalogo.crear_carrera",
			"carrera", c.ID, nil, c, ipDe(r))
		escribirJSON(w, http.StatusCreated, map[string]any{"data": c})
	}
}

func actualizarCarrera(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var c catalog.Carrera
		if err := decodificarJSON(w, r, &c); err != nil {
			return
		}
		id := chi.URLParam(r, "id")
		if err := cat.ActualizarCarrera(r.Context(), id, &c); err != nil {
			responderErrorCatalogo(w, err)
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "catalogo.actualizar_carrera",
			"carrera", id, nil, c, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": c})
	}
}

func desactivarCarrera(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := cat.DesactivarCarrera(r.Context(), id); err != nil {
			responderErrorCatalogo(w, err)
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "catalogo.desactivar_carrera",
			"carrera", id, nil, map[string]any{"activo": false}, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": id, "activo": "false"}})
	}
}

// --- periodos ---

func listarPeriodos(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lim, off := paginacionDe(r)
		p, err := cat.ListarPeriodos(r.Context(), chi.URLParam(r, "cid"), lim, off)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": p})
	}
}

func crearPeriodo(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var pe catalog.Periodo
		if err := decodificarJSON(w, r, &pe); err != nil {
			return
		}
		pe.CarreraID = chi.URLParam(r, "cid")
		if err := cat.CrearPeriodo(r.Context(), &pe); err != nil {
			responderErrorCatalogo(w, err)
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "catalogo.crear_periodo",
			"periodo", pe.ID, nil, pe, ipDe(r))
		escribirJSON(w, http.StatusCreated, map[string]any{"data": pe})
	}
}

func actualizarPeriodo(cat *catalog.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var pe catalog.Periodo
		if err := decodificarJSON(w, r, &pe); err != nil {
			return
		}
		id := chi.URLParam(r, "id")
		if err := cat.ActualizarPeriodo(r.Context(), id, &pe); err != nil {
			responderErrorCatalogo(w, err)
			return
		}
		audit.Registrar(r.Context(), cat.Pool, actorDe(r), "catalogo.actualizar_periodo",
			"periodo", id, nil, pe, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": pe})
	}
}
