INSERT INTO products (sku, name, price, stock, is_active) VALUES
('SKU-001', 'Kopi Arabica 250g', 85000, 40, TRUE),
('SKU-002', 'Kopi Robusta 250g', 65000, 25, TRUE),
('SKU-003', 'Teh Hijau 100g', 45000, 60, TRUE),
('SKU-004', 'Gula Aren 500g', 38000, 8, TRUE),
('SKU-005', 'Susu Oat 1L', 52000, 15, TRUE),
('SKU-006', 'Cokelat Bubuk 200g', 72000, 5, TRUE),
('SKU-007', 'Madu Hutan 350ml', 98000, 12, TRUE),
('SKU-008', 'Biskuit Gandum', 28000, 100, TRUE),
('SKU-009', 'Air Mineral 600ml', 6000, 200, TRUE),
('SKU-010', 'Sirup Vanilla 750ml', 56000, 3, TRUE),
('SKU-011', 'Matcha Powder 100g', 120000, 0, TRUE),
('SKU-012', 'Tumbler Stainless', 150000, 18, FALSE)
ON CONFLICT (sku) DO NOTHING;

-- Sample movements
INSERT INTO stock_movements (product_id, type, qty, note, created_at)
SELECT p.id, 'in', 40, 'Initial stock', now() - interval '2 days'
FROM products p WHERE p.sku = 'SKU-001'
AND NOT EXISTS (SELECT 1 FROM stock_movements sm WHERE sm.product_id = p.id AND sm.note = 'Initial stock');

INSERT INTO stock_movements (product_id, type, qty, note, created_at)
SELECT p.id, 'out', 5, 'Sample adjustment', now() - interval '2 hours'
FROM products p WHERE p.sku = 'SKU-002'
AND NOT EXISTS (SELECT 1 FROM stock_movements sm WHERE sm.product_id = p.id AND sm.note = 'Sample adjustment');

-- Sample orders
DO $$
DECLARE
  o1 INT;
  o2 INT;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM sales_orders WHERE customer = 'PT Kopi Pagi') THEN
    INSERT INTO sales_orders (customer, status, total, created_at)
    VALUES ('PT Kopi Pagi', 'CREATED', 235000, now() - interval '1 day')
    RETURNING id INTO o1;
    INSERT INTO sales_order_items (order_id, sku, qty, unit_price) VALUES
      (o1, 'SKU-001', 2, 85000),
      (o1, 'SKU-003', 1, 45000),
      (o1, 'SKU-008', 1, 20000);
    -- fix total to match items
    UPDATE sales_orders SET total = (SELECT COALESCE(SUM(qty*unit_price),0) FROM sales_order_items WHERE order_id=o1) WHERE id=o1;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM sales_orders WHERE customer = 'Cafe Senja') THEN
    INSERT INTO sales_orders (customer, status, total, created_at)
    VALUES ('Cafe Senja', 'CANCELLED', 150000, now() - interval '3 days')
    RETURNING id INTO o2;
    INSERT INTO sales_order_items (order_id, sku, qty, unit_price) VALUES
      (o2, 'SKU-005', 2, 52000),
      (o2, 'SKU-009', 8, 5750);
    UPDATE sales_orders SET total = (SELECT COALESCE(SUM(qty*unit_price),0) FROM sales_order_items WHERE order_id=o2), cancelled_at = now() - interval '2 days' WHERE id=o2;
  END IF;
END $$;
