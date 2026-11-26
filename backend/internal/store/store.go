package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"stockflow/backend/internal/model"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrProductHasHistory = errors.New("product has history")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrAlreadyCancelled  = errors.New("already cancelled")
)

type InsufficientStockError struct {
	SKU       string
	Available int
}

func (e *InsufficientStockError) Error() string { return ErrInsufficientStock.Error() }

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) ListProducts(ctx context.Context, search, filter string, page, limit int) (model.ProductsResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	conds := []string{}
	args := []any{}
	if strings.TrimSpace(search) != "" {
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(search))+"%")
		n := len(args)
		conds = append(conds, fmt.Sprintf("(LOWER(p.sku) LIKE $%d OR LOWER(p.name) LIKE $%d)", n, n))
	}
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "active":
		conds = append(conds, "p.is_active = TRUE")
	case "inactive":
		conds = append(conds, "p.is_active = FALSE")
	case "low", "low_stock", "low-stock":
		conds = append(conds, "p.stock <= 10")
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	countQ := fmt.Sprintf("SELECT COUNT(*) FROM products p %s", where)
	var total int
	if err := s.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return model.ProductsResponse{}, err
	}

	argsWithPage := append([]any{}, args...)
	argsWithPage = append(argsWithPage, limit, offset)
	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2

	q := fmt.Sprintf(`
		SELECT p.id, p.sku, p.name, p.price, p.stock, p.is_active, p.created_at,
		       (SELECT MAX(sm.created_at) FROM stock_movements sm WHERE sm.product_id = p.id) AS last_movement_at
		FROM products p
		%s
		ORDER BY p.id DESC
		LIMIT $%d OFFSET $%d`, where, limitIdx, offsetIdx)

	rows, err := s.pool.Query(ctx, q, argsWithPage...)
	if err != nil {
		return model.ProductsResponse{}, err
	}
	defer rows.Close()

	data := []model.Product{}
	for rows.Next() {
		var p model.Product
		if err := rows.Scan(&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &p.IsActive, &p.CreatedAt, &p.LastMovementAt); err != nil {
			return model.ProductsResponse{}, err
		}
		data = append(data, p)
	}
	if err := rows.Err(); err != nil {
		return model.ProductsResponse{}, err
	}

	return model.ProductsResponse{Data: data, Page: page, Limit: limit, Total: total}, nil
}

func (s *Store) Summary(ctx context.Context) (model.Summary, error) {
	var sum model.Summary
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE is_active)::int,
			COALESCE(SUM(price * stock),0)::int,
			COUNT(*) FILTER (WHERE stock <= 10)::int
		FROM products
	`).Scan(&sum.TotalProducts, &sum.ActiveProducts, &sum.TotalStockValue, &sum.LowStockCount)
	return sum, err
}

func (s *Store) CreateProduct(ctx context.Context, req model.CreateProductRequest) (model.Product, error) {
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	var p model.Product
	err := s.pool.QueryRow(ctx, `
		INSERT INTO products (sku, name, price, is_active)
		VALUES ($1,$2,$3,$4)
		RETURNING id, sku, name, price, stock, is_active, created_at, NULL::timestamptz
	`, strings.TrimSpace(req.SKU), strings.TrimSpace(req.Name), *req.Price, isActive).
		Scan(&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &p.IsActive, &p.CreatedAt, &p.LastMovementAt)
	return p, err
}

func (s *Store) UpdateProduct(ctx context.Context, id int, req model.UpdateProductRequest) (model.Product, error) {
	sets := []string{}
	args := []any{}
	add := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	if req.SKU != nil {
		add("sku", strings.TrimSpace(*req.SKU))
	}
	if req.Name != nil {
		add("name", strings.TrimSpace(*req.Name))
	}
	if req.Price != nil {
		add("price", *req.Price)
	}
	if req.IsActive != nil {
		add("is_active", *req.IsActive)
	}
	if len(sets) == 0 {
		return s.GetProduct(ctx, id)
	}
	args = append(args, id)
	q := fmt.Sprintf(`UPDATE products SET %s WHERE id=$%d RETURNING id, sku, name, price, stock, is_active, created_at,
		(SELECT MAX(sm.created_at) FROM stock_movements sm WHERE sm.product_id = products.id)`, strings.Join(sets, ","), len(args))
	var p model.Product
	err := s.pool.QueryRow(ctx, q, args...).Scan(&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &p.IsActive, &p.CreatedAt, &p.LastMovementAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Product{}, ErrNotFound
	}
	return p, err
}

func (s *Store) GetProduct(ctx context.Context, id int) (model.Product, error) {
	var p model.Product
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, p.sku, p.name, p.price, p.stock, p.is_active, p.created_at,
		       (SELECT MAX(sm.created_at) FROM stock_movements sm WHERE sm.product_id = p.id)
		FROM products p WHERE p.id=$1`, id).
		Scan(&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &p.IsActive, &p.CreatedAt, &p.LastMovementAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Product{}, ErrNotFound
	}
	return p, err
}

