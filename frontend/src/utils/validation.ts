export function validateStockQty(value: string): string | null {
  if (value.trim() === '') return 'Qty is required';
  const n = Number(value);
  if (!Number.isInteger(n) || n <= 0) return 'Qty must be an integer > 0';
  return null;
}

export function validateOrderDraft(customer: string, items: Array<{ sku: string; qty: string }>): Record<string, string> {
  const errs: Record<string, string> = {};
  if (!customer.trim()) errs.customer = 'Customer is required';
  if (items.length === 0) errs.items = 'At least one item is required';
  const seen = new Set<string>();
  items.forEach((it, i) => {
    const sku = it.sku.trim();
    if (!sku) errs[`items.${i}.sku`] = 'SKU is required';
    else if (seen.has(sku)) errs[`items.${i}.sku`] = 'Duplicate SKU';
    seen.add(sku);
    const qErr = validateStockQty(it.qty);
    if (qErr) errs[`items.${i}.qty`] = qErr;
  });
  return errs;
}
