#!/usr/bin/env bash
# Verificación de la Fase 1 (multi-negocio) de LIBSAU.
# Requisito: el servidor ya está corriendo en otra terminal (go run cmd/api/main.go).
# Uso:       bash scripts/verify_fase1.sh
# Es un archivo (no pegado en la terminal) para evitar el error "event not found" de bash con "!".
set -u

BASE="${BASE:-http://localhost:8080}"
SUF="$(date +%s)"                      # sufijo único: se puede correr varias veces
PSQL=(docker exec -i libsau_postgres psql -U libsau_user -d libsau_db -tA -v ON_ERROR_STOP=1)
PASS=0; FAIL=0

ok()  { echo "  ✅ $1"; PASS=$((PASS+1)); }
bad() { echo "  ❌ $1"; FAIL=$((FAIL+1)); }
eq()  { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 — esperado: [$2] obtenido: [$3]"; fi; }
sql() { "${PSQL[@]}" -c "$1" 2>&1 | tr -d '\r'; }

# call TOKEN BIZ METHOD PATH [JSON]  → imprime cuerpo + "\n" + código HTTP
call() {
  local tok="$1" biz="$2" m="$3" p="$4" d="${5:-}"
  local a=(-s -X "$m" "$BASE$p" -H "Content-Type: application/json")
  [ -n "$tok" ] && a+=(-H "Authorization: Bearer $tok")
  [ -n "$biz" ] && a+=(-H "X-Business-ID: $biz")
  [ -n "$d" ]   && a+=(-d "$d")
  curl "${a[@]}" -w $'\n%{http_code}'
}
http_code() { echo "${1##*$'\n'}"; }
resp_body() { echo "${1%$'\n'*}"; }
login() {
  curl -s -X POST "$BASE/auth/login" -H "Content-Type: application/json" \
    -d "{\"email\":\"$1\",\"password\":\"Admin2025!\"}" | jq -r '.access_token // empty'
}

if [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/health")" = "000" ]; then
  echo "❌ El servidor no responde en $BASE. Levántalo primero: go run cmd/api/main.go"; exit 1
fi

# ── A. Esquema ───────────────────────────────────────────────────────────────
echo "A. Esquema de base de datos"
T="'categories','products','sales'"
eq "business_id existe en categories, products y sales" 3 \
  "$(sql "SELECT count(*) FROM information_schema.columns WHERE column_name='business_id' AND table_name IN ($T)")"
eq "business_id es NOT NULL en las 3" 3 \
  "$(sql "SELECT count(*) FROM information_schema.columns WHERE column_name='business_id' AND table_name IN ($T) AND is_nullable='NO'")"
eq "ninguna fila con business_id NULL" 0 \
  "$(sql "SELECT (SELECT count(*) FROM categories WHERE business_id IS NULL)+(SELECT count(*) FROM products WHERE business_id IS NULL)+(SELECT count(*) FROM sales WHERE business_id IS NULL)")"
eq "sales.user_id acepta NULL (pedidos WOO)" YES \
  "$(sql "SELECT is_nullable FROM information_schema.columns WHERE table_name='sales' AND column_name='user_id'")"
eq "negocio libreria-saber sembrado con tier FULL" FULL \
  "$(sql "SELECT tier FROM businesses WHERE slug='libreria-saber'")"
eq "ya no hay unique global sobre sku" 0 \
  "$(sql "SELECT count(*) FROM pg_indexes WHERE tablename='products' AND indexdef LIKE 'CREATE UNIQUE%' AND indexdef LIKE '%(sku)%'")"
eq "existe unique de sku por negocio" 1 \
  "$(sql "SELECT (count(*)>0)::int FROM pg_indexes WHERE tablename='products' AND indexdef LIKE 'CREATE UNIQUE%' AND indexdef LIKE '%business_id%' AND indexdef LIKE '%sku%'")"

# ── B. Autenticación ─────────────────────────────────────────────────────────
echo "B. Autenticación"
NOTOK="$(http_code "$(call '' '' GET /api/v1/products)")"
eq "sin token → 401 (el bypass de desarrollo no está activo)" 401 "$NOTOK"
[ "$NOTOK" != "401" ] && echo "  ⚠️  Bypass activo (APP_ENV=development): los tests con vendedor no son válidos. Reinicia con APP_ENV=test"

# ── C. Datos de prueba ───────────────────────────────────────────────────────
echo "C. Preparando negocio 2 y vendedor"
BIZ1="$(sql "SELECT id FROM businesses WHERE slug='libreria-saber'")"
sql "INSERT INTO businesses (slug,name,tier,active,created_at) VALUES ('libros-estudiante','Libros El Estudiante','CATALOG',true,now()) ON CONFLICT (slug) DO NOTHING" >/dev/null
BIZ2="$(sql "SELECT id FROM businesses WHERE slug='libros-estudiante'")"
sql "INSERT INTO users (name,email,password_hash,role,active,business_id,created_at,updated_at) SELECT 'Vendedor Test','vendedor@test.local',password_hash,'vendedor',true,$BIZ1,now(),now() FROM users WHERE email='admin@libreriasaber.com' ON CONFLICT (email) DO NOTHING" >/dev/null
echo "  negocio 1 = $BIZ1, negocio 2 = $BIZ2"

ADMIN="$(login admin@libreriasaber.com)"; VEND="$(login vendedor@test.local)"
[ -n "$ADMIN" ] && ok "login admin" || { bad "login admin"; exit 1; }
[ -n "$VEND" ]  && ok "login vendedor" || { bad "login vendedor"; exit 1; }
VEND_ID="$(sql "SELECT id FROM users WHERE email='vendedor@test.local'")"

mkcat() { resp_body "$(call "$ADMIN" "$1" POST /api/v1/categories "{\"name\":\"Verif-$SUF\"}")" | jq -r '.id // empty'; }
mkprod() { call "$ADMIN" "$1" POST /api/v1/products \
  "{\"name\":\"Verif $2\",\"sku\":\"$2\",\"price\":6,\"cost\":3,\"stock\":$4,\"category_id\":$3}"; }
CAT1="$(mkcat "$BIZ1")"; CAT2="$(mkcat "$BIZ2")"

echo "D. Creación con negocio"
R="$(mkprod "$BIZ1" "V1-$SUF" "$CAT1" 10)"; P1="$(resp_body "$R" | jq -r '.id // empty')"
eq "crear producto en negocio 1 → 201" 201 "$(http_code "$R")"
eq "el producto queda con business_id del negocio 1" "$BIZ1" "$(sql "SELECT business_id FROM products WHERE id=${P1:-0}")"
R="$(mkprod "$BIZ2" "V2-$SUF" "$CAT2" 5)"; P2="$(resp_body "$R" | jq -r '.id // empty')"
eq "crear producto en negocio 2 (header X-Business-ID) → 201" 201 "$(http_code "$R")"
eq "el producto queda con business_id del negocio 2" "$BIZ2" "$(sql "SELECT business_id FROM products WHERE id=${P2:-0}")"
eq "mismo SKU en OTRO negocio → 201" 201 "$(http_code "$(mkprod "$BIZ2" "V1-$SUF" "$CAT2" 1)")"
R="$(mkprod "$BIZ1" "V1-$SUF" "$CAT1" 1)"
[ "$(http_code "$R")" != "201" ] && ok "mismo SKU en el MISMO negocio → rechazado ($(http_code "$R"))" \
  || bad "mismo SKU en el mismo negocio fue aceptado (debería rechazarse)"

echo "E. Aislamiento de lectura (productos)"
L="$(resp_body "$(call "$ADMIN" '' GET /api/v1/products)")"
eq "admin sin header no ve productos del negocio 2" 0 \
  "$(echo "$L" | jq --arg s "V2-$SUF" '[.[]?|select(.sku==$s)]|length' 2>/dev/null)"
L="$(resp_body "$(call "$ADMIN" "$BIZ2" GET /api/v1/products)")"
eq "admin con X-Business-ID ve el producto del negocio 2" 1 \
  "$(echo "$L" | jq --arg s "V2-$SUF" '[.[]?|select(.sku==$s)]|length' 2>/dev/null)"
eq "admin con X-Business-ID no ve productos de otro negocio" 0 \
  "$(echo "$L" | jq --argjson b "$BIZ2" '[.[]?|select(.business_id!=$b)]|length' 2>/dev/null)"
eq "vendedor pidiendo el negocio 2 → 403" 403 "$(http_code "$(call "$VEND" "$BIZ2" GET /api/v1/products)")"
S="$(resp_body "$(call "$VEND" '' GET "/api/v1/products/search?q=V2-$SUF")")"
echo "$S" | grep -qF "Verif V2-$SUF" && bad "la búsqueda del vendedor filtró un producto del negocio 2" \
  || ok "la búsqueda del vendedor no devuelve productos del negocio 2"

echo "F. Ventas"
eq "vendedor vende producto del negocio 2 → 404" 404 \
  "$(http_code "$(call "$VEND" '' POST /api/v1/sales "{\"pay_method\":\"EFECTIVO\",\"items\":[{\"product_id\":${P2:-0},\"quantity\":1,\"discount\":0}]}")")"
R="$(call "$VEND" '' POST /api/v1/sales "{\"pay_method\":\"EFECTIVO\",\"items\":[{\"product_id\":${P1:-0},\"quantity\":1,\"discount\":0}]}")"
SID="$(resp_body "$R" | jq -r '.id // empty')"
eq "vendedor vende producto de su negocio → 201" 201 "$(http_code "$R")"
eq "la venta queda con el business_id correcto" "$BIZ1" "$(sql "SELECT business_id FROM sales WHERE id=${SID:-0}")"
eq "la venta guarda el user_id del vendedor" "$VEND_ID" "$(sql "SELECT user_id FROM sales WHERE id=${SID:-0}")"
eq "el stock bajó de 10 a 9" 9 "$(sql "SELECT stock FROM products WHERE id=${P1:-0}")"
eq "GET /sales/{id} desde otro negocio → 404" 404 "$(http_code "$(call "$ADMIN" "$BIZ2" GET "/api/v1/sales/${SID:-0}")")"
eq "GET /sales de otro negocio no incluye la venta" 0 \
  "$(resp_body "$(call "$ADMIN" "$BIZ2" GET /api/v1/sales)" | jq --argjson id "${SID:-0}" '[.data[]?|select(.id==$id)]|length' 2>/dev/null)"

echo "G. Pedidos WOO pendientes: aislamiento por negocio (sin webhook)"
WSID="$(sql "INSERT INTO sales (business_id,origin,pay_method,status,total,notes,created_at,updated_at) VALUES ($BIZ1,'WOO','WEB','PENDING_CONFIRM',0,'verif-$SUF',now(),now()) RETURNING id" | head -1)"
if ! [[ "$WSID" =~ ^[0-9]+$ ]]; then
  bad "no se pudo crear el pedido pendiente de prueba: $WSID"
else
  eq "/sales/pending del negocio 1 lista el pedido" 1 \
    "$(resp_body "$(call "$ADMIN" "$BIZ1" GET /api/v1/sales/pending)" | jq --argjson id "$WSID" '[.data[]?|select(.id==$id)]|length' 2>/dev/null)"
  eq "/sales/pending del negocio 2 NO lista el pedido" 0 \
    "$(resp_body "$(call "$ADMIN" "$BIZ2" GET /api/v1/sales/pending)" | jq --argjson id "$WSID" '[.data[]?|select(.id==$id)]|length' 2>/dev/null)"
  eq "rechazar desde OTRO negocio → 404" 404 \
    "$(http_code "$(call "$ADMIN" "$BIZ2" PATCH "/api/v1/sales/$WSID/confirm" '{"action":"reject","reason":"verif"}')")"
  eq "confirmar desde OTRO negocio → 404" 404 \
    "$(http_code "$(call "$ADMIN" "$BIZ2" PATCH "/api/v1/sales/$WSID/confirm" '{"action":"confirm"}')")"
  eq "el pedido sigue PENDING_CONFIRM tras los intentos ajenos" PENDING_CONFIRM "$(sql "SELECT status FROM sales WHERE id=$WSID")"
  eq "confirmar desde su negocio → 200" 200 \
    "$(http_code "$(call "$VEND" '' PATCH "/api/v1/sales/$WSID/confirm" '{"action":"confirm"}')")"
  eq "tras confirmar: COMPLETED con el user_id del vendedor" "COMPLETED|$VEND_ID" \
    "$(sql "SELECT status||'|'||coalesce(user_id::text,'null') FROM sales WHERE id=$WSID")"
fi

echo "H. Webhook WooCommerce (opcional, Fase 11)"
if [ -z "${WOO_WEBHOOK_SECRET:-}" ]; then
  echo "  ⏭  omitido: la integración WooCommerce está diferida (Fase 11)."
  echo "     Para probarla: WOO_WEBHOOK_SECRET=<secreto> bash scripts/verify_fase1.sh"
else
ORD="$SUF"
WOO_SECRET="${WOO_WEBHOOK_SECRET:-}"   # mismo valor que lee internal/webhook/woo_handler.go
[ -z "$WOO_SECRET" ] && echo "  ⚠️  WOO_WEBHOOK_SECRET vacío: el webhook se enviará sin firma (probable 401)"
WH() {
  local payload
  payload="{\"id\":$ORD,\"status\":\"processing\",\"total\":\"12.00\",\"billing\":{\"first_name\":\"Ana\",\"last_name\":\"Torres\",\"phone\":\"987654321\"},\"shipping\":{\"address_1\":\"Jr. Real 456\",\"city\":\"Huancayo\"},\"line_items\":[{\"id\":1,\"sku\":\"V1-$SUF\",\"name\":\"Verif V1\",\"quantity\":2,\"price\":\"6.00\"}]}"
  local sig=()
  if [ -n "$WOO_SECRET" ]; then
    sig=(-H "X-Wc-Webhook-Signature: $(printf '%s' "$payload" | openssl dgst -sha256 -hmac "$WOO_SECRET" -binary | base64)")
  fi
  curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/webhooks/woocommerce" -H "Content-Type: application/json" \
    -H "X-Wc-Webhook-Topic: order.created" -H "X-Wc-Webhook-Delivery-Id: $1-$SUF" \
    "${sig[@]}" -d "$payload"
}
echo "  webhook respondió HTTP $(WH a)"
sleep 2
eq "pedido WOO creado como PENDING_CONFIRM, sin usuario, en el negocio 1" "PENDING_CONFIRM|null|$BIZ1" \
  "$(sql "SELECT status||'|'||coalesce(user_id::text,'null')||'|'||business_id FROM sales WHERE external_id='$ORD'")"
eq "el pedido WOO NO descuenta stock todavía" 9 "$(sql "SELECT stock FROM products WHERE id=${P1:-0}")"
WH b >/dev/null; sleep 2
eq "reenvío con otro delivery-id no duplica la venta" 1 "$(sql "SELECT count(*) FROM sales WHERE external_id='$ORD'")"
WSID="$(sql "SELECT id FROM sales WHERE external_id='$ORD'")"

fi

echo
echo "Resultado: $PASS ok, $FAIL fallos"
[ "$FAIL" -eq 0 ] && echo "FASE 1 VERIFICADA ✅" || echo "Hay fallos: pégame la salida completa."
exit "$FAIL"