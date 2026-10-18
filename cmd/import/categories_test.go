package main

import (
	"reflect"
	"testing"
)

func TestSplitCategoryPath(t *testing.T) {
	cases := map[string][]string{
		"Grupo A > Hoja uno":        {"Grupo A", "Hoja uno"},
		"  Grupo B>Hoja, con coma ": {"Grupo B", "Hoja, con coma"},
		"Solo":                      {"Solo"},
		"":                          nil,
		" > ":                       nil,
		"A > > B":                   {"A", "B"},
	}
	for in, want := range cases {
		if got := splitCategoryPath(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitCategoryPath(%q) = %v, quería %v", in, got, want)
		}
	}
}

func TestSameParent(t *testing.T) {
	a, b, c := uint(1), uint(1), uint(2)
	if !sameParent(nil, nil) || !sameParent(&a, &b) || sameParent(&a, &c) || sameParent(&a, nil) || sameParent(nil, &a) {
		t.Error("sameParent no se comporta como se esperaba")
	}
}