func (s *Store) DeleteProduct(ctx context.Context, id int) error {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1)`, id).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	var hasHistory bool
	err = s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM stock_movements WHERE product_id=$1)
		    OR EXISTS(SELECT 1 FROM sales_order_items soi JOIN products p ON p.sku = soi.sku WHERE p.id=$1)
	`, id).Scan(&hasHistory)
	if err != nil {
		return err
	}
	if hasHistory {
		return ErrProductHasHistory
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM products WHERE id=$1`, id)
	return err
}

func (s *Store) AdjustStock(ctx context.Context, productID int, req model.StockAdjustRequest) (model.Product, model.StockMovement, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Product{}, model.StockMovement{}, err
	}
	defer tx.Rollback(ctx)

	var p model.Product
	err = tx.QueryRow(ctx, `SELECT id, sku, name, price, stock, is_active, created_at FROM products WHERE id=$1 FOR UPDATE`, productID).
		Scan(&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &p.IsActive, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Product{}, model.StockMovement{}, ErrNotFound
	}
	if err != nil {
		return model.Product{}, model.StockMovement{}, err
	}

	qty := *req.Qty
	if req.Type == "out" {
		tag, err := tx.Exec(ctx, `UPDATE products SET stock = stock - $1 WHERE id=$2 AND stock >= $1`, qty, productID)
		if err != nil {
			return model.Product{}, model.StockMovement{}, err
		}
		if tag.RowsAffected() == 0 {
			return model.Product{}, model.StockMovement{}, &InsufficientStockError{SKU: p.SKU, Available: p.Stock}
		}
		p.Stock -= qty
	} else {
		_, err := tx.Exec(ctx, `UPDATE products SET stock = stock + $1 WHERE id=$2`, qty, productID)
		if err != nil {
			return model.Product{}, model.StockMovement{}, err
		}
		p.Stock += qty
	}

	var m model.StockMovement
	note := strings.TrimSpace(req.Note)
	err = tx.QueryRow(ctx, `
		INSERT INTO stock_movements (product_id, type, qty, note)
		VALUES ($1,$2,$3,$4)
		RETURNING id, product_id, type, qty, COALESCE(note,''), created_at
	`, productID, req.Type, qty, note).Scan(&m.ID, &m.ProductID, &m.Type, &m.Qty, &m.Note, &m.CreatedAt)
	if err != nil {
		return model.Product{}, model.StockMovement{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Product{}, model.StockMovement{}, err
	}
	p.LastMovementAt = &m.CreatedAt
	return p, m, nil
}

func (s *Store) ListMovements(ctx context.Context, productID int) ([]model.StockMovement, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1)`, productID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, product_id, type, qty, COALESCE(note,''), created_at
		FROM stock_movements WHERE product_id=$1 ORDER BY created_at DESC, id DESC
	`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.StockMovement{}
	for rows.Next() {
		var m model.StockMovement
		if err := rows.Scan(&m.ID, &m.ProductID, &m.Type, &m.Qty, &m.Note, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type orderItemResolved struct {
	SKU       string
	Qty       int
	UnitPrice int
	ProductID int
}

func (s *Store) CreateOrder(ctx context.Context, req model.CreateOrderRequest) (model.SalesOrder, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.SalesOrder{}, err
	}
	defer tx.Rollback(ctx)

	skus := make([]string, 0, len(req.Items))
	for _, it := range req.Items {
		skus = append(skus, it.SKU)
	}

	rows, err := tx.Query(ctx, `SELECT id, sku, price, stock FROM products WHERE sku = ANY($1) FOR UPDATE`, skus)
	if err != nil {
		return model.SalesOrder{}, err
	}
	type prodInfo struct {
		ID    int
		SKU   string
		Price int
		Stock int
	}
	found := map[string]prodInfo{}
	for rows.Next() {
		var p prodInfo
		if err := rows.Scan(&p.ID, &p.SKU, &p.Price, &p.Stock); err != nil {
			rows.Close()
			return model.SalesOrder{}, err
		}
		found[p.SKU] = p
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return model.SalesOrder{}, err
	}

	resolved := make([]orderItemResolved, 0, len(req.Items))
	total := 0
	for _, it := range req.Items {
		p, ok := found[it.SKU]
		if !ok {
			return model.SalesOrder{}, fmt.Errorf("unknown sku: %s", it.SKU)
		}
		qty := *it.Qty
		resolved = append(resolved, orderItemResolved{SKU: p.SKU, Qty: qty, UnitPrice: p.Price, ProductID: p.ID})
		total += p.Price * qty
	}

	sort.Slice(resolved, func(i, j int) bool { return resolved[i].ProductID < resolved[j].ProductID })

	for _, it := range resolved {
		tag, err := tx.Exec(ctx, `
			UPDATE products SET stock = stock - $1
			WHERE id = $2 AND stock >= $1
		`, it.Qty, it.ProductID)
		if err != nil {
			return model.SalesOrder{}, err
		}
		if tag.RowsAffected() == 0 {
			var available int
			_ = tx.QueryRow(ctx, `SELECT stock FROM products WHERE id=$1`, it.ProductID).Scan(&available)
			return model.SalesOrder{}, &InsufficientStockError{SKU: it.SKU, Available: available}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO stock_movements (product_id, type, qty, note)
			VALUES ($1,'out',$2,$3)
		`, it.ProductID, it.Qty, "Order sale")
		if err != nil {
			return model.SalesOrder{}, err
		}
	}

	var order model.SalesOrder
	err = tx.QueryRow(ctx, `
		INSERT INTO sales_orders (customer, status, total)
		VALUES ($1,'CREATED',$2)
		RETURNING id, customer, status, total, created_at, cancelled_at
	`, strings.TrimSpace(req.Customer), total).Scan(&order.ID, &order.Customer, &order.Status, &order.Total, &order.CreatedAt, &order.CancelledAt)
	if err != nil {
		return model.SalesOrder{}, err
	}

	order.Items = make([]model.SalesOrderItem, 0, len(resolved))
	for _, it := range resolved {
		var oi model.SalesOrderItem
		err = tx.QueryRow(ctx, `
			INSERT INTO sales_order_items (order_id, sku, qty, unit_price)
			VALUES ($1,$2,$3,$4)
			RETURNING id, order_id, sku, qty, unit_price
		`, order.ID, it.SKU, it.Qty, it.UnitPrice).Scan(&oi.ID, &oi.OrderID, &oi.SKU, &oi.Qty, &oi.UnitPrice)
		if err != nil {
			return model.SalesOrder{}, err
		}
		order.Items = append(order.Items, oi)
	}

	if err := tx.Commit(ctx); err != nil {
		return model.SalesOrder{}, err
	}
	return order, nil
}

