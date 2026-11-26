package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"stockflow/backend/internal/model"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return New(pool)
}

func seedProduct(t *testing.T, s *Store, sku string, stock int) model.Product {
	t.Helper()
	price := 10000
	uniq := fmt.Sprintf("%s-%d", sku, time.Now().UnixNano())
	p, err := s.CreateProduct(context.Background(), model.CreateProductRequest{
		SKU:   uniq,
		Name:  "Test " + sku,
		Price: &price,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	if stock > 0 {
		qty := stock
		_, _, err := s.AdjustStock(context.Background(), p.ID, model.StockAdjustRequest{Type: "in", Qty: &qty, Note: "seed"})
		if err != nil {
			t.Fatalf("seed stock: %v", err)
		}
		p, _ = s.GetProduct(context.Background(), p.ID)
	}
	return p
}

func TestAdjustStock_InsufficientStockRejectedNoWrite(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := seedProduct(t, s, "TST-NOWRITE-1", 2)

	qty := 5
	_, _, err := s.AdjustStock(ctx, p.ID, model.StockAdjustRequest{Type: "out", Qty: &qty, Note: "should fail"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ise *InsufficientStockError
	if !AsInsufficient(err, &ise) {
		t.Fatalf("expected InsufficientStockError, got %v", err)
	}
	if ise.Available != 2 {
		t.Fatalf("expected available 2, got %d", ise.Available)
	}

	after, err := s.GetProduct(ctx, p.ID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if after.Stock != 2 {
		t.Fatalf("stock changed unexpectedly: got %d", after.Stock)
	}
	moves, err := s.ListMovements(ctx, p.ID)
	if err != nil {
		t.Fatalf("list movements: %v", err)
	}
	for _, m := range moves {
		if m.Note == "should fail" {
			t.Fatalf("failed movement was written")
		}
	}
}

func TestAdjustStock_HappyPathWritesMovementAndUpdatesStock(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := seedProduct(t, s, "TST-HAPPY-1", 10)

	qty := 3
	updated, mv, err := s.AdjustStock(ctx, p.ID, model.StockAdjustRequest{Type: "out", Qty: &qty, Note: "sold"})
	if err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if updated.Stock != 7 {
		t.Fatalf("expected stock 7, got %d", updated.Stock)
	}
	if mv.Type != "out" || mv.Qty != 3 {
		t.Fatalf("unexpected movement: %+v", mv)
	}
	after, _ := s.GetProduct(ctx, p.ID)
	if after.Stock != 7 {
		t.Fatalf("persisted stock mismatch: %d", after.Stock)
	}
}

func AsInsufficient(err error, target **InsufficientStockError) bool {
	if e, ok := err.(*InsufficientStockError); ok {
		*target = e
		return true
	}
	return false
}

func TestCreateOrder_ConcurrentNeverNegative(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := seedProduct(t, s, "TST-CONC-1", 10)

	const workers = 20
	qty := 1
	var wg sync.WaitGroup
	successes := 0
	var mu sync.Mutex

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.CreateOrder(ctx, model.CreateOrderRequest{
				Customer: fmt.Sprintf("C-%d", i),
				Items:    []model.CreateOrderItem{{SKU: p.SKU, Qty: &qty}},
			})
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
				return
			}
			if _, ok := err.(*InsufficientStockError); ok {
				return
			}
			t.Errorf("unexpected error: %v", err)
		}(i)
	}
	wg.Wait()

	after, err := s.GetProduct(ctx, p.ID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if after.Stock < 0 {
		t.Fatalf("stock went negative: %d", after.Stock)
	}
	if successes > 10 {
		t.Fatalf("too many successes: %d", successes)
	}
	if after.Stock != 10-successes {
		t.Fatalf("stock mismatch: got %d want %d", after.Stock, 10-successes)
	}
}
