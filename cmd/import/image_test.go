package main

import (
	"testing"

	"github.com/pieropalacioscode/ladmincabo/internal/importer"
)

func TestCleanImagePrefix(t *testing.T) {
	ok := map[string]string{"": "", " /libprep/ ": "libprep", "neo-chompa/portadas": "neo-chompa/portadas"}
	for in, want := range ok {
		if got, err := cleanImagePrefix(in); err != nil || got != want {
			t.Errorf("cleanImagePrefix(%q) = %q, %v; quería %q", in, got, err, want)
		}
	}
	for _, in := range []string{"LIBPREP", "../x", "a b", "https://img.x.com", "a//b"} {
		if _, err := cleanImagePrefix(in); err == nil {
			t.Errorf("cleanImagePrefix(%q) debía fallar", in)
		}
	}
}

func TestImageRefFromRow(t *testing.T) {
	cases := []struct {
		name, imagenes, prefix, want string
		wantErr                      bool
	}{
		{"con prefijo", "https://old.com/wp/portada-1.avif, https://x/y.avif", "libprep", "libprep/portada-1.avif", false},
		{"quita query", "https://old.com/a/mi-libro.avif?v=2", "libprep", "libprep/mi-libro.avif", false},
		{"conserva %20", "https://old.com/a/Mi%20Libro.png", "libprep", "libprep/Mi%20Libro.png", false},
		{"solo nombre de archivo", "portada.avif", "libprep", "libprep/portada.avif", false},
		{"sin prefijo: URL completa", "https://dnieve.kelpad.com/a/b.avif", "", "https://dnieve.kelpad.com/a/b.avif", false},
		{"sin prefijo: http falla", "http://x.com/a.avif", "", "", true},
		{"sin archivo", "https://img.x.com/", "libprep", "", true},
		{"placeholder -", "-", "libprep", "", false},
		{"vacío", "", "", "", false},
	}
	for _, c := range cases {
		got, err := imageRefFromRow(importer.ProductRow{Imagenes: c.imagenes}, c.prefix)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr = %v", c.name, err, c.wantErr)
			continue
		}
		switch {
		case c.want == "" && got != nil:
			t.Errorf("%s: quería nil, dio %q", c.name, *got)
		case c.want != "" && (got == nil || *got != c.want):
			t.Errorf("%s: dio %v, quería %q", c.name, got, c.want)
		}
	}
}
