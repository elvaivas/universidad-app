// Rutas del núcleo académico: materias, secciones e inscripciones.
// Lectura: autenticado. Creación de materias: mantenedor. Creación de
// secciones: profesor o mantenedor (el caller queda como profesor salvo que,
// siendo mantenedor, fije profesor_id en el body). Inscripción: el propio
// usuario (o un mantenedor a terceros). Lista de inscritos: profesor dueño
// de la sección o mantenedor.
package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/elvaivas/universidad-app/backend/internal/academic"
	"github.com/elvaivas/universidad-app/backend/internal/audit"
	"github.com/elvaivas/universidad-app/backend/internal/rbac"
)

func (s *Server) MontarAcademico(acad *academic.Servicio) {
	// lectura: autenticado
	s.router.Group(func(r chi.Router) {
		r.Use(rbac.RequireAuth(s.jwtSecreto))
		r.Get("/api/me/secciones", listarMisSecciones(acad))
		r.Get("/api/carreras/{cid}/materias", listarMaterias(acad))
		r.Get("/api/materias/{mid}/secciones", listarSecciones(acad))
		r.Get("/api/secciones/{sid}/inscritos", listarInscritos(acad))
	})
	// escritura por privilegio (el detalle fino se valida en cada handler)
	s.router.With(rbac.RequireAuth(s.jwtSecreto), rbac.RequireMantenedor).
		Post("/api/carreras/{cid}/materias", crearMateria(acad))
	s.router.With(rbac.RequireAuth(s.jwtSecreto)).
		Post("/api/materias/{mid}/secciones", crearSeccion(acad))
	s.router.With(rbac.RequireAuth(s.jwtSecreto)).
		Post("/api/secciones/{sid}/inscripcion", inscribir(acad))
	s.router.With(rbac.RequireAuth(s.jwtSecreto)).
		Delete("/api/secciones/{sid}/inscripcion", bajar(acad))
}

func paginaDe(r *http.Request) (int, int) { return paginacionDe(r) }

func responderErrorAcademic(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, academic.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "recurso no encontrado")
	case errors.Is(err, academic.ErrDatosInvalidos):
		escribirError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, academic.ErrCupoLleno),
		errors.Is(err, academic.ErrYaInscrito),
		errors.Is(err, academic.ErrNoInscrito):
		escribirError(w, http.StatusConflict, err.Error())
	default:
		escribirError(w, http.StatusInternalServerError, "error interno")
	}
}

// listarMisSecciones devuelve el resumen de secciones del usuario autenticado.
func listarMisSecciones(acad *academic.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secciones, err := acad.ListarSeccionesDeUsuario(r.Context(), uidDe(r))
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": secciones})
	}
}

func listarMaterias(acad *academic.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lim, off := paginaDe(r)
		p, err := acad.ListarMaterias(r.Context(), chi.URLParam(r, "cid"), lim, off)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": p})
	}
}

type materiaReq struct {
	Codigo string `json:"codigo"`
	Nombre string `json:"nombre"`
}

func crearMateria(acad *academic.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req materiaReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		m := &academic.Materia{CarreraID: chi.URLParam(r, "cid"), Codigo: req.Codigo, Nombre: req.Nombre}
		if err := acad.CrearMateria(r.Context(), m); err != nil {
			responderErrorAcademic(w, err)
			return
		}
		audit.Registrar(r.Context(), acad.Pool, actorDe(r), "academico.crear_materia",
			"materia", m.ID, nil, m, ipDe(r))
		escribirJSON(w, http.StatusCreated, map[string]any{"data": m})
	}
}

func listarSecciones(acad *academic.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lim, off := paginaDe(r)
		p, err := acad.ListarSecciones(r.Context(), chi.URLParam(r, "mid"), lim, off)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": p})
	}
}

type seccionReq struct {
	PeriodoID  string `json:"periodo_id"`
	Codigo     string `json:"codigo"`
	Cupo       int    `json:"cupo"`
	ProfesorID string `json:"profesor_id,omitempty"` // solo mantenedor puede fijarlo
}

func crearSeccion(acad *academic.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := rbac.ClaimsDe(r)
		if c == nil || (c.RolBase != "profesor" && !c.EsMantenedor) {
			escribirError(w, http.StatusForbidden, "solo profesores o mantenedores crean secciones")
			return
		}
		var req seccionReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		profesorID := c.Subject
		if req.ProfesorID != "" {
			if !c.EsMantenedor {
				escribirError(w, http.StatusForbidden, "solo un mantenedor puede asignar otro profesor")
				return
			}
			profesorID = req.ProfesorID
		}
		sec := &academic.Seccion{
			MateriaID: chi.URLParam(r, "mid"), PeriodoID: req.PeriodoID,
			ProfesorID: profesorID, Codigo: req.Codigo, Cupo: req.Cupo,
		}
		if err := acad.CrearSeccion(r.Context(), sec); err != nil {
			responderErrorAcademic(w, err)
			return
		}
		audit.Registrar(r.Context(), acad.Pool, actorDe(r), "academico.crear_seccion",
			"seccion", sec.ID, nil, sec, ipDe(r))
		escribirJSON(w, http.StatusCreated, map[string]any{"data": sec})
	}
}

func inscribir(acad *academic.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := rbac.ClaimsDe(r)
		estudianteID := c.Subject
		var req struct {
			EstudianteID string `json:"estudiante_id,omitempty"`
		}
		if r.Body != nil {
			_ = decodificarJSON(w, r, &req) // body opcional
		}
		if req.EstudianteID != "" {
			if !c.EsMantenedor {
				escribirError(w, http.StatusForbidden, "solo un mantenedor inscribe a terceros")
				return
			}
			estudianteID = req.EstudianteID
		}
		ins, err := acad.Inscribir(r.Context(), chi.URLParam(r, "sid"), estudianteID)
		if err != nil {
			responderErrorAcademic(w, err)
			return
		}
		audit.Registrar(r.Context(), acad.Pool, actorDe(r), "academico.inscripcion",
			"inscripcion", ins.ID, nil, ins, ipDe(r))
		escribirJSON(w, http.StatusCreated, map[string]any{"data": ins})
	}
}

func bajar(acad *academic.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := rbac.ClaimsDe(r)
		if err := acad.Bajar(r.Context(), chi.URLParam(r, "sid"), c.Subject); err != nil {
			responderErrorAcademic(w, err)
			return
		}
		audit.Registrar(r.Context(), acad.Pool, actorDe(r), "academico.baja",
			"seccion", chi.URLParam(r, "sid"), nil, nil, ipDe(r))
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"baja": true}})
	}
}

func listarInscritos(acad *academic.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := rbac.ClaimsDe(r)
		sid := chi.URLParam(r, "sid")
		sec, err := acad.ObtenerSeccion(r.Context(), sid)
		if err != nil {
			responderErrorAcademic(w, err)
			return
		}
		if sec.ProfesorID != c.Subject && !c.EsMantenedor {
			escribirError(w, http.StatusForbidden, "solo el profesor de la sección o un mantenedor ven la lista")
			return
		}
		lim, off := paginaDe(r)
		p, err := acad.ListarInscritos(r.Context(), sid, lim, off)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": p})
	}
}
