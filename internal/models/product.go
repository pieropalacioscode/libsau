// models/product.go
package models

type Category struct {
	ID         uint     `gorm:"primaryKey" json:"id"`
	BusinessID uint     `gorm:"not null;index;uniqueIndex:uq_category_business_name" json:"business_id"`
	Business   Business `gorm:"foreignKey:BusinessID" json:"-"`
	Name       string   `gorm:"uniqueIndex:uq_category_business_name;not null" json:"name"`
	Active     bool     `json:"active"`
}

type Product struct {
	ID         uint     `gorm:"primaryKey" json:"id"`
	BusinessID uint     `gorm:"not null;index;uniqueIndex:uq_product_business_sku;uniqueIndex:uq_product_business_isbn,priority:1" json:"business_id"`
	Business   Business `gorm:"foreignKey:BusinessID" json:"-"`
	Name       string   `json:"name"`
	SKU        string   `gorm:"uniqueIndex:uq_product_business_sku" json:"sku"`
	// ISBN es el mismo concepto que GTIN/UPC/EAN en el Excel de onboarding:
	// un identificador de barras universal. nil cuando el producto no lo
	// trae (papelería, D'Nieve, autopublicados sin código). Único por
	// negocio — pero NULL no choca contra NULL en Postgres, así que no
	// molesta a los negocios que nunca lo usan.
	ISBN *string `gorm:"type:varchar(20);uniqueIndex:uq_product_business_isbn,priority:2" json:"isbn,omitempty"`
	// ImageURL es la URL completa de la foto (Cloudflare en la Fase 9).
	// nil cuando el producto todavía no tiene foto.
	ImageURL    *string  `gorm:"type:varchar(500)" json:"image_url,omitempty"`
	Description *string  `gorm:"type:text" json:"description,omitempty"`
	Price       float64  `gorm:"type:numeric(10,2);not null;default:0" json:"price"`
	Stock       int      `json:"stock"`
	CategoryID  uint     `json:"category_id"`
	Category    Category `gorm:"foreignKey:CategoryID" json:"category"`
	Cost        float64  `gorm:"type:numeric(10,2);not null;default:0" json:"cost"`
	Active      bool     `gorm:"default:true" json:"active"`
}

// ProductAttribute guarda los atributos libres del Excel (Autor, Editorial,
// Año de edición, Tamaño, Tipo de Papel, etc.) como pares nombre/valor en vez
// de columnas fijas. El nombre de cada uno de los 16 bloques del Excel lo
// define quien arma el catálogo y varía entre un negocio de libros y uno de
// papelería o de artículos de Navidad — una columna fija por atributo no
// sirve para un catálogo multi-rubro.
//
// Esta es la tabla que LIBSAU_v5_02_modelo_de_datos.md dejó anotada como
// "diferida hasta que se justifique con el catálogo actual". Se activa ahora
// porque la Fase 4 mostró que esta información ya se estaba perdiendo en
// cada import.
type ProductAttribute struct {
	ID        uint    `gorm:"primaryKey" json:"id"`
	ProductID uint    `gorm:"not null;index;uniqueIndex:uq_product_attribute_name" json:"product_id"`
	Product   Product `gorm:"foreignKey:ProductID" json:"-"`
	Name      string  `gorm:"type:varchar(100);uniqueIndex:uq_product_attribute_name" json:"name"`
	Value     string  `gorm:"type:text" json:"value"`
}

func (ProductAttribute) TableName() string { return "product_attributes" }