func (s *Store) ListOrders(ctx context.Context, page, limit int) (model.OrdersResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sales_orders`).Scan(&total); err != nil {
		return model.OrdersResponse{}, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, customer, status, total, created_at, cancelled_at
		FROM sales_orders
		ORDER BY id DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return model.OrdersResponse{}, err
	}
	defer rows.Close()

	orders := []model.SalesOrder{}
	ids := []int{}
	for rows.Next() {
		var o model.SalesOrder
		if err := rows.Scan(&o.ID, &o.Customer, &o.Status, &o.Total, &o.CreatedAt, &o.CancelledAt); err != nil {
			return model.OrdersResponse{}, err
		}
		o.Items = []model.SalesOrderItem{}
		ids = append(ids, o.ID)
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return model.OrdersResponse{}, err
	}
	if len(ids) > 0 {
		itemRows, err := s.pool.Query(ctx, `
			SELECT id, order_id, sku, qty, unit_price
			FROM sales_order_items
			WHERE order_id = ANY($1)
			ORDER BY id
		`, ids)
		if err != nil {
			return model.OrdersResponse{}, err
		}
		defer itemRows.Close()
		itemsByOrder := map[int][]model.SalesOrderItem{}
		for itemRows.Next() {
			var it model.SalesOrderItem
			if err := itemRows.Scan(&it.ID, &it.OrderID, &it.SKU, &it.Qty, &it.UnitPrice); err != nil {
				return model.OrdersResponse{}, err
			}
			itemsByOrder[it.OrderID] = append(itemsByOrder[it.OrderID], it)
		}
		if err := itemRows.Err(); err != nil {
			return model.OrdersResponse{}, err
		}
		for i := range orders {
			if items, ok := itemsByOrder[orders[i].ID]; ok {
				orders[i].Items = items
			}
		}
	}
	return model.OrdersResponse{Data: orders, Page: page, Limit: limit, Total: total}, nil
}

func (s *Store) GetOrder(ctx context.Context, id int) (model.SalesOrder, error) {
	var o model.SalesOrder
	err := s.pool.QueryRow(ctx, `
		SELECT id, customer, status, total, created_at, cancelled_at
		FROM sales_orders WHERE id=$1
	`, id).Scan(&o.ID, &o.Customer, &o.Status, &o.Total, &o.CreatedAt, &o.CancelledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.SalesOrder{}, ErrNotFound
	}
	if err != nil {
		return model.SalesOrder{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, order_id, sku, qty, unit_price FROM sales_order_items WHERE order_id=$1 ORDER BY id
	`, id)
	if err != nil {
		return model.SalesOrder{}, err
	}
	defer rows.Close()
	o.Items = []model.SalesOrderItem{}
	for rows.Next() {
		var it model.SalesOrderItem
		if err := rows.Scan(&it.ID, &it.OrderID, &it.SKU, &it.Qty, &it.UnitPrice); err != nil {
			return model.SalesOrder{}, err
		}
		o.Items = append(o.Items, it)
	}
	return o, rows.Err()
}

