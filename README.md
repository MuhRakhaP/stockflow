# StockFlow

Small inventory + sales take-home (React + TS, Go, Postgres).

## Run

### Option A (dev)

1. Start DB (migrations + seed run on first init):
```powershell
docker compose up -d postgres
```

2. Backend:
```powershell
cd backend
go mod tidy
go run ./cmd/server
```
API: `http://localhost:8080` (`DATABASE_URL` and `PORT` can override defaults)

3. Frontend:
```powershell
cd frontend
npm install
npm run dev
```
UI: `http://localhost:5173` (Vite proxies `/api` to backend)

### Option B (docker compose)

```powershell
docker compose up -d --build
```
UI: `http://localhost:5173`, API: `http://localhost:8080`

Reset DB: `docker compose down -v`

## Tests

Backend (needs Postgres running + migrations applied):
```powershell
cd backend
$env:TEST_DATABASE_URL="postgres://stockflow:stockflow@localhost:5432/stockflow?sslmode=disable"
go test ./... -v
```
Includes the required cases:
- insufficient stock rejected and nothing written
- happy path writes movement + updates stock
- concurrency test for same SKU (`go test -race`)

Frontend:
```powershell
cd frontend
npm test
```

## API examples

```bash
curl "http://localhost:8080/api/products?search=kopi&filter=low&page=1&limit=10"
curl http://localhost:8080/api/products/summary

curl -X POST http://localhost:8080/api/products \
  -H "Content-Type: application/json" \
  -d '{"sku":"SKU-999","name":"Test Product","price":12000}'

curl -X POST http://localhost:8080/api/products/1/stock \
  -H "Content-Type: application/json" \
  -d '{"type":"out","qty":2,"note":"Damaged"}'

curl -X POST http://localhost:8080/api/orders \
  -H "Content-Type: application/json" \
  -d '{"customer":"PT Contoh","items":[{"sku":"SKU-001","qty":2},{"sku":"SKU-003","qty":1}]}'

curl "http://localhost:8080/api/orders?page=1&limit=10"
curl http://localhost:8080/api/orders/1
curl -X POST http://localhost:8080/api/orders/1/cancel
```

## Notes

- `last_movement_at` is derived from `stock_movements` (MAX created_at per product), kept simple with an index.
- Order stock decrement is atomic in one tx. I lock rows with `FOR UPDATE` and use guarded update `UPDATE ... SET stock = stock - $1 WHERE id=$2 AND stock >= $1`. If affected rows is 0, return 409 with sku+available. No select-then-update for stock checks.
- Prices/total are server-side only; client never sends unit price.
- Validation happens before writes (empty items, qty<=0, duplicate sku, unknown sku, etc).
- Added a small extra: paginated order history (`GET /api/orders?page=&limit=` + UI table). Spec only asked for order detail by ID, but history is handy for checking recent orders.

## If I had more time

- Better backend integration setup/CI for race tests
- Product create/edit UI
- Cleaner pagination/filter UX
- More frontend tests (component/integration)

## Time spent

Around 5-6 hours.