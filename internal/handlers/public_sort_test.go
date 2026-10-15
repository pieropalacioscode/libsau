package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestPublicSortSQL(t *testing.T) {
	for k, v := range publicSortSQL {
		if !strings.HasPrefix(v, "products.") || strings.ContainsAny(v, ";'\"") {
			t.Errorf("orden %q no es seguro: %q", k, v)
		}
	}
	if _, ok := publicSortSQL["; DROP TABLE products"]; ok {
		t.Error("una clave arbitraria no debe existir")
	}
}

func TestOptTime(t *testing.T) {
	if optTime(time.Time{}) != nil {
		t.Error("la fecha cero debe dar nil")
	}
	now := time.Now()
	if got := optTime(now); got == nil || !got.Equal(now) {
		t.Error("una fecha real debe conservarse")
	}
}
