// Rutas del feed de sección: avisos, votaciones y comentarios.
// Membresía de sección requerida para leer/comentar/votar; publicar exige
// profesor de la sección, vocero de clase del alcance o mantenedor.
package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/elvaivas/universidad-app/backend/internal/audit"
	"github.com/elvaivas/universidad-app/backend/internal/feed"
	"github.com/elvaivas/universidad-app/backend/internal/rbac"
)

func (s *Server) MontarFeed(f *feed.Servicio) {
	s.router.Group(func(r chi.Router) {
		r.Use(rbac.RequireAuth(s.jwtSecreto))
		r.Get("/api/secciones/{sid}/avisos", listarAvisos(f))
		r.Post("/api/secciones/{sid}/avisos", publicarAviso(f))
		r.Post("/api/secciones/{sid}/votaciones", crearVotacionHTTP(f))
		r.Get("/api/avisos/{aid}/comentarios", listarComentarios(f))
		r.Post("/api/avisos/{aid}/comentarios", comentar(f))
		r.Post("/api/votaciones/{vid}/votar", votar(f))
		r.Get("/api/votaciones/{vid}/resultados", resultados(f))
		r.Post("/api/votaciones/{vid}/cerrar", cerrarVotacion(f))
	})
}

// miembroO403 resuelve membresía; escribe el 403 y devuelve false si no pasa.
func miembroO403(w http.ResponseWriter, r *http.Request, f *feed.Servicio, seccionID string) bool {
	c := rbac.ClaimsDe(r)
	ok, err := f.EsMiembro(r.Context(), c.Subject, seccionID)
	if err != nil || !ok {
		escribirError(w, http.StatusForbidden, "no eres miembro de esta sección")
		return false
	}
	return true
}

func publicarO403(w http.ResponseWriter, r *http.Request, f *feed.Servicio, seccionID string) bool {
	c := rbac.ClaimsDe(r)
	ok, err := f.PuedePublicar(r.Context(), c.Subject, seccionID)
	if err != nil || !ok {
		escribirError(w, http.StatusForbidden, "solo el profesor, un vocero de clase o un mantenedor publican aquí")
		return false
	}
	return true
}

func responderErrorFeed(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, feed.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "recurso no encontrado")
	case errors.Is(err, feed.ErrDatosInvalidos), errors.Is(err, feed.ErrOpcionInvalida):
		escribirError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, feed.ErrYaVoto), errors.Is(err, feed.ErrVotacionCerrada):
		escribirError(w, http.StatusConflict, err.Error())
	default:
		escribirError(w, http.StatusInternalServerError, "error interno")
	}
}

func listarAvisos(f *feed.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := chi.URLParam(r, "sid")
		if !miembroO403(w, r, f, sid) {
			return
		}
		lim, off := paginacionDe(r)
		p, err := f.ListarAvisos(r.Context(), sid, lim, off)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": p})
	}
}

type avisoReq struct {
	Titulo string `json:"titulo"`
	Cuerpo string `json:"cuerpo"`
}

func publicarAviso(f *feed.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := chi.URLParam(r, "sid")
		if !publicarO403(w, r, f, sid) {
			return
		}
		var req avisoReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		a, err := f.PublicarAviso(r.Context(), sid, actorDe(r), req.Titulo, req.Cuerpo)
		if err != nil {
			responderErrorFeed(w, err)
			return
		}
		audit.Registrar(r.Context(), f.Pool, actorDe(r), "feed.publicar_aviso",
			"aviso", a.ID, nil, a, ipDe(r))
		escribirJSON(w, http.StatusCreated, map[string]any{"data": a})
	}
}

type votacionReq struct {
	Titulo   string   `json:"titulo"`
	Pregunta string   `json:"pregunta"`
	Opciones []string `json:"opciones"`
	Multiple bool     `json:"multiple"`
}

func crearVotacionHTTP(f *feed.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := chi.URLParam(r, "sid")
		if !publicarO403(w, r, f, sid) {
			return
		}
		var req votacionReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		v, err := f.CrearVotacion(r.Context(), sid, actorDe(r), req.Titulo, req.Pregunta, req.Opciones, req.Multiple)
		if err != nil {
			responderErrorFeed(w, err)
			return
		}
		audit.Registrar(r.Context(), f.Pool, actorDe(r), "feed.crear_votacion",
			"votacion", v.ID, nil, v, ipDe(r))
		escribirJSON(w, http.StatusCreated, map[string]any{"data": v})
	}
}

func listarComentarios(f *feed.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aid := chi.URLParam(r, "aid")
		sid, err := f.SeccionDeAviso(r.Context(), aid)
		if err != nil {
			responderErrorFeed(w, err)
			return
		}
		if !miembroO403(w, r, f, sid) {
			return
		}
		lim, off := paginacionDe(r)
		p, err := f.ListarComentarios(r.Context(), aid, lim, off)
		if err != nil {
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": p})
	}
}

type comentarioReq struct {
	Cuerpo string `json:"cuerpo"`
}

func comentar(f *feed.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aid := chi.URLParam(r, "aid")
		sid, err := f.SeccionDeAviso(r.Context(), aid)
		if err != nil {
			responderErrorFeed(w, err)
			return
		}
		if !miembroO403(w, r, f, sid) {
			return
		}
		var req comentarioReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		c, err := f.Comentar(r.Context(), aid, actorDe(r), req.Cuerpo)
		if err != nil {
			responderErrorFeed(w, err)
			return
		}
		escribirJSON(w, http.StatusCreated, map[string]any{"data": c})
	}
}

type votarReq struct {
	Opciones []int `json:"opciones"`
}

// seccionDeVotacion resuelve aviso→sección de una votación.
func seccionDeVotacion(r *http.Request, f *feed.Servicio, vid string) (string, error) {
	var avisoID string
	if err := f.Pool.QueryRow(r.Context(),
		`SELECT aviso_id::text FROM votaciones WHERE id=$1::uuid`, vid).Scan(&avisoID); err != nil {
		return "", feed.ErrNoEncontrado
	}
	return f.SeccionDeAviso(r.Context(), avisoID)
}

func votar(f *feed.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vid := chi.URLParam(r, "vid")
		sid, err := seccionDeVotacion(r, f, vid)
		if err != nil {
			responderErrorFeed(w, err)
			return
		}
		if !miembroO403(w, r, f, sid) {
			return
		}
		var req votarReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		if err := f.Votar(r.Context(), vid, actorDe(r), req.Opciones); err != nil {
			responderErrorFeed(w, err)
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"votado": true}})
	}
}

func resultados(f *feed.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vid := chi.URLParam(r, "vid")
		sid, err := seccionDeVotacion(r, f, vid)
		if err != nil {
			responderErrorFeed(w, err)
			return
		}
		if !miembroO403(w, r, f, sid) {
			return
		}
		res, err := f.Resultados(r.Context(), vid)
		if err != nil {
			responderErrorFeed(w, err)
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": res})
	}
}

// cerrarVotacion: quien puede publicar en la sección puede cerrar su votación.
func cerrarVotacion(f *feed.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vid := chi.URLParam(r, "vid")
		sid, err := seccionDeVotacion(r, f, vid)
		if err != nil {
			responderErrorFeed(w, err)
			return
		}
		if !publicarO403(w, r, f, sid) {
			return
		}
		if err := f.CerrarVotacion(r.Context(), vid); err != nil {
			responderErrorFeed(w, err)
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"cerrada": true}})
	}
}
