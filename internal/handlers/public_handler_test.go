package handlers

import (
	"reflect"
	"strings"
	"testing"
)

func TestSearchTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"  Científicas   DEL perú ", []string{"cientificas", "del", "peru"}},
		{"lobo", []string{"lobo"}},
		{"   ", []string{}},
		{"", []string{}},
		{"a b c d e f g h", []string{"a", "b", "c", "d", "e", "f"}}, // máx. 6 palabras
	}
	for _, c := range cases {
		got := searchTokens(c.in)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("searchTokens(%q) = %v, quería %v", c.in, got, c.want)
		}
	}
}

func TestSearchTokensLargo(t *testing.T) {
	// Una consulta enorme se recorta, y recortar por runas no rompe un carácter UTF-8.
	got := searchTokens(strings.Repeat("ñ", 500))
	if len(got) != 1 || len([]rune(got[0])) != searchMaxQueryLen {
		t.Errorf("tokens = %d, largo primer token = %d", len(got), len([]rune(got[0])))
	}
}

func TestLikeEscaper(t *testing.T) {
	if got := likeEscaper.Replace("100%_off!"); got != "100!%!_off!!" {
		t.Errorf("escape = %q", got)
	}
}
