package handlers

import (
	"fmt"
	"net/url"
	"testing"
)

func TestParseFilters(t *testing.T) {
	v := url.Values{
		"f":         {"Editorial:Alfaguara", "Editorial:Norma", "Editorial:Norma", "Impresión:Color"},
		"min_price": {"10"},
	}
	spec, err := parseFilters(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.attrs["Editorial"]) != 2 {
		t.Errorf("Editorial = %v, quería 2 valores sin repetir", spec.attrs["Editorial"])
	}
	if spec.minPrice == nil || *spec.minPrice != 10 || spec.maxPrice != nil {
		t.Errorf("rango de precio mal leído")
	}

	spec, err = parseFilters(url.Values{"f": {"Título:Parte 1: inicio"}})
	if err != nil || spec.attrs["Título"][0] != "Parte 1: inicio" {
		t.Errorf("un valor con ':' debe conservarse completo: %v %v", spec.attrs, err)
	}

	bad := []url.Values{
		{"f": {"sinvalor"}}, {"f": {":x"}}, {"f": {"a:"}},
		{"min_price": {"-1"}}, {"max_price": {"NaN"}}, {"min_price": {"abc"}}, {"max_price": {"1e9"}},
	}
	for _, b := range bad {
		if _, err := parseFilters(b); err == nil {
			t.Errorf("%v debía fallar", b)
		}
	}

	many := url.Values{}
	for i := 0; i <= filterMaxGroups; i++ {
		many.Add("f", fmt.Sprintf("a%d:x", i))
	}
	if _, err := parseFilters(many); err == nil {
		t.Error("más de filterMaxGroups atributos debía fallar")
	}
	vals := url.Values{}
	for i := 0; i <= filterMaxValues; i++ {
		vals.Add("f", fmt.Sprintf("a:v%d", i))
	}
	if _, err := parseFilters(vals); err == nil {
		t.Error("más de filterMaxValues valores debía fallar")
	}
}
