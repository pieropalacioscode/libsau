-- scripts/backfill_slugs.sql
-- Rellena products.slug en los productos que aún no lo tienen.
-- Base del slug: nombre del archivo de la imagen (sin extensión); si no hay
-- imagen, el nombre del producto. Sin tildes, solo [a-z0-9-], máx. 150.
-- Si dos productos del mismo negocio dan igual, el segundo recibe -2, etc.
--
-- Es idempotente (solo toca filas con slug NULL o vacío) y corre en una
-- transacción. Por defecto SIMULA y termina con ROLLBACK:
--
--   psql ... -v apply=0 < scripts/backfill_slugs.sql     (simulación)
--   psql ... -v apply=1 < scripts/backfill_slugs.sql     (confirma los cambios)
--
-- Si algo choca con el índice único, ON_ERROR_STOP aborta y no queda nada a medias.

\set ON_ERROR_STOP on
\if :{?apply}
\else
  \set apply 0
\endif

BEGIN;

WITH base AS (
  SELECT id, business_id,
         CASE WHEN coalesce(image_url, '') <> ''
              THEN regexp_replace(
                     regexp_replace(
                       regexp_replace(image_url, '[?#].*$', ''),
                       '^.*/', ''),
                     '\.[A-Za-z0-9]+$', '')
              ELSE coalesce(name, '')
         END AS raw
  FROM products
  WHERE slug IS NULL OR slug = ''
),
clean AS (
  SELECT id, business_id,
         trim(both '-' from left(
           regexp_replace(
             translate(lower(raw), 'áéíóúüñàèìòùâêîôûäëïöç', 'aeiouunaeiouaeiouaeioc'),
             '[^a-z0-9]+', '-', 'g'),
           150)) AS s
  FROM base
),
fin AS (
  SELECT id, business_id,
         CASE WHEN s = ''            THEN 'producto'
              WHEN s ~ '^[0-9]+$'    THEN 'producto-' || s
              ELSE s
         END AS s2
  FROM clean
),
ranked AS (
  SELECT id, s2,
         row_number() OVER (PARTITION BY business_id, s2 ORDER BY id) AS rn
  FROM fin
)
UPDATE products p
SET slug = CASE WHEN r.rn = 1 THEN r.s2 ELSE r.s2 || '-' || r.rn END
FROM ranked r
WHERE p.id = r.id;

\echo '── resumen por negocio (productos = con_slug = slugs_distintos) ──'
SELECT b.slug AS negocio,
       count(*)               AS productos,
       count(p.slug)          AS con_slug,
       count(DISTINCT p.slug) AS slugs_distintos
FROM products p JOIN businesses b ON b.id = p.business_id
GROUP BY b.slug ORDER BY 1;

\echo '── muestra ──'
SELECT b.slug AS negocio, p.id, p.slug
FROM products p JOIN businesses b ON b.id = p.business_id
ORDER BY b.slug, p.id
LIMIT 15;

\echo '── a revisar: slugs que terminan en número o sin imagen ni nombre ──'
SELECT b.slug AS negocio, p.id, p.slug
FROM products p JOIN businesses b ON b.id = p.business_id
WHERE p.slug ~ '-[0-9]+$' OR p.slug = 'producto'
ORDER BY b.slug, p.id;

\if :apply
  COMMIT;
  \echo 'CAMBIOS CONFIRMADOS (COMMIT)'
\else
  ROLLBACK;
  \echo 'SIMULACION: se hizo ROLLBACK, no se guardó nada. Usa -v apply=1 para confirmar.'
\endif