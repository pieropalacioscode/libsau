package handlers

import (
	"encoding/json"
	"testing"

	"github.com/pieropalacioscode/ladmincabo/internal/models"
)

func marcaStr(s string) *string { return &s }

func marcaVal(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestMarcaHex(t *testing.T) {
	cases := []struct {
		in   *string
		want string
	}{
		{nil, "<nil>"},
		{marcaStr("#F4B400"), "#F4B400"},
		{marcaStr(" #0a1f4d "), "#0a1f4d"},
		{marcaStr("F4B400"), "<nil>"},
		{marcaStr("#FFF"), "<nil>"},
		{marcaStr("red"), "<nil>"},
		{marcaStr("#12345g"), "<nil>"},
		{marcaStr("#F4B400;x"), "<nil>"},
	}
	for _, c := range cases {
		if got := marcaVal(publicHex(c.in)); got != c.want {
			t.Errorf("publicHex(%s) = %s, quería %s", marcaVal(c.in), got, c.want)
		}
	}
}

func TestMarcaSocial(t *testing.T) {
	cases := []struct {
		in      string
		domains []string
		want    bool
	}{
		{"https://www.instagram.com/demo", []string{"instagram.com"}, true},
		{"https://instagram.com/demo", []string{"instagram.com"}, true},
		{"https://m.facebook.com/demo", []string{"facebook.com", "fb.com"}, true},
		{"https://fb.com/demo", []string{"facebook.com", "fb.com"}, true},
		{"http://instagram.com/demo", []string{"instagram.com"}, false},
		{"https://evil.com/instagram.com", []string{"instagram.com"}, false},
		{"https://instagram.com.evil.com/x", []string{"instagram.com"}, false},
		{"https://user:pw@instagram.com/x", []string{"instagram.com"}, false},
		{"javascript:alert(1)", []string{"instagram.com"}, false},
		{"https:///x", []string{"instagram.com"}, false},
		{"", []string{"instagram.com"}, false},
	}
	for _, c := range cases {
		got := publicSocial(marcaStr(c.in), c.domains...) != nil
		if got != c.want {
			t.Errorf("publicSocial(%q) = %v, quería %v", c.in, got, c.want)
		}
	}
	if publicSocial(nil, "instagram.com") != nil {
		t.Error("nil debe seguir siendo nil")
	}
}

func TestMarcaTexto(t *testing.T) {
	long := make([]rune, 301)
	for i := range long {
		long[i] = 'a'
	}
	cases := []struct {
		in   string
		want string
	}{
		{"  Av. Real 123  ", "Av. Real 123"},
		{"Lun-Vie 9:00-18:00\nSáb 9:00-13:00", "Lun-Vie 9:00-18:00\nSáb 9:00-13:00"},
		{"   ", "<nil>"},
		{"a\tb", "<nil>"},
		{"a\x00b", "<nil>"},
		{string(long), "<nil>"},
	}
	for _, c := range cases {
		if got := marcaVal(publicText(marcaStr(c.in), 300)); got != c.want {
			t.Errorf("publicText(%q) = %q, quería %q", c.in, got, c.want)
		}
	}
}

func TestMarcaNegocioPublico(t *testing.T) {
	b := &models.Business{
		Slug: "demo", Name: "Demo",
		WhatsappNumber: marcaStr("51900000000"),
		PrimaryColor:   marcaStr("#0A1F4D"),
		AccentColor:    marcaStr("amarillo"),
		InstagramURL:   marcaStr("https://www.instagram.com/demo"),
		TikTokURL:      marcaStr("http://tiktok.com/@demo"),
		FacebookURL:    marcaStr("https://evil.example/facebook.com"),
		Address:        marcaStr("  Huancayo  "),
		OpeningHours:   marcaStr("   "),
	}
	got := newPublicBusiness(b, marcaStr("https://img.kelbazar.com/demo/logo.png"))
	checks := []struct {
		name string
		got  *string
		want string
	}{
		{"logo", got.LogoURL, "https://img.kelbazar.com/demo/logo.png"},
		{"primary", got.PrimaryColor, "#0A1F4D"},
		{"accent inválido", got.AccentColor, "<nil>"},
		{"instagram", got.InstagramURL, "https://www.instagram.com/demo"},
		{"tiktok http", got.TikTokURL, "<nil>"},
		{"facebook ajeno", got.FacebookURL, "<nil>"},
		{"address", got.Address, "Huancayo"},
		{"horario vacío", got.OpeningHours, "<nil>"},
	}
	for _, c := range checks {
		if v := marcaVal(c.got); v != c.want {
			t.Errorf("%s = %s, quería %s", c.name, v, c.want)
		}
	}
}

// Solo estas claves salen en el JSON público: nada de id, tier, active ni created_at.
func TestMarcaNegocioClaves(t *testing.T) {
	raw, err := json.Marshal(newPublicBusiness(&models.Business{Slug: "x", Name: "X"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	want := []string{"slug", "name", "whatsapp_number", "logo_url", "primary_color", "accent_color",
		"instagram_url", "tiktok_url", "facebook_url", "address", "opening_hours"}
	if len(m) != len(want) {
		t.Errorf("claves = %d, quería %d: %s", len(m), len(want), raw)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("falta la clave %q", k)
		}
	}
}
