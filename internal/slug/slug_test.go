package slug

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Científicas del Perú: 24 historias": "cientificas-del-peru-24-historias",
		"  ¡Año Nuevo!  ":                    "ano-nuevo",
		"ÁÉÍÓÚ Ü Ñ":                          "aeiou-u-n",
		"---":                                "",
		"":                                   "",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestFold(t *testing.T) {
	if got := Fold("Científicas DEL Perú, Ñandú!"); got != "cientificas del peru, nandu!" {
		t.Errorf("Fold = %q", got)
	}
	if len([]rune(FoldFrom)) != len([]rune(FoldTo)) {
		t.Errorf("FoldFrom y FoldTo deben tener el mismo número de caracteres")
	}
	// Cada pareja de translate() debe dar lo mismo que Fold.
	from, to := []rune(FoldFrom), []rune(FoldTo)
	for i := range from {
		if got := Fold(string(from[i])); got != string(to[i]) {
			t.Errorf("Fold(%q) = %q, translate daría %q", string(from[i]), got, string(to[i]))
		}
	}
}

func TestSlugifyLargo(t *testing.T) {
	got := Slugify(strings.Repeat("a", 300))
	if len(got) != MaxLen {
		t.Errorf("largo = %d, quería %d", len(got), MaxLen)
	}
}

func TestFromImageURL(t *testing.T) {
	cases := map[string]string{
		"https://img.docentesmart.com/libprep/cientificas-del-peru-24-historias-por-descubrir-concytec.avif": "cientificas-del-peru-24-historias-por-descubrir-concytec",
		"https://img.docentesmart.com/libprep/mi-libro.avif?v=2":                                             "mi-libro",
		"https://img.docentesmart.com/libprep/Mi%20Libro%20Ñandú.png":                                        "mi-libro-nandu",
		"https://img.docentesmart.com/":                                                                      "",
		"::no es una url::":                                                                                  "",
	}
	for in, want := range cases {
		if got := FromImageURL(in); got != want {
			t.Errorf("FromImageURL(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestForProduct(t *testing.T) {
	img := "https://img.docentesmart.com/libprep/chimoc-y-el-aseo.avif"
	vacia := ""

	cases := []struct {
		img  *string
		name string
		want string
	}{
		{&img, "Chimoc y el aseo", "chimoc-y-el-aseo"},
		{nil, "Cuaderno A4 rayado", "cuaderno-a4-rayado"},
		{&vacia, "Lápiz HB", "lapiz-hb"},
		{nil, "2024", "producto-2024"},
		{nil, "", "producto"},
	}
	for _, c := range cases {
		if got := ForProduct(c.img, c.name); got != c.want {
			t.Errorf("ForProduct(%v, %q) = %q, quería %q", c.img, c.name, got, c.want)
		}
	}
}
