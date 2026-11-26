export interface Product {
  id: number;
  sku: string;
  name: string;
  price: number;
  stock: number;
  is_active: boolean;
  created_at: string;
  last_movement_at: string | null;
}

export interface StockMovement {
  id: number;
  product_id: number;
  type: 'in' | 'out';
  qty: number;
  note: string;
  created_at: string;
}

export interface SalesOrderItem {
  id: number;
  order_id: number;
  sku: string;
  qty: number;
  unit_price: number;
}

export interface SalesOrder {
  id: number;
  customer: string;
  status: 'CREATED' | 'CANCELLED';
  total: number;
  created_at: string;
  cancelled_at?: string | null;
  items?: SalesOrderItem[];
}

export interface ProductsResponse {
  data: Product[];
  page: number;
  limit: number;
  total: number;
}

export interface Summary {
  total_products: number;
  active_products: number;
  total_stock_value: number;
  low_stock_count: number;
}

export interface OrdersResponse {
  data: SalesOrder[];
  page: number;
  limit: number;
  total: number;
}

export interface ApiErrorBody {
  code: string;
  message?: string;
  available?: number;
  sku?: string;
  details?: unknown;
}

export interface ApiErrorEnvelope {
  error: ApiErrorBody;
}

export interface CreateProductPayload {
  sku: string;
  name: string;
  price: number;
  is_active?: boolean;
}

export interface StockAdjustPayload {
  type: 'in' | 'out';
  qty: number;
  note: string;
}

export interface CreateOrderPayload {
  customer: string;
  items: Array<{ sku: string; qty: number }>;
}
