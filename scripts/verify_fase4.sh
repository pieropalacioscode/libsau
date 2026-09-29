#!/usr/bin/env bash
# Verificación de la Fase 4: importador desde el Excel oficial de onboarding.
# No necesita el servidor HTTP corriendo — go run cmd/import/main.go habla
# directo con Postgres.
# Uso:
#   FILE=./prueba-libros.xlsx bash scripts/verify_fase4.sh
#   FILE=./catalogo_libros_completado_-_copia.xlsx SHEET=Alfaguara bash scripts/verify_fase4.sh
set -u

FILE="${FILE:-./prueba-libros.xlsx}"
SHEET="${SHEET:-}"
BIZ_SLUG="${BIZ_SLUG:-libreria-saber}"
PSQL=(docker exec -i libsau_postgres psql -U libsau_user -d libsau_db -tA -v ON_ERROR_STOP=1)
PASS=0; FAIL=0

ok()  { echo "  ✅ $1"; PASS=$((PASS+1)); }
bad() { echo "  ❌ $1"; FAIL=$((FAIL+1)); }
eq()  { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 — esperado: [$2] obtenido: [$3]"; fi; }
sql() { "${PSQL[@]}" -c "$1" 2>&1 | tr -d '\r'; }

sheet_arg=()
[ -n "$SHEET" ] && sheet_arg=(--sheet="$SHEET")

if [ ! -f "$FILE" ]; then
  echo "❌ No se encontró $FILE. Pasa la ruta real: FILE=./ruta/archivo.xlsx bash scripts/verify_fase4.sh"
  exit 1
fi

echo "0. Compila"
go build ./... || { echo "❌ go build falló"; exit 1; }
ok "go build ./... sin errores"

BIZ_ID="$(sql "SELECT id FROM businesses WHERE slug='$BIZ_SLUG'")"
[[ "$BIZ_ID" =~ ^[0-9]+$ ]] || { echo "❌ El negocio '$BIZ_SLUG' no existe. Ajusta BIZ_SLUG."; exit 1; }

BEFORE="$(sql "SELECT count(*) FROM products WHERE business_id=$BIZ_ID")"

echo "A. Negocio inexistente no crea nada"
OUT="$(go run cmd/import/main.go --file="$FILE" --business=negocio-que-no-existe-$(date +%s) "${sheet_arg[@]}" 2>&1)"
echo "$OUT" | grep -qi "no existe" && ok "el programa se detiene con error claro" || bad "el programa no reportó el negocio inexistente: $OUT"
AFTER_BAD="$(sql "SELECT count(*) FROM products WHERE business_id=$BIZ_ID")"
eq "no se crearon productos huérfanos" "$BEFORE" "$AFTER_BAD"

echo "B. --dry-run no escribe nada"
OUTDRY="$(go run cmd/import/main.go --file="$FILE" --business="$BIZ_SLUG" "${sheet_arg[@]}" --dry-run 2>&1)"
echo "$OUTDRY"
AFTER_DRY="$(sql "SELECT count(*) FROM products WHERE business_id=$BIZ_ID")"
eq "el conteo de productos no cambia con --dry-run" "$BEFORE" "$AFTER_DRY"
echo "$OUTDRY" | grep -q "no se escribe nada" && ok "el aviso de dry-run aparece" || bad "no se vio el aviso de --dry-run"

echo "C. Primera corrida real — carga real"
OUT1="$(go run cmd/import/main.go --file="$FILE" --business="$BIZ_SLUG" "${sheet_arg[@]}" 2>&1)"
echo "$OUT1"
AFTER1="$(sql "SELECT count(*) FROM products WHERE business_id=$BIZ_ID")"
[ "$AFTER1" -gt "$BEFORE" ] && ok "la primera corrida agregó productos ($BEFORE → $AFTER1)" \
  || bad "la primera corrida no agregó productos ($BEFORE → $AFTER1) — revisa la salida arriba"

echo "D. Segunda corrida con el mismo archivo — no duplica"
OUT2="$(go run cmd/import/main.go --file="$FILE" --business="$BIZ_SLUG" "${sheet_arg[@]}" 2>&1)"
AFTER2="$(sql "SELECT count(*) FROM products WHERE business_id=$BIZ_ID")"
eq "el conteo no cambia al reimportar el mismo archivo" "$AFTER1" "$AFTER2"
echo "$OUT2" | grep -q " 0 actualizados/a actualizar\| 0 creados" > /dev/null 2>&1
echo "$OUT2" | grep -qE "\b0 creados\b" && ok "la segunda corrida reporta 0 creados" || bad "la segunda corrida no reportó 0 creados: $OUT2"

echo "E. Categorías quedaron limpias, no texto crudo del Excel"
DIRTY="$(sql "SELECT count(*) FROM categories WHERE business_id=$BIZ_ID AND (name LIKE '%>%' OR name = '')")"
eq "ninguna categoría trae '>' sin resolver ni queda vacía" "0" "$DIRTY"

echo "F. Todo quedó bajo el negocio correcto"
ORPHANS="$(sql "SELECT count(*) FROM products p LEFT JOIN categories c ON c.id=p.category_id WHERE p.business_id=$BIZ_ID AND (c.business_id IS NULL OR c.business_id<>$BIZ_ID)")"
eq "ningún producto quedó con categoría de otro negocio" "0" "$ORPHANS"

echo
echo "Resultado: $PASS ok, $FAIL fallos"
[ "$FAIL" -eq 0 ] && echo "FASE 4 VERIFICADA ✅" || echo "Hay fallos: pégame la salida completa junto con internal/database (cómo se conecta *gorm.DB)."
exit "$FAIL"