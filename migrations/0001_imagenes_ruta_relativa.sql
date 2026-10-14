-- Pasa las imágenes de URL completa a ruta relativa a MEDIA_BASE_URL.
-- Idempotente: solo toca filas que todavía empiezan con el host indicado.
-- Uso (siempre ensayar primero con ROLLBACK):
--   psql -v ON_ERROR_STOP=1 -v host='https://img.docentesmart.com/' -v finish=ROLLBACK < 0001_imagenes_ruta_relativa.sql
--   psql -v ON_ERROR_STOP=1 -v host='https://img.docentesmart.com/' -v finish=COMMIT   < 0001_imagenes_ruta_relativa.sql
-- Revertir: UPDATE products SET image_url = 'https://img.docentesmart.com/' || image_url
--           WHERE image_url !~* '^https?://';  (idem businesses.logo_url)
BEGIN;

UPDATE products
   SET image_url = substr(image_url, length(:'host') + 1)
 WHERE starts_with(image_url, :'host')
   AND length(image_url) > length(:'host');

UPDATE businesses
   SET logo_url = substr(logo_url, length(:'host') + 1)
 WHERE starts_with(logo_url, :'host')
   AND length(logo_url) > length(:'host');

SELECT 'products' AS tabla,
       count(*) FILTER (WHERE image_url IS NULL) AS sin_imagen,
       count(*) FILTER (WHERE image_url ~* '^https?://') AS url_completa,
       count(*) FILTER (WHERE image_url IS NOT NULL AND image_url !~* '^https?://') AS ruta_relativa
  FROM products
UNION ALL
SELECT 'businesses',
       count(*) FILTER (WHERE logo_url IS NULL),
       count(*) FILTER (WHERE logo_url ~* '^https?://'),
       count(*) FILTER (WHERE logo_url IS NOT NULL AND logo_url !~* '^https?://')
  FROM businesses;

:finish;
