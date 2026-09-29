#!/usr/bin/env bash
# Verificación de la Fase 2: CRUD de producto/categoría con negocio obligatorio.
# Requisitos: servidor corriendo sin bypass y haber corrido antes verify_fase1.sh (crea el negocio 2).
# Uso: bash scripts/verify_fase2.sh
set -u

BASE="${BASE:-http://localhost:8080}"
SUF="$(date +%s)"
PSQL=(docker exec -i libsau_postgres psql -U libsau_user -d libsau_db -tA -v ON_ERROR_STOP=1)
PASS=0; FAIL=0

ok()  { echo "  ✅ $1"; PASS=$((PASS+1)); }
bad() { echo "  ❌ $1"; FAIL=$((FAIL+1)); }
eq()  { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 — esperado: [$2] obtenido: [$3]"; fi; }
sql() { "${PSQL[@]}" -c "$1" 2>&1 | tr -d '\r'; }

call() {  # call TOKEN BIZ METHOD PATH [JSON] → cuerpo + "\n" + código HTTP
  local tok="$1" biz="$2" m="$3" p="$4" d="${5:-}"
  local a=(-s -X "$m" "$BASE$p" -H "Content-Type: application/json")
  [ -n "$tok" ] && a+=(-H "Authorization: Bearer $tok")
  [ -n "$biz" ] && a+=(-H "X-Business-ID: $biz")
  [ -n "$d" ]   && a+=(-d "$d")
  curl "${a[@]}" -w $'\n%{http_code}'
}
http_code() { echo "${1##*$'\n'}"; }
resp_body() { echo "${1%$'\n'*}"; }

if [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/health")" = "000" ]; then
  echo "❌ El servidor no responde en $BASE. Levántalo: go run ./cmd/api"; exit 1
fi
if [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/products")" != "401" ]; then
  echo "❌ La API acepta peticiones sin token (bypass activo). Reinicia sin APP_ENV=development."; exit 1
fi

ADMIN="$(curl -s -X POST "$BASE/auth/login" -H "Content-Type: application/json" \
  -d '{"email":"admin@libreriasaber.com","password":"Admin2025!"}' | jq -r '.access_token // empty')"
[ -n "$ADMIN" ] || { echo "❌ No se pudo iniciar sesión como admin."; exit 1; }
BIZ1="$(sql "SELECT id FROM businesses WHERE slug='libreria-saber'")"
BIZ2="$(sql "SELECT id FROM businesses WHERE slug='libros-estudiante'")"
[[ "$BIZ2" =~ ^[0-9]+$ ]] || { echo "❌ Falta el negocio 2: corre primero scripts/verify_fase1.sh"; exit 1; }

mkcat()  { resp_body "$(call "$ADMIN" "$1" POST /api/v1/categories "{\"name\":\"$2\"}")" | jq -r '.id // empty'; }
mkprod() { call "$ADMIN" "$1" POST /api/v1/products \
  "{\"name\":\"Fase2 $2\",\"sku\":\"$2\",\"price\":6,\"cost\":3,\"stock\":10,\"category_id\":$3}"; }
pid()    { resp_body "$1" | jq -r '.id // empty'; }

echo "Preparación"
CAT1="$(mkcat "$BIZ1" "F2-A-$SUF")"; CAT2="$(mkcat "$BIZ2" "F2-B-$SUF")"
PA="$(pid "$(mkprod "$BIZ1" "F2A-$SUF" "${CAT1:-0}")")"   # producto del negocio 1
PB="$(pid "$(mkprod "$BIZ2" "F2B-$SUF" "${CAT2:-0}")")"   # producto del negocio 2
if [ -z "$CAT1" ] || [ -z "$CAT2" ] || [ -z "$PA" ] || [ -z "$PB" ]; then
  bad "no se pudieron crear las categorías/productos de prueba (cat1=$CAT1 cat2=$CAT2 pa=$PA pb=$PB)"; exit 1
fi
ok "categorías y productos de prueba creados (negocio $BIZ1: prod $PA · negocio $BIZ2: prod $PB)"

echo "A. Leer un producto ajeno"
eq "GET producto del negocio 2 desde el negocio 1 → 404" 404 "$(http_code "$(call "$ADMIN" "$BIZ1" GET "/api/v1/products/$PB")")"
eq "GET producto desde su propio negocio → 200" 200 "$(http_code "$(call "$ADMIN" "$BIZ2" GET "/api/v1/products/$PB")")"

echo "B. Editar un producto ajeno"
eq "PATCH producto ajeno → 404" 404 "$(http_code "$(call "$ADMIN" "$BIZ1" PATCH "/api/v1/products/$PB" '{"name":"Hackeado"}')")"
eq "el producto ajeno no cambió de nombre" "Fase2 F2B-$SUF" "$(sql "SELECT name FROM products WHERE id=$PB")"
eq "PATCH producto propio → 200" 200 \
  "$(http_code "$(call "$ADMIN" "$BIZ2" PATCH "/api/v1/products/$PB" "{\"name\":\"Fase2 F2B-$SUF editado\"}")")"

echo "C. No se puede mover un producto de negocio"
call "$ADMIN" "$BIZ1" PATCH "/api/v1/products/$PA" "{\"business_id\":$BIZ2,\"name\":\"Fase2 F2A-$SUF mov\"}" >/dev/null
eq "PATCH con business_id en el body no cambia el negocio" "$BIZ1" "$(sql "SELECT business_id FROM products WHERE id=$PA")"
R="$(call "$ADMIN" "$BIZ1" POST /api/v1/products \
  "{\"name\":\"Fase2 F2C-$SUF\",\"sku\":\"F2C-$SUF\",\"price\":6,\"cost\":3,\"stock\":1,\"category_id\":$CAT1,\"business_id\":$BIZ2}")"
if [ "$(http_code "$R")" = "201" ]; then
  eq "POST con business_id ajeno en el body se ignora" "$BIZ1" "$(sql "SELECT business_id FROM products WHERE id=$(pid "$R")")"
else
  ok "POST con business_id ajeno en el body → rechazado ($(http_code "$R"))"
fi

echo "D. Stock y borrado de un producto ajeno"
STK0="$(sql "SELECT stock FROM products WHERE id=$PB")"
C="$(http_code "$(call "$ADMIN" "$BIZ1" PATCH "/api/v1/products/$PB/stock" '{"tipo":"sub","amount":5,"reason":"verificacion"}')")"
eq "ajustar stock de un producto ajeno no lo modifica" "$STK0" "$(sql "SELECT stock FROM products WHERE id=$PB")"
[ "$C" = "404" ] && ok "ajuste de stock ajeno → 404" || bad "ajuste de stock ajeno respondió $C (esperado 404)"
C="$(http_code "$(call "$ADMIN" "$BIZ1" DELETE "/api/v1/products/$PB")")"
eq "DELETE ajeno no lo borra (sigue visible en su negocio)" 200 "$(http_code "$(call "$ADMIN" "$BIZ2" GET "/api/v1/products/$PB")")"
[ "$C" = "404" ] && ok "DELETE ajeno → 404" || bad "DELETE ajeno respondió $C (esperado 404)"

echo "E. Categorías por negocio"
LC="$(resp_body "$(call "$ADMIN" "$BIZ1" GET /api/v1/categories)")"
echo "$LC" | grep -qF "F2-B-$SUF" && bad "el negocio 1 ve la categoría del negocio 2" || ok "el negocio 1 no ve categorías del negocio 2"
LC="$(resp_body "$(call "$ADMIN" "$BIZ2" GET /api/v1/categories)")"
echo "$LC" | grep -qF "F2-B-$SUF" && ok "el negocio 2 ve su propia categoría" || bad "el negocio 2 no ve su propia categoría"

echo "F. Referencias cruzadas entre negocios"
C="$(http_code "$(mkprod "$BIZ1" "F2X-$SUF" "$CAT2")")"
[ "$C" != "201" ] && ok "crear producto con categoría de OTRO negocio → rechazado ($C)" \
  || bad "se aceptó un producto del negocio 1 con categoría del negocio 2"
C="$(http_code "$(call "$ADMIN" "$BIZ1" PATCH "/api/v1/products/$PA" "{\"category_id\":$CAT2}")")"
eq "PATCH con categoría de otro negocio no la asigna" "$CAT1" "$(sql "SELECT category_id FROM products WHERE id=$PA")"
[ "$C" != "200" ] && ok "PATCH con categoría ajena → rechazado ($C)" || bad "PATCH con categoría ajena respondió 200"

echo "G. El SKU no se puede editar por PATCH (inmutable por API)"
PD="$(pid "$(mkprod "$BIZ1" "F2D-$SUF" "$CAT1")")"
C="$(http_code "$(call "$ADMIN" "$BIZ1" PATCH "/api/v1/products/${PD:-0}" "{\"sku\":\"F2A-$SUF\"}")")"
eq "PATCH con sku no cambia el SKU" "F2D-$SUF" "$(sql "SELECT sku FROM products WHERE id=${PD:-0}")"
[ "$C" != "200" ] && ok "PATCH solo con sku → rechazado ($C)" || bad "PATCH solo con sku respondió 200"

echo
echo "Resultado: $PASS ok, $FAIL fallos"
[ "$FAIL" -eq 0 ] && echo "FASE 2 VERIFICADA ✅" || echo "Hay fallos: pégame la salida completa junto con product_handler.go, category_handler.go y dto.go."
exit "$FAIL"