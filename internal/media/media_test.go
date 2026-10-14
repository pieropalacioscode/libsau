package media

import "testing"

func TestURL(t *testing.T) {
	r := NewResolver("https://img.kelbazar.com/")
	cases := map[string]string{
		"libprep/portada.avif":               "https://img.kelbazar.com/libprep/portada.avif",
		"/libprep/portada.avif":              "https://img.kelbazar.com/libprep/portada.avif",
		"libprep/Mi%20Libro.png?v=2":         "https://img.kelbazar.com/libprep/Mi%20Libro.png?v=2",
		"../secreto.png":                     "https://img.kelbazar.com/secreto.png",
		"https://dnieve.kelpad.com/x/a.avif": "https://dnieve.kelpad.com/x/a.avif",
		"HTTP://otro.com/a.png":              "HTTP://otro.com/a.png",
		"":                                   "",
		"   ":                                "",
	}
	for in, want := range cases {
		if got := r.URL(in); got != want {
			t.Errorf("URL(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestURLSinBase(t *testing.T) {
	r := NewResolver("")
	if got := r.URL("libprep/a.avif"); got != "" {
		t.Errorf("ruta relativa sin base debe dar vacío, dio %q", got)
	}
	if got := r.URL("https://x.com/a.avif"); got != "https://x.com/a.avif" {
		t.Errorf("URL completa debe pasar igual, dio %q", got)
	}
}

func TestPtr(t *testing.T) {
	r := NewResolver("https://img.kelbazar.com")
	if r.Ptr(nil) != nil {
		t.Error("nil debe dar nil")
	}
	vacio := ""
	if r.Ptr(&vacio) != nil {
		t.Error("vacío debe dar nil")
	}
	k := "libprep/a.avif"
	if got := r.Ptr(&k); got == nil || *got != "https://img.kelbazar.com/libprep/a.avif" {
		t.Errorf("Ptr = %v", got)
	}
}
