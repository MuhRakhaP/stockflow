import { useCallback, useEffect, useState } from 'react';
import { api, ApiError } from './api/client';
import type { Product, SalesOrder, Summary } from './api/types';
import { formatIDR, timeAgo } from './utils/format';
import { validateOrderDraft, validateStockQty } from './utils/validation';

type Filter = 'all' | 'active' | 'inactive' | 'low';

function useDebounced<T>(value: T, delay: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), delay);
    return () => clearTimeout(t);
  }, [value, delay]);
  return v;
}

export default function App() {
  const [search, setSearch] = useState('');
  const debouncedSearch = useDebounced(search, 350);
  const [filter, setFilter] = useState<Filter>('all');
  const [products, setProducts] = useState<Product[]>([]);
  const [allProducts, setAllProducts] = useState<Product[]>([]);
  const [total, setTotal] = useState(0);
  const [page] = useState(1);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [summary, setSummary] = useState<Summary>({ total_products: 0, active_products: 0, total_stock_value: 0, low_stock_count: 0 });

  const [adjustProduct, setAdjustProduct] = useState<Product | null>(null);
  const [adjustType, setAdjustType] = useState<'in' | 'out'>('in');
  const [adjustQty, setAdjustQty] = useState('1');
  const [adjustNote, setAdjustNote] = useState('');
  const [adjustErr, setAdjustErr] = useState<string | null>(null);
  const [adjustSaving, setAdjustSaving] = useState(false);

  const [orderCustomer, setOrderCustomer] = useState('');
  const [orderItems, setOrderItems] = useState<Array<{ sku: string; qty: string }>>([{ sku: '', qty: '1' }]);
  const [orderErrs, setOrderErrs] = useState<Record<string, string>>({});
  const [orderApiErr, setOrderApiErr] = useState<string | null>(null);
  const [orderFieldErrSku, setOrderFieldErrSku] = useState<string | null>(null);
  const [orderSaving, setOrderSaving] = useState(false);
  const [createdOrder, setCreatedOrder] = useState<SalesOrder | null>(null);
  const [orders, setOrders] = useState<SalesOrder[]>([]);
  const [orderPage, setOrderPage] = useState(1);
  const [orderTotal, setOrderTotal] = useState(0);
  const orderLimit = 10;

  const [detailId, setDetailId] = useState('');
  const [detailOrder, setDetailOrder] = useState<SalesOrder | null>(null);
  const [detailErr, setDetailErr] = useState<string | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [cancelLoading, setCancelLoading] = useState(false);

  const loadProducts = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const res = await api.listProducts({ search: debouncedSearch, filter, page, limit: 50 });
      setProducts(res.data);
      setTotal(res.total);
    } catch (e) {
      setLoadError(e instanceof ApiError ? e.message : 'Failed to load products');
    } finally {
      setLoading(false);
    }
  }, [debouncedSearch, filter, page]);

  const loadSummary = useCallback(async () => {
    try {
      const res = await api.getSummary();
      setSummary(res);
    } catch {
    }
  }, []);

  const loadAllProducts = useCallback(async () => {
    try {
      const res = await api.listProducts({ page: 1, limit: 200 });
      setAllProducts(res.data);
    } catch {
    }
  }, []);

  const loadOrders = useCallback(async (p: number = orderPage) => {
    try {
      const res = await api.listOrders({ page: p, limit: orderLimit });
      setOrders(res.data);
      setOrderTotal(res.total);
      setOrderPage(res.page);
    } catch {
    }
  }, [orderPage, orderLimit]);

  const refreshAll = useCallback(async () => {
    await Promise.all([loadProducts(), loadSummary(), loadAllProducts(), loadOrders(orderPage)]);
  }, [loadProducts, loadSummary, loadAllProducts, loadOrders, orderPage]);

  useEffect(() => {
    void loadProducts();
  }, [loadProducts]);

  useEffect(() => {
    void loadSummary();
    void loadAllProducts();
    void loadOrders();
  }, [loadSummary, loadAllProducts, loadOrders]);

  const stats = {
    totalProducts: summary.total_products,
    active: summary.active_products,
    totalStockValue: summary.total_stock_value,
    lowStock: summary.low_stock_count,
  };

  const openAdjust = (p: Product) => {
    setAdjustProduct(p);
    setAdjustType('in');
    setAdjustQty('1');
    setAdjustNote('');
    setAdjustErr(null);
  };

  const submitAdjust = async () => {
    if (!adjustProduct) return;
    const qErr = validateStockQty(adjustQty);
    if (qErr) {
      setAdjustErr(qErr);
      return;
    }
    setAdjustSaving(true);
    setAdjustErr(null);
    try {
      await api.adjustStock(adjustProduct.id, { type: adjustType, qty: Number(adjustQty), note: adjustNote.trim() });
      setAdjustProduct(null);
      await refreshAll();
    } catch (e) {
      if (e instanceof ApiError && e.code === 'INSUFFICIENT_STOCK') {
        setAdjustErr(`Insufficient stock. Available: ${e.available ?? 0}`);
      } else if (e instanceof ApiError) {
        setAdjustErr(e.message);
      } else {
        setAdjustErr('Failed to adjust stock');
      }
    } finally {
      setAdjustSaving(false);
    }
  };

  const updateOrderItem = (idx: number, field: 'sku' | 'qty', value: string) => {
    setOrderItems((prev) => prev.map((it, i) => (i === idx ? { ...it, [field]: value } : it)));
  };

  const addOrderItem = () => setOrderItems((prev) => [...prev, { sku: '', qty: '1' }]);
  const removeOrderItem = (idx: number) => setOrderItems((prev) => prev.filter((_, i) => i !== idx));

  const submitOrder = async () => {
    const errs = validateOrderDraft(orderCustomer, orderItems);
    setOrderErrs(errs);
    setOrderApiErr(null);
    setOrderFieldErrSku(null);
    if (Object.keys(errs).length > 0) return;
    setOrderSaving(true);
    try {
      const payload = {
        customer: orderCustomer.trim(),
        items: orderItems.map((it) => ({ sku: it.sku.trim(), qty: Number(it.qty) })),
      };
      const order = await api.createOrder(payload);
      setCreatedOrder(order);
      setDetailOrder(order);
      setDetailId(String(order.id));
      setOrderCustomer('');
      setOrderItems([{ sku: '', qty: '1' }]);
      setOrderErrs({});
      await refreshAll();
      await loadOrders(1);
    } catch (e) {
      if (e instanceof ApiError && e.code === 'INSUFFICIENT_STOCK') {
        setOrderFieldErrSku(e.sku ?? null);
        setOrderApiErr(`Insufficient stock for ${e.sku ?? 'item'}. Available: ${e.available ?? 0}`);
      } else if (e instanceof ApiError && e.code === 'VALIDATION_ERROR') {
        const d = e.details;
        if (d && typeof d === 'object') {
          setOrderErrs(d as Record<string, string>);
        }
        setOrderApiErr(e.message);
      } else if (e instanceof ApiError) {
        setOrderApiErr(e.message);
      } else {
        setOrderApiErr('Failed to create order');
      }
    } finally {
      setOrderSaving(false);
    }
  };

  const loadOrderDetail = async () => {
    const id = Number(detailId);
    if (!Number.isInteger(id) || id <= 0) {
      setDetailErr('Enter a valid order ID');
      return;
    }
    setDetailLoading(true);
    setDetailErr(null);
    try {
      const o = await api.getOrder(id);
      setDetailOrder(o);
    } catch (e) {
      setDetailOrder(null);
      setDetailErr(e instanceof ApiError ? e.message : 'Failed to load order');
    } finally {
      setDetailLoading(false);
    }
  };

  const cancelOrder = async () => {
    if (!detailOrder) return;
    if (!confirm(`Cancel order #${detailOrder.id}? Stock will be restored.`)) return;
    setCancelLoading(true);
    setDetailErr(null);
    try {
      const o = await api.cancelOrder(detailOrder.id);
      setDetailOrder(o);
      setCreatedOrder((prev) => (prev?.id === o.id ? o : prev));
      await refreshAll();
      await loadOrders(1);
    } catch (e) {
      setDetailErr(e instanceof ApiError ? e.message : 'Failed to cancel order');
    } finally {
      setCancelLoading(false);
    }
  };

  return (
    <div className="container">
      <div className="header">
        <div>
          <h1>StockFlow</h1>
          <p className="sub">Inventory & Sales - simple warehouse console</p>
        </div>
        <button className="btn" onClick={() => void refreshAll()}>Refresh</button>
      </div>

      <div className="grid stats">
        <div className="card"><div className="stat-label">Total Products</div><div className="stat-value">{stats.totalProducts}</div></div>
        <div className="card"><div className="stat-label">Active Products</div><div className="stat-value">{stats.active}</div></div>
        <div className="card"><div className="stat-label">Total Stock Value</div><div className="stat-value">{formatIDR(stats.totalStockValue)}</div></div>
        <div className="card"><div className="stat-label">Low Stock (≤ 10)</div><div className="stat-value">{stats.lowStock}</div></div>
      </div>

      <div className="card">
        <div className="toolbar">
          <input placeholder="Search SKU or name..." value={search} onChange={(e) => setSearch(e.target.value)} />
          <select value={filter} onChange={(e) => setFilter(e.target.value as Filter)}>
            <option value="all">All</option>
            <option value="active">Active</option>
            <option value="inactive">Inactive</option>
            <option value="low">Low stock (≤ 10)</option>
          </select>
          <span className="muted small" style={{ alignSelf: 'center' }}>{products.length} shown / {total} total</span>
        </div>

        {loading && <p className="muted">Loading products...</p>}
        {loadError && (
          <div className="error">
            <div>{loadError}</div>
            <button className="btn btn-sm" style={{ marginTop: 8 }} onClick={() => void loadProducts()}>Retry</button>
          </div>
        )}
        {!loading && !loadError && products.length === 0 && (
          <div className="muted">No products found. <button className="btn btn-sm" onClick={() => void loadProducts()}>Retry</button></div>
        )}
        {!loading && !loadError && products.length > 0 && (
          <div style={{ overflowX: 'auto' }}>
            <table>
              <thead>
                <tr>
                  <th>SKU</th><th>Name</th><th>Price</th><th>Stock</th><th>Status</th><th>Last movement</th><th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {products.map((p) => (
                  <tr key={p.id}>
                    <td>{p.sku}</td>
                    <td>{p.name}</td>
                    <td>{formatIDR(p.price)}</td>
                    <td>
                      {p.stock}
                      {p.stock <= 10 && <span className="badge low" style={{ marginLeft: 6 }}>Low</span>}
                    </td>
                    <td><span className={`badge ${p.is_active ? 'active' : 'inactive'}`}>{p.is_active ? 'Active' : 'Inactive'}</span></td>
                    <td className="muted">{timeAgo(p.last_movement_at)}</td>
                    <td>
                      <div className="actions">
                        <button className="btn btn-sm" onClick={() => openAdjust(p)}>Adjust stock</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <h2 className="section-title">Create Order</h2>
      <div className="card">
        <div className="form-row">
          <label>Customer</label>
          <input value={orderCustomer} onChange={(e) => setOrderCustomer(e.target.value)} placeholder="Customer name" />
          {orderErrs.customer && <div className="error">{orderErrs.customer}</div>}
        </div>
        <div className="order-items">
          <div className="small muted" style={{ marginBottom: 8 }}>Items (SKU + qty). Unit price comes from server.</div>
          {orderItems.map((it, idx) => (
            <div key={idx}>
              <div className="order-item-row">
                <div className="form-row" style={{ marginBottom: 0 }}>
                  <label>SKU</label>
                  <select value={it.sku} onChange={(e) => updateOrderItem(idx, 'sku', e.target.value)}>
                    <option value="">Select SKU...</option>
                    {allProducts.map((p) => (
                      <option key={p.id} value={p.sku}>
                        {p.sku} - {p.name}{!p.is_active ? ' (inactive)' : ''}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="form-row" style={{ marginBottom: 0 }}>
                  <label>Qty</label>
                  <input value={it.qty} onChange={(e) => updateOrderItem(idx, 'qty', e.target.value)} inputMode="numeric" />
                </div>
                <button className="btn btn-sm" onClick={() => removeOrderItem(idx)} disabled={orderItems.length === 1}>×</button>
              </div>
              {(orderErrs[`items.${idx}.sku`] || orderErrs[`items.${idx}.qty`]) && (
                <div className="error small">
                  {orderErrs[`items.${idx}.sku`] && <div>{orderErrs[`items.${idx}.sku`]}</div>}
                  {orderErrs[`items.${idx}.qty`] && <div>{orderErrs[`items.${idx}.qty`]}</div>}
                </div>
              )}
              {orderFieldErrSku && it.sku.trim() === orderFieldErrSku && (
                <div className="error small">This SKU failed due to insufficient stock.</div>
              )}
            </div>
          ))}
          <button className="btn btn-sm" onClick={addOrderItem}>+ Add item</button>
        </div>
        {orderErrs.items && <div className="error">{orderErrs.items}</div>}
        {orderApiErr && <div className="error">{orderApiErr}</div>}
        <div className="modal-actions" style={{ justifyContent: 'flex-start' }}>
          <button className="btn primary" onClick={() => void submitOrder()} disabled={orderSaving}>
            {orderSaving ? 'Creating...' : 'Create Order'}
          </button>
        </div>
        {createdOrder && (
          <div className="success">
            <strong>Order #{createdOrder.id} created.</strong> Status: {createdOrder.status}. Server total: {formatIDR(createdOrder.total)}.
          </div>
        )}
      </div>

      <h2 className="section-title">Order History</h2>
      <div className="card" style={{ marginBottom: 16 }}>
        {orders.length === 0 ? (
          <p className="muted">No orders yet.</p>
        ) : (
          <>
            <div style={{ overflowX: 'auto' }}>
              <table>
                <thead>
                  <tr><th>ID</th><th>Customer</th><th>Status</th><th>Total</th><th>Created</th><th></th></tr>
                </thead>
                <tbody>
                  {orders.map((o) => (
                    <tr key={o.id}>
                      <td>#{o.id}</td>
                      <td>{o.customer}</td>
                      <td><span className={`badge ${o.status === 'CREATED' ? 'created' : 'cancelled'}`}>{o.status}</span></td>
                      <td>{formatIDR(o.total)}</td>
                      <td className="muted">{new Date(o.created_at).toLocaleString('id-ID')}</td>
                      <td>
                        <button className="btn btn-sm" onClick={() => { setDetailId(String(o.id)); setDetailOrder(o); setDetailErr(null); }}>View</button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="row" style={{ marginTop: 12, justifyContent: 'space-between' }}>
              <span className="muted small">Page {orderPage} of {Math.max(1, Math.ceil(orderTotal / orderLimit))} ({orderTotal} orders)</span>
              <div className="row">
                <button className="btn btn-sm" disabled={orderPage <= 1} onClick={() => void loadOrders(orderPage - 1)}>Prev</button>
                <button className="btn btn-sm" disabled={orderPage >= Math.ceil(orderTotal / orderLimit)} onClick={() => void loadOrders(orderPage + 1)}>Next</button>
              </div>
            </div>
          </>
        )}
      </div>

      <h2 className="section-title">Order Detail</h2>
      <div className="card">
        <div className="row" style={{ marginBottom: 12 }}>
          <input placeholder="Order ID" value={detailId} onChange={(e) => setDetailId(e.target.value)} style={{ width: 140, padding: '8px 10px', border: '1px solid #d1d5db', borderRadius: 8 }} />
          <button className="btn" onClick={() => void loadOrderDetail()} disabled={detailLoading}>{detailLoading ? 'Loading...' : 'Load'}</button>
          {detailOrder && detailOrder.status === 'CREATED' && (
            <button className="btn danger" onClick={() => void cancelOrder()} disabled={cancelLoading}>
              {cancelLoading ? 'Cancelling...' : 'Cancel Order'}
            </button>
          )}
        </div>
        {detailErr && <div className="error">{detailErr}</div>}
        {detailOrder && (
          <div>
            <div className="row" style={{ gap: 16, flexWrap: 'wrap', marginBottom: 8 }}>
              <div><span className="muted small">Order</span><div><strong>#{detailOrder.id}</strong></div></div>
              <div><span className="muted small">Customer</span><div>{detailOrder.customer}</div></div>
              <div><span className="muted small">Status</span><div><span className={`badge ${detailOrder.status === 'CREATED' ? 'created' : 'cancelled'}`}>{detailOrder.status}</span></div></div>
              <div><span className="muted small">Total</span><div><strong>{formatIDR(detailOrder.total)}</strong></div></div>
            </div>
            <table>
              <thead><tr><th>SKU</th><th>Qty</th><th>Unit Price</th><th>Line Total</th></tr></thead>
              <tbody>
                {(detailOrder.items ?? []).map((it) => (
                  <tr key={it.id}>
                    <td>{it.sku}</td>
                    <td>{it.qty}</td>
                    <td>{formatIDR(it.unit_price)}</td>
                    <td>{formatIDR(it.unit_price * it.qty)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {adjustProduct && (
        <div className="modal-backdrop" onClick={() => !adjustSaving && setAdjustProduct(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h3>Adjust Stock - {adjustProduct.sku}</h3>
            <p className="muted small" style={{ marginTop: 0 }}>{adjustProduct.name}. Current stock: <strong>{adjustProduct.stock}</strong></p>
            <div className="form-row">
              <label>Direction</label>
              <select value={adjustType} onChange={(e) => setAdjustType(e.target.value as 'in' | 'out')}>
                <option value="in">In (add)</option>
                <option value="out">Out (remove)</option>
              </select>
            </div>
            <div className="form-row">
              <label>Qty</label>
              <input value={adjustQty} onChange={(e) => setAdjustQty(e.target.value)} inputMode="numeric" />
            </div>
            <div className="form-row">
              <label>Note</label>
              <textarea value={adjustNote} onChange={(e) => setAdjustNote(e.target.value)} rows={2} />
            </div>
            {adjustErr && <div className="error">{adjustErr}</div>}
            <div className="modal-actions">
              <button className="btn" onClick={() => setAdjustProduct(null)} disabled={adjustSaving}>Cancel</button>
              <button className="btn primary" onClick={() => void submitAdjust()} disabled={adjustSaving}>
                {adjustSaving ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