func (s *Store) CancelOrder(ctx context.Context, id int) (model.SalesOrder, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.SalesOrder{}, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM sales_orders WHERE id=$1 FOR UPDATE`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.SalesOrder{}, ErrNotFound
	}
	if err != nil {
		return model.SalesOrder{}, err
	}
	if status == "CANCELLED" {
		return model.SalesOrder{}, ErrAlreadyCancelled
	}

	rows, err := tx.Query(ctx, `SELECT sku, qty FROM sales_order_items WHERE order_id=$1`, id)
	if err != nil {
		return model.SalesOrder{}, err
	}
	type item struct{ SKU string; Qty int }
	items := []item{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.SKU, &it.Qty); err != nil {
			rows.Close()
			return model.SalesOrder{}, err
		}
		items = append(items, it)
	}
	rows.Close()

	for _, it := range items {
		var pid int
		err = tx.QueryRow(ctx, `SELECT id FROM products WHERE sku=$1 FOR UPDATE`, it.SKU).Scan(&pid)
		if err != nil {
			return model.SalesOrder{}, err
		}
		_, err = tx.Exec(ctx, `UPDATE products SET stock = stock + $1 WHERE id=$2`, it.Qty, pid)
		if err != nil {
			return model.SalesOrder{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO stock_movements (product_id, type, qty, note) VALUES ($1,'in',$2,$3)`, pid, it.Qty, fmt.Sprintf("Order #%d cancelled", id))
		if err != nil {
			return model.SalesOrder{}, err
		}
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `UPDATE sales_orders SET status='CANCELLED', cancelled_at=$1 WHERE id=$2`, now, id)
	if err != nil {
		return model.SalesOrder{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.SalesOrder{}, err
	}
	return s.GetOrder(ctx, id)
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
