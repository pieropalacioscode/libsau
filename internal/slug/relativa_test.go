package slug

import "testing"

func TestFromImageURLRutaRelativa(t *testing.T) {
	cases := map[string]string{
		"libprep/mi-libro.avif":          "mi-libro",
		"libprep/mi-libro.avif?v=2":      "mi-libro",
		"libprep/Mi%20Libro%20Ñandú.png": "mi-libro-nandu",
		"mi-libro.avif":                  "mi-libro",
	}
	for in, want := range cases {
		if got := FromImageURL(in); got != want {
			t.Errorf("FromImageURL(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestForProductIgualRelativaYAbsoluta(t *testing.T) {
	abs := "https://img.kelbazar.com/libprep/chimoc-y-el-aseo.avif"
	rel := "libprep/chimoc-y-el-aseo.avif"
	if a, r := ForProduct(&abs, "x"), ForProduct(&rel, "x"); a != r {
		t.Errorf("absoluta %q != relativa %q", a, r)
	}
}
