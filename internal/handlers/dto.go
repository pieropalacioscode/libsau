package handlers

import "time"

type recentSaleRow struct {
	ID        uint
	Total     float64
	PayMethod string
	ItemCount int64
	Seller    string
	CreatedAt time.Time
}

type RecentSale struct {
	ID        uint      `json:"id"`
	Total     float64   `json:"total"`
	PayMethod string    `json:"pay_method"`
	ItemCount int64     `json:"item_count"`
	Seller    string    `json:"seller"`
	CreatedAt time.Time `json:"created_at"`
}
