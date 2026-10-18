package handlers

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/pieropalacioscode/ladmincabo/internal/models"
)

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// publicHex: solo #RRGGBB; cualquier otra cosa se oculta (el frontend usa su color de respaldo).
func publicHex(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if !hexColorRe.MatchString(v) {
		return nil
	}
	return &v
}

// publicSocial: solo https, sin credenciales y con host del dominio esperado (o un
// subdominio). Evita que un valor mal cargado publique un enlace ajeno.
func publicSocial(s *string, domains ...string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return &v
		}
	}
	return nil
}

// publicText: texto libre de una o varias líneas; se oculta si está vacío, pasa del
// límite o trae caracteres de control (salvo el salto de línea).
func publicText(s *string, limit int) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" || len([]rune(v)) > limit {
		return nil
	}
	for _, r := range v {
		if unicode.IsControl(r) && r != '\n' {
			return nil
		}
	}
	return &v
}

// newPublicBusiness arma el DTO público. logo ya viene resuelto por media.Resolver.
func newPublicBusiness(b *models.Business, logo *string) publicBusiness {
	return publicBusiness{
		Slug:           b.Slug,
		Name:           b.Name,
		WhatsappNumber: b.WhatsappNumber,
		LogoURL:        logo,
		PrimaryColor:   publicHex(b.PrimaryColor),
		AccentColor:    publicHex(b.AccentColor),
		InstagramURL:   publicSocial(b.InstagramURL, "instagram.com"),
		TikTokURL:      publicSocial(b.TikTokURL, "tiktok.com"),
		FacebookURL:    publicSocial(b.FacebookURL, "facebook.com", "fb.com"),
		Address:        publicText(b.Address, 300),
		OpeningHours:   publicText(b.OpeningHours, 300),
	}
}
