import type {
  ApiErrorEnvelope,
  CreateOrderPayload,
  CreateProductPayload,
  Product,
  ProductsResponse,
  SalesOrder,
  StockAdjustPayload,
  StockMovement,
} from './types';

export class ApiError extends Error {
  status: number;
  code: string;
  details?: unknown;
  available?: number;
  sku?: string;

  constructor(status: number, code: string, message: string, details?: unknown, available?: number, sku?: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.details = details;
    this.available = available;
    this.sku = sku;
  }
}

async function parseError(res: Response): Promise<ApiError> {
  let body: ApiErrorEnvelope | undefined;
  try {
    body = (await res.json()) as ApiErrorEnvelope;
  } catch {
  }
  const err = body?.error;
  return new ApiError(
    res.status,
    err?.code ?? 'HTTP_ERROR',
    err?.message ?? `Request failed (${res.status})`,
    err?.details,
    err?.available,
    err?.sku,
  );
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  if (res.status === 204) {
    return undefined as T;
  }
  return (await res.json()) as T;
}

export const api = {
  listProducts(params: { search?: string; filter?: string; page?: number; limit?: number }): Promise<ProductsResponse> {
    const sp = new URLSearchParams();
    if (params.search) sp.set('search', params.search);
    if (params.filter && params.filter !== 'all') sp.set('filter', params.filter);
    if (params.page) sp.set('page', String(params.page));
    if (params.limit) sp.set('limit', String(params.limit));
    const qs = sp.toString();
    return request<ProductsResponse>(`/api/products${qs ? `?${qs}` : ''}`);
  },

  getSummary(): Promise<import('./types').Summary> {
    return request<import('./types').Summary>('/api/products/summary');
  },

  createProduct(payload: CreateProductPayload): Promise<Product> {
    return request<Product>('/api/products', { method: 'POST', body: JSON.stringify(payload) });
  },

  adjustStock(productId: number, payload: StockAdjustPayload): Promise<{ product: Product; movement: StockMovement }> {
    return request<{ product: Product; movement: StockMovement }>(`/api/products/${productId}/stock`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },

  listMovements(productId: number): Promise<{ data: StockMovement[] }> {
    return request<{ data: StockMovement[] }>(`/api/products/${productId}/movements`);
  },

  createOrder(payload: CreateOrderPayload): Promise<SalesOrder> {
    return request<SalesOrder>('/api/orders', { method: 'POST', body: JSON.stringify(payload) });
  },

  listOrders(params: { page?: number; limit?: number } = {}): Promise<import('./types').OrdersResponse> {
    const sp = new URLSearchParams();
    if (params.page) sp.set('page', String(params.page));
    if (params.limit) sp.set('limit', String(params.limit));
    const qs = sp.toString();
    return request<import('./types').OrdersResponse>(`/api/orders${qs ? `?${qs}` : ''}`);
  },

  getOrder(id: number): Promise<SalesOrder> {
    return request<SalesOrder>(`/api/orders/${id}`);
  },

  cancelOrder(id: number): Promise<SalesOrder> {
    return request<SalesOrder>(`/api/orders/${id}/cancel`, { method: 'POST' });
  },
};
