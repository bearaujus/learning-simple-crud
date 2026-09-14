package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"

	apispec "learning-simple-crud/api"
	"learning-simple-crud/web"
)

type application struct {
	db *sql.DB
}

type apiError struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Details   map[string]string `json:"details,omitempty"`
	RequestID string            `json:"request_id"`
}

// NewHandler builds the API and playground routes with the configured CORS origins.
func NewHandler(db *sql.DB, origins []string) http.Handler {
	app := &application{db: db}
	mux := http.NewServeMux()
	// Fallback handlers keep 404 and 405 responses in the same JSON format.
	route := func(path string, methods map[string]http.HandlerFunc) {
		allowed := []string{http.MethodOptions}
		for method, handler := range methods {
			mux.HandleFunc(method+" "+path, handler)
			allowed = append(allowed, method)
			if method == http.MethodGet {
				allowed = append(allowed, http.MethodHead)
			}
		}
		sort.Strings(allowed)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			sendError(w, http.StatusMethodNotAllowed, "method_not_allowed", "This endpoint does not support that HTTP method. See the Allow header.", nil)
		})
	}
	route("/products", map[string]http.HandlerFunc{"GET": app.listProducts, "POST": app.createProduct})
	route("/products/{id}", map[string]http.HandlerFunc{
		"GET": app.getProduct, "PUT": app.replaceProduct, "PATCH": app.patchProduct, "DELETE": app.deleteProduct,
	})
	route("/health", map[string]http.HandlerFunc{"GET": app.health})
	for path, file := range map[string]string{
		"/{$}": "index.html", "/docs": "docs.html", "/app.js": "app.js",
		"/api.js": "api.js", "/styles.css": "styles.css",
	} {
		route(path, map[string]http.HandlerFunc{"GET": func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, web.Files, file)
		}})
	}
	route("/openapi.json", map[string]http.HandlerFunc{"GET": func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, apispec.Files, "openapi.json")
	}})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		sendError(w, http.StatusNotFound, "route_not_found", "Endpoint not found. Open /docs for available routes.", nil)
	})
	return middleware(mux, origins)
}

func middleware(next http.Handler, origins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", rand.Text())
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Add("Vary", "Origin")
		for _, origin := range origins {
			origin = strings.TrimSpace(origin)
			if origin == "*" || (origin != "" && origin == r.Header.Get("Origin")) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Expose-Headers", "Location, X-Request-ID")
				break
			}
		}
		defer func() {
			if value := recover(); value != nil {
				log.Printf("request_id=%s panic=%v", w.Header().Get("X-Request-ID"), value)
				sendError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func sendJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("request_id=%s encode response: %v", w.Header().Get("X-Request-ID"), err)
	}
}

func sendData(w http.ResponseWriter, status int, data any) {
	sendJSON(w, status, struct {
		Data any `json:"data"`
	}{Data: data})
}

func sendError(w http.ResponseWriter, status int, code, message string, details map[string]string) {
	sendJSON(w, status, struct {
		Error apiError `json:"error"`
	}{Error: apiError{Code: code, Message: message, Details: details, RequestID: w.Header().Get("X-Request-ID")}})
}

func internalError(w http.ResponseWriter, err error) {
	log.Printf("request_id=%s database error: %v", w.Header().Get("X-Request-ID"), err)
	sendError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.", nil)
}

func (app *application) health(w http.ResponseWriter, r *http.Request) {
	if err := app.db.PingContext(r.Context()); err != nil {
		log.Printf("request_id=%s health: %v", w.Header().Get("X-Request-ID"), err)
		sendError(w, http.StatusServiceUnavailable, "service_unavailable", "The database is unavailable. Please try again later.", nil)
		return
	}
	sendData(w, http.StatusOK, map[string]string{"status": "ok"})
}
