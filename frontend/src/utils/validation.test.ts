import { describe, expect, it } from 'vitest';
import { validateOrderDraft, validateStockQty } from './validation';

describe('validateStockQty', () => {
  it('rejects empty, zero, negative, and non-integer', () => {
    expect(validateStockQty('')).toBeTruthy();
    expect(validateStockQty('0')).toBeTruthy();
    expect(validateStockQty('-1')).toBeTruthy();
    expect(validateStockQty('1.5')).toBeTruthy();
    expect(validateStockQty('abc')).toBeTruthy();
  });

  it('accepts positive integer', () => {
    expect(validateStockQty('1')).toBeNull();
    expect(validateStockQty('10')).toBeNull();
  });
});

describe('validateOrderDraft', () => {
  it('requires customer and items', () => {
    const errs = validateOrderDraft('', []);
    expect(errs.customer).toBeTruthy();
    expect(errs.items).toBeTruthy();
  });

  it('flags duplicate sku and invalid qty', () => {
    const errs = validateOrderDraft('Acme', [
      { sku: 'SKU-001', qty: '1' },
      { sku: 'SKU-001', qty: '0' },
    ]);
    expect(errs['items.1.sku']).toMatch(/Duplicate/i);
    expect(errs['items.1.qty']).toBeTruthy();
  });

  it('returns empty object for valid draft', () => {
    const errs = validateOrderDraft('Acme', [{ sku: 'SKU-001', qty: '2' }]);
    expect(Object.keys(errs)).toHaveLength(0);
  });
});
