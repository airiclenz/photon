package tune

import (
	"embed"
	"encoding/json"
	"errors"
	"image"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/airiclenz/photon/internal/render"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// pageFiles is the page the server hands the browser: page/index.html and the assets beside it.
//
//go:embed page
var pageFiles embed.FS

// The page's files inside pageFiles, and the one served at the root.
const (
	pageDir   = "page"
	pageIndex = "index.html"
)

// The server's routes: the page, its assets, and the JSON API over the session.
const (
	routeIndex   = "GET /{$}"
	routeAssets  = "GET /assets/"
	assetsPrefix = "/assets/"
	routeState   = "GET /api/state"
	routeFrame   = "GET /api/frame"
	routeEdit    = "POST /api/edit"
	routeUndo    = "POST /api/undo"
	routeRedo    = "POST /api/redo"
	routeRevert  = "POST /api/revert"
	routeReload  = "POST /api/reload"
	routeSave    = "POST /api/save"
	routePick    = "POST /api/pick"
	routePath    = "GET /api/path"
)

// The headers and media types the API reads and writes.
const (
	headerContentType  = "Content-Type"
	headerCacheControl = "Cache-Control"
	headerNoSniff      = "X-Content-Type-Options"
	mediaJSON          = "application/json"
	mediaPNG           = "image/png"
	noStore            = "no-store"
	noSniff            = "nosniff"
)

// frameTimeParam is the frame and path routes' query parameter: the output time in whole
// milliseconds.
const frameTimeParam = "t"

// pathCameraParam is the path route's query parameter: the via pan's index in the camera list.
const pathCameraParam = "camera"

// localhostName is the other host name, beside the listen address's own IP, a request may name.
const localhostName = "localhost"

// maxBodyBytes bounds a POST body; an edit or a pick is a few dozen bytes.
const maxBodyBytes = 1 << 20

// server serves one session's page and API. hosts are the Host header values it answers, so a
// page on another origin that resolves its own name to 127.0.0.1 is refused; page is the
// embedded page directory it serves.
type server struct {
	session *Session
	hosts   map[string]bool
	page    fs.FS
}

// apiError is the body of every API failure.
type apiError struct {
	Error string `json:"error"`
}

// pickRequest is a pick's body: the output time in milliseconds and the rectangle's two
// corners, in output-frame pixels.
type pickRequest struct {
	TimeMS int64      `json:"t"`
	From   pixelPoint `json:"from"`
	To     pixelPoint `json:"to"`
}

// pixelPoint is a point on the output frame, in pixels.
type pixelPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// pathResponse is a path's answer: the via pan's points in order, in output-frame pixels.
type pathResponse struct {
	Points []pixelPoint `json:"points"`
}

// pickResponse is a pick's answer: the target, the cell box it resolves to, that box in
// output-frame pixels, and the snippets keyed by place.
type pickResponse struct {
	Target   storyboard.Target `json:"target"`
	Box      take.Box          `json:"box"`
	Rect     pixelRect         `json:"rect"`
	Snippets map[string]string `json:"snippets"`
}

// pickTargetOf is target as a pick proposes it on the wire: an nth of first becomes the zero nth,
// which the wire leaves out.
func pickTargetOf(target storyboard.Target) storyboard.Target {
	if target.Nth == storyboard.NthFirst {
		target.Nth = 0
	}
	return target
}

// pixelRect is a rectangle on the output frame, in pixels.
type pixelRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// newHandler returns the handler serving session to a browser that reached it at addr, the
// listener's host:port: the embedded page at / and /assets/, and the JSON API under /api/.
//
// A request whose Host is neither addr nor localhost on addr's port is refused with 403, and a
// POST whose body is not application/json with 415, so another site's page can reach neither
// the session nor the storyboard on disk. Every POST answers with the session's state after it
// but a pick, which answers with its proposal. A refused edit, undo or redo, a frame, pick or
// path the session cannot give, and a save refused while an error stands answer 422; a save
// refused while the storyboard changed on disk answers 409. Every failure's body is
// {"error": "…"}.
func newHandler(session *Session, addr string) http.Handler {
	serving := &server{session: session, hosts: map[string]bool{addr: true}, page: pageFS()}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		serving.hosts[net.JoinHostPort(localhostName, port)] = true
	}

	mux := http.NewServeMux()
	mux.HandleFunc(routeIndex, serving.handleIndex)
	mux.Handle(routeAssets, http.StripPrefix(assetsPrefix, http.FileServerFS(serving.page)))
	mux.HandleFunc(routeState, serving.handleState)
	mux.HandleFunc(routeFrame, serving.handleFrame)
	mux.HandleFunc(routeEdit, serving.handleEdit)
	mux.HandleFunc(routeUndo, serving.stateAfter(session.Undo))
	mux.HandleFunc(routeRedo, serving.stateAfter(session.Redo))
	mux.HandleFunc(routeRevert, serving.stateAfter(func() error {
		session.Revert()
		return nil
	}))
	mux.HandleFunc(routeReload, serving.stateAfter(session.Reload))
	mux.HandleFunc(routeSave, serving.stateAfter(session.Save))
	mux.HandleFunc(routePick, serving.handlePick)
	mux.HandleFunc(routePath, serving.handlePath)
	return serving.guard(mux)
}

// pageFS is the page directory of pageFiles, rooted at the page itself.
func pageFS() fs.FS {
	page, err := fs.Sub(pageFiles, pageDir)
	if err != nil {
		// fs.Sub fails only on an invalid path, and pageDir is a constant valid one.
		panic(err)
	}
	return page
}

// guard refuses a request naming a foreign Host with 403 and a POST that is not JSON with 415
// before next sees it, and bounds every body at [maxBodyBytes].
func (s *server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hosts[r.Host] {
			writeError(w, http.StatusForbidden, "this server answers only at the address it printed")
			return
		}
		if r.Method == http.MethodPost && !isJSON(r) {
			writeError(w, http.StatusUnsupportedMediaType, "a POST body must be "+mediaJSON)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// isJSON says whether r's body is declared application/json, parameters such as charset aside.
func isJSON(r *http.Request) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get(headerContentType))
	return err == nil && media == mediaJSON
}

// handleIndex serves the page.
func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, s.page, pageIndex)
}

// handleState takes in what changed on disk and answers the session's state.
func (s *server) handleState(w http.ResponseWriter, _ *http.Request) {
	if err := s.session.Refresh(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.session.State())
}

// handleFrame answers the PNG of the frame at the output time ?t=, in whole milliseconds.
func (s *server) handleFrame(w http.ResponseWriter, r *http.Request) {
	out, ok := frameTimeOf(w, r)
	if !ok {
		return
	}
	frame, err := s.session.Frame(out)
	if err != nil {
		writeError(w, statusOf(err), err.Error())
		return
	}
	w.Header().Set(headerContentType, mediaPNG)
	w.Header().Set(headerCacheControl, noStore)
	w.Header().Set(headerNoSniff, noSniff)
	w.WriteHeader(http.StatusOK)
	//nolint:errcheck,gosec // a failed write means the page went away; nothing is left to tell
	w.Write(frame)
}

// handlePath answers the points of the via pan ?camera= on the frame at the output time ?t=, in
// whole milliseconds, or 422 when that camera move is not a via pan.
func (s *server) handlePath(w http.ResponseWriter, r *http.Request) {
	out, ok := frameTimeOf(w, r)
	if !ok {
		return
	}
	index, err := strconv.Atoi(r.URL.Query().Get(pathCameraParam))
	if err != nil {
		writeError(w, http.StatusBadRequest, "the camera index must be a whole number")
		return
	}
	points, err := s.session.Path(out, index)
	if err != nil {
		writeError(w, statusOf(err), err.Error())
		return
	}
	response := pathResponse{Points: make([]pixelPoint, 0, len(points))}
	for _, point := range points {
		response.Points = append(response.Points, pixelPoint{X: point.X, Y: point.Y})
	}
	writeJSON(w, http.StatusOK, response)
}

// frameTimeOf is r's output time ?t=, in whole milliseconds; when it is not one, it answers 400
// and reports false.
func frameTimeOf(w http.ResponseWriter, r *http.Request) (time.Duration, bool) {
	ms, err := strconv.ParseInt(r.URL.Query().Get(frameTimeParam), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the frame time t must be whole milliseconds")
		return 0, false
	}
	return time.Duration(ms) * time.Millisecond, true
}

// handleEdit applies the [Edit] in the body and answers the state, or 422 when it is refused.
func (s *server) handleEdit(w http.ResponseWriter, r *http.Request) {
	var edit Edit
	if !decodeBody(w, r, &edit) {
		return
	}
	if err := s.session.Apply(edit); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.session.State())
}

// stateAfter is the handler that runs change on the session and answers the state after it.
func (s *server) stateAfter(change func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if err := change(); err != nil {
			writeError(w, statusOf(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s.session.State())
	}
}

// handlePick answers the [Proposal] for the pick in the body, or 422 when there is none.
func (s *server) handlePick(w http.ResponseWriter, r *http.Request) {
	var pick pickRequest
	if !decodeBody(w, r, &pick) {
		return
	}
	proposal, err := s.session.Pick(
		time.Duration(pick.TimeMS)*time.Millisecond,
		image.Pt(pick.From.X, pick.From.Y),
		image.Pt(pick.To.X, pick.To.Y),
	)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, responseOf(proposal))
}

// responseOf is proposal as a pick answers it.
func responseOf(proposal Proposal) pickResponse {
	rect := proposal.Rect
	return pickResponse{
		Target:   pickTargetOf(proposal.Target),
		Box:      proposal.Box,
		Rect:     pixelRect{X: rect.Min.X, Y: rect.Min.Y, W: rect.Dx(), H: rect.Dy()},
		Snippets: proposal.Snippets,
	}
}

// decodeBody decodes r's JSON body into into, refusing unknown fields and trailing data. On
// failure it answers 400, or 413 past [maxBodyBytes], and reports false.
func decodeBody(w http.ResponseWriter, r *http.Request, into any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(into)
	if err == nil && decoder.More() {
		err = errors.New("trailing data after the JSON value")
	}
	if err == nil {
		return true
	}
	status := http.StatusBadRequest
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		status = http.StatusRequestEntityTooLarge
	}
	writeError(w, status, "read the request body: "+err.Error())
	return false
}

// statusOf is the status a session error answers with: 409 while stale, 422 for what the
// session refuses, 500 for anything else.
func statusOf(err error) int {
	switch {
	case errors.Is(err, ErrStale):
		return http.StatusConflict
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrNoUndo), errors.Is(err, ErrNoRedo),
		errors.Is(err, render.ErrOutsideClip), errors.Is(err, ErrNoProposal),
		errors.Is(err, ErrNoPath):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// writeError answers status with message as an [apiError].
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiError{Error: message})
}

// writeJSON answers status with value encoded as JSON, never cached.
func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		encoded = []byte(`{"error":"encode the response"}`)
	}
	w.Header().Set(headerContentType, mediaJSON)
	w.Header().Set(headerCacheControl, noStore)
	w.Header().Set(headerNoSniff, noSniff)
	w.WriteHeader(status)
	//nolint:errcheck,gosec // a failed write means the page went away; nothing is left to tell
	w.Write(encoded)
}
