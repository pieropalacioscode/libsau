package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"
	"runtime"
)

// baseDir resuelve la ruta raíz del proyecto para encontrar templates.
// Funciona tanto desde go run como desde el binario compilado.
func baseDir() string {
	_, filename, _, _ := runtime.Caller(0)
	// filename = .../internal/handlers/page_handler.go
	// subimos 3 niveles: handlers → internal → proyecto
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	base := baseDir()
	tmpl, err := template.ParseFiles(
		filepath.Join(base, "templates", "layout.html"),
		filepath.Join(base, "templates", name),
	)
	if err != nil {
		http.Error(w, "Error cargando template: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "Error renderizando: "+err.Error(), http.StatusInternalServerError)
	}
}

// GET / → redirige al POS
func PageHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/pos", http.StatusFound)
}

// GET /pos → pantalla principal de ventas
func PagePOS(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "pos/index.html", nil)
}

// GET /dashboard → métricas del día
func PageDashboard(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "dashboard/index.html", nil)
}
