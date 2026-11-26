package model

import "time"

type Product struct {
	ID             int        `json:"id"`
	SKU            string     `json:"sku"`
	Name           string     `json:"name"`
	Price          int        `json:"price"`
	Stock          int        `json:"stock"`
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	LastMovementAt *time.Time `json:"last_movement_at"`
}

type StockMovement struct {
	ID        int       `json:"id"`
	ProductID int       `json:"product_id"`
	Type      string    `json:"type"`
	Qty       int       `json:"qty"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

type SalesOrder struct {
	ID          int              `json:"id"`
	Customer    string           `json:"customer"`
	Status      string           `json:"status"`
	Total       int              `json:"total"`
	CreatedAt   time.Time        `json:"created_at"`
	CancelledAt *time.Time       `json:"cancelled_at,omitempty"`
	Items       []SalesOrderItem `json:"items,omitempty"`
}

type SalesOrderItem struct {
	ID        int    `json:"id"`
	OrderID   int    `json:"order_id"`
	SKU       string `json:"sku"`
	Qty       int    `json:"qty"`
	UnitPrice int    `json:"unit_price"`
}

type CreateProductRequest struct {
	SKU      string `json:"sku"`
	Name     string `json:"name"`
	Price    *int   `json:"price"`
	IsActive *bool  `json:"is_active"`
}

type UpdateProductRequest struct {
	SKU      *string `json:"sku"`
	Name     *string `json:"name"`
	Price    *int    `json:"price"`
	IsActive *bool   `json:"is_active"`
}

type StockAdjustRequest struct {
	Type string `json:"type"`
	Qty  *int   `json:"qty"`
	Note string `json:"note"`
}

type CreateOrderRequest struct {
	Customer string             `json:"customer"`
	Items    []CreateOrderItem  `json:"items"`
}

type CreateOrderItem struct {
	SKU string `json:"sku"`
	Qty *int   `json:"qty"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message,omitempty"`
	Available *int   `json:"available,omitempty"`
	SKU       string `json:"sku,omitempty"`
	Details   any    `json:"details,omitempty"`
}

type ProductsResponse struct {
	Data  []Product `json:"data"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
	Total int       `json:"total"`
}

type Summary struct {
	TotalProducts   int `json:"total_products"`
	ActiveProducts  int `json:"active_products"`
	TotalStockValue int `json:"total_stock_value"`
	LowStockCount   int `json:"low_stock_count"`
}

type OrdersResponse struct {
	Data  []SalesOrder `json:"data"`
	Page  int          `json:"page"`
	Limit int          `json:"limit"`
	Total int          `json:"total"`
}
