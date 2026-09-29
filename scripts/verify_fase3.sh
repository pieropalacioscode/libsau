#!/usr/bin/env bash
# Verificación de la Fase 3: caja y dashboard solo para negocios FULL, sin mezclar datos.
# Requisitos: servidor corriendo sin bypass; negocio 1 (libreria-saber) FULL y negocio 2
# (libros-estudiante) que NO sea FULL (lo crea verify_fase1.sh).
# Seguro de correr en datos reales: el POST /cash/close se prueba con un monto inválido (-1),
# que se rechaza antes de escribir nada.
# Uso: bash scripts/verify_fase3.sh
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

T1="$(sql "SELECT tier FROM businesses WHERE id=$BIZ1")"
T2="$(sql "SELECT tier FROM businesses WHERE id=$BIZ2")"
[ "$T1" = "FULL" ] || { echo "❌ El negocio 1 debe ser tier FULL (es: $T1)."; exit 1; }
[ "$T2" != "FULL" ] || { echo "❌ El negocio 2 debe ser CATALOG, no FULL (es: $T2)."; exit 1; }
echo "Negocio $BIZ1 = $T1 · negocio $BIZ2 = $T2"

echo "A. Negocio FULL: todo sigue funcionando"
eq "dashboard → 200"     200 "$(http_code "$(call "$ADMIN" "$BIZ1" GET /api/v1/dashboard)")"
eq "cash/today → 200"    200 "$(http_code "$(call "$ADMIN" "$BIZ1" GET /api/v1/cash/today)")"
eq "cash/history → 200"  200 "$(http_code "$(call "$ADMIN" "$BIZ1" GET /api/v1/cash/history)")"
eq "cash/close con monto inválido → 400 (pasa el guard, la validación lo frena)" 400 \
  "$(http_code "$(call "$ADMIN" "$BIZ1" POST /api/v1/cash/close '{"cash_declared":-1}')")"

echo "B. Negocio CATALOG: rutas FULL bloqueadas"
eq "dashboard → 403"     403 "$(http_code "$(call "$ADMIN" "$BIZ2" GET /api/v1/dashboard)")"
eq "cash/today → 403"    403 "$(http_code "$(call "$ADMIN" "$BIZ2" GET /api/v1/cash/today)")"
eq "cash/history → 403"  403 "$(http_code "$(call "$ADMIN" "$BIZ2" GET /api/v1/cash/history)")"
eq "cash/close → 403 (antes de validar el body)" 403 \
  "$(http_code "$(call "$ADMIN" "$BIZ2" POST /api/v1/cash/close '{"cash_declared":-1}')")"

echo "C. El dashboard del negocio FULL no muestra productos de otro negocio"
CAT2="$(resp_body "$(call "$ADMIN" "$BIZ2" POST /api/v1/categories "{\"name\":\"F3-$SUF\"}")" | jq -r '.id // empty')"
P2="$(resp_body "$(call "$ADMIN" "$BIZ2" POST /api/v1/products \
  "{\"name\":\"Fase3 F3-$SUF\",\"sku\":\"F3-$SUF\",\"price\":6,\"cost\":3,\"stock\":0,\"category_id\":${CAT2:-0}}")" | jq -r '.id // empty')"
if [ -z "$CAT2" ] || [ -z "$P2" ]; then
  bad "no se pudo crear el producto de prueba en el negocio 2"
else
  D="$(resp_body "$(call "$ADMIN" "$BIZ1" GET "/api/v1/dashboard?low_stock_threshold=5")")"
  echo "$D" | grep -qF "F3-$SUF" && bad "el stock bajo del negocio 1 muestra el producto del negocio 2" \
    || ok "el producto de stock 0 del negocio 2 no aparece en el dashboard del negocio 1"
  IDS="$(echo "$D" | jq -r '.low_stock | map(.id) | join(",")')"
  if [ -n "$IDS" ]; then
    eq "todos los ítems de stock bajo pertenecen al negocio $BIZ1" 0 \
      "$(sql "SELECT count(*) FROM products WHERE id IN ($IDS) AND business_id <> $BIZ1")"
  else
    ok "stock bajo del negocio $BIZ1 vacío (nada que mezclar)"
  fi
fi

echo
echo "Resultado: $PASS ok, $FAIL fallos"
[ "$FAIL" -eq 0 ] && echo "FASE 3 VERIFICADA ✅" || echo "Hay fallos: pégame la salida completa junto con internal/middleware (la parte que resuelve BusinessID)."
exit "$FAIL"