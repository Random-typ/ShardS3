package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"shards3/services/dashboard/internal/modules/stats"
)

const (
	Prefix = "/api"
	Health = "/health"
	// storage
	StorageUsed = "/storage"
	// backends
	Backends      = "/backends"
	BackendByType = "/backends/:type"
	BackendTypes  = "/backend-types"
	// buckets
	Buckets      = "/buckets"
	BucketByName = "/buckets/:name"
	// objects
	Objects       = "/objects"
	ObjectsByPath = "/objects/:path"
	// chunks
	Chunks    = "/chunks"
	ChunkByID = "/chunks/:id"
	// shards
	Shards    = "/shards"
	ShardByID = "/shards/:id"
)

type Handler struct {
	stats stats.Service
}

type response struct {
	Message string `json:"message"`
}

func New(statsService stats.Service) http.Handler {
	h := &Handler{stats: statsService}
	mux := http.NewServeMux()

	mux.HandleFunc("GET "+Prefix+Health, h.health)
	mux.HandleFunc("GET "+Prefix+StorageUsed, h.storageUsed)
	mux.HandleFunc("GET "+Prefix+Backends, h.backends)
	mux.HandleFunc("GET "+Prefix+BackendTypes, h.backendTypes)
	mux.HandleFunc("GET "+Prefix+Buckets, h.notImplemented)
	mux.HandleFunc("GET "+Prefix+Objects, h.notImplemented)
	mux.HandleFunc("GET "+Prefix+Chunks, h.notImplemented)
	mux.HandleFunc("GET "+Prefix+Shards, h.notImplemented)

	// Item routes still rely on DB-facing functions that are not available yet.
	mux.HandleFunc("GET "+Prefix+"/backends/", h.backendByType)
	mux.HandleFunc("GET "+Prefix+"/buckets/", h.notImplemented)
	mux.HandleFunc("GET "+Prefix+"/objects/", h.notImplemented)
	mux.HandleFunc("GET "+Prefix+"/chunks/", h.notImplemented)
	mux.HandleFunc("GET "+Prefix+"/shards/", h.notImplemented)

	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{Message: "ok"})
}

func (h *Handler) storageUsed(w http.ResponseWriter, r *http.Request) {
	if h.stats == nil {
		writeJSON(w, http.StatusServiceUnavailable, response{Message: "stats service not configured"})
		return
	}

	if err := h.stats.GetStorageUsed(); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Message: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, response{Message: "storage usage collected"})
}

func (h *Handler) backends(w http.ResponseWriter, r *http.Request) {
	if h.stats == nil {
		writeJSON(w, http.StatusServiceUnavailable, response{Message: "stats service not configured"})
		return
	}

	if err := h.stats.GetSummary(); err != nil {
		if errors.Is(err, errors.ErrUnsupported) {
			writeJSON(w, http.StatusNotImplemented, response{Message: "summary is not implemented yet"})
			return
		}

		writeJSON(w, http.StatusInternalServerError, response{Message: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, response{Message: "backend summary collected"})
}

func (h *Handler) backendTypes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, response{Message: "backend type listing is not implemented yet"})
}

func (h *Handler) backendByType(w http.ResponseWriter, r *http.Request) {
	backendType := strings.TrimPrefix(r.URL.Path, Prefix+"/backends/")
	if backendType == "" || strings.Contains(backendType, "/") {
		writeJSON(w, http.StatusBadRequest, response{Message: "invalid backend type"})
		return
	}

	writeJSON(w, http.StatusNotImplemented, response{Message: "backend detail is not implemented yet"})
}

func (h *Handler) notImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, response{Message: "route is not implemented yet"})
}

func writeJSON(w http.ResponseWriter, status int, payload response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
