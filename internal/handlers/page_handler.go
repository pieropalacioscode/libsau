package handlers

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"
)

func tmplDir() string {
	// Busca templates desde el directorio de trabajo actual
	wd, _ := os.Getwd()
	return filepath.Join(wd, "templates")
}

func render(w http.ResponseWriter, r *http.Request, page string, data any) {
	base := tmplDir()
	files := []string{
		filepath.Join(base, "layout.html"),
		filepath.Join(base, page),
	}
	tmpl, err := template.ParseFiles(files...)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "Render error: "+err.Error(), 500)
	}
}

func renderLogin(w http.ResponseWriter, r *http.Request) {
	base := tmplDir()
	tmpl, err := template.ParseFiles(filepath.Join(base, "login.html"))
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, nil)
}

func PageHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/pos", http.StatusFound)
}

func PageLogin(w http.ResponseWriter, r *http.Request) {
	renderLogin(w, r)
}

func PagePOS(w http.ResponseWriter, r *http.Request) {
	render(w, r, "pos/index.html", map[string]string{"Title": "Punto de Venta"})
}

// func PageDashboard(w http.ResponseWriter, r *http.Request) {
// 	render(w, r, "dashboard/index.html", map[string]string{"Title": "Dashboard"})
// }

func PageCash(w http.ResponseWriter, r *http.Request) {
	render(w, r, "cash/index.html", map[string]string{"Title": "Cierre de Caja"})
}

func PageProducts(w http.ResponseWriter, r *http.Request) {
	render(w, r, "products/index.html", map[string]string{"Title": "Productos"})
}
