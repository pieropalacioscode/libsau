-- Requiere que la API ya haya arrancado una vez (AutoMigrate crea las columnas).
-- Simulación:  psql -v apply=0 < migrations/0002_slug_categorias_y_fechas.sql
-- Confirmar:   psql -v apply=1 < migrations/0002_slug_categorias_y_fechas.sql
-- Idempotente: solo toca filas sin slug o sin fecha.
\set ON_ERROR_STOP on
\if :{?apply}
\else
  \set apply 0
\endif

BEGIN;

WITH base AS (
  SELECT id, business_id,
         trim(both '-' from left(
           regexp_replace(
             translate(lower(trim(name)), 'áéíóúüñàèìòùâêîôûäëïöç', 'aeiouunaeiouaeiouaeioc'),
             '[^a-z0-9]+', '-', 'g'),
           150)) AS s
  FROM categories
  WHERE slug IS NULL OR slug = ''
),
fin AS (
  SELECT id, business_id, CASE WHEN s = '' THEN 'categoria' ELSE s END AS s2
  FROM base
),
ranked AS (
  SELECT id, s2, row_number() OVER (PARTITION BY business_id, s2 ORDER BY id) AS rn
  FROM fin
)
UPDATE categories c
SET slug = CASE WHEN r.rn = 1 THEN r.s2 ELSE r.s2 || '-' || r.rn END
FROM ranked r
WHERE c.id = r.id;

UPDATE products SET created_at = now() WHERE created_at IS NULL;
UPDATE products SET updated_at = now() WHERE updated_at IS NULL;

\echo '── categorías de libprep ──'
SELECT c.id, c.name, c.slug
FROM categories c JOIN businesses b ON b.id = c.business_id
WHERE b.slug = 'libprep' ORDER BY c.id;

\echo '── comprobaciones (todas deben ser 0) ──'
SELECT (SELECT count(*) FROM categories WHERE slug IS NULL OR slug = '') AS cat_sin_slug,
       (SELECT count(*) FROM products WHERE created_at IS NULL OR updated_at IS NULL) AS prod_sin_fecha;

\if :apply
  COMMIT;
  \echo 'CAMBIOS CONFIRMADOS (COMMIT)'
\else
  ROLLBACK;
  \echo 'SIMULACION: se hizo ROLLBACK. Usa -v apply=1 para confirmar.'
\endif
