package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"stockflow/backend/internal/model"
	"stockflow/backend/internal/store"
)

type Handler struct {
	store *store.Store
}

func New(st *store.Store) *Handler { return &Handler{store: st} }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/products", h.listProducts)
	mux.HandleFunc("GET /api/products/summary", h.summary)
	mux.HandleFunc("POST /api/products", h.createProduct)
	mux.HandleFunc("PUT /api/products/{id}", h.updateProduct)
	mux.HandleFunc("DELETE /api/products/{id}", h.deleteProduct)
	mux.HandleFunc("POST /api/products/{id}/stock", h.adjustStock)
	mux.HandleFunc("GET /api/products/{id}/movements", h.listMovements)
	mux.HandleFunc("GET /api/orders", h.listOrders)
	mux.HandleFunc("POST /api/orders", h.createOrder)
	mux.HandleFunc("POST /api/orders/{id}/cancel", h.cancelOrder)
	mux.HandleFunc("GET /api/orders/{id}", h.getOrder)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string, details any) {
	writeJSON(w, status, model.ErrorEnvelope{Error: model.ErrorBody{Code: code, Message: msg, Details: details}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_JSON", err.Error(), nil)
		return false
	}
	return true
}

func parseID(r *http.Request) (int, error) {
	return strconv.Atoi(r.PathValue("id"))
}

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	search := q.Get("search")
	filter := q.Get("filter")
	resp, err := h.store.ListProducts(r.Context(), search, filter, page, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	sum, err := h.store.Summary(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var req model.CreateProductRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.SKU = strings.TrimSpace(req.SKU)
	req.Name = strings.TrimSpace(req.Name)
	errs := map[string]string{}
	if req.SKU == "" {
		errs["sku"] = "sku is required"
	}
	if req.Name == "" {
		errs["name"] = "name is required"
	}
	if req.Price == nil {
		errs["price"] = "price is required"
	} else if *req.Price < 0 {
		errs["price"] = "price must be >= 0"
	}
	if len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, model.ErrorEnvelope{Error: model.ErrorBody{Code: "VALIDATION_ERROR", Message: "validation failed", Details: errs}})
		return
	}
	p, err := h.store.CreateProduct(r.Context(), req)
	if err != nil {
		if isUnique(err) {
			writeJSON(w, http.StatusConflict, model.ErrorEnvelope{Error: model.ErrorBody{Code: "SKU_ALREADY_EXISTS", Message: "sku already exists"}})
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_ID", "invalid id", nil)
		return
	}
	var req model.UpdateProductRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	if req.SKU != nil && strings.TrimSpace(*req.SKU) == "" {
		errs["sku"] = "sku cannot be empty"
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		errs["name"] = "name cannot be empty"
	}
	if req.Price != nil && *req.Price < 0 {
		errs["price"] = "price must be >= 0"
	}
	if len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, model.ErrorEnvelope{Error: model.ErrorBody{Code: "VALIDATION_ERROR", Message: "validation failed", Details: errs}})
		return
	}
	p, err := h.store.UpdateProduct(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "product not found", nil)
			return
		}
		if isUnique(err) {
			writeJSON(w, http.StatusConflict, model.ErrorEnvelope{Error: model.ErrorBody{Code: "SKU_ALREADY_EXISTS", Message: "sku already exists"}})
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) deleteProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_ID", "invalid id", nil)
		return
	}
	err = h.store.DeleteProduct(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "product not found", nil)
			return
		}
		if errors.Is(err, store.ErrProductHasHistory) {
			writeJSON(w, http.StatusConflict, model.ErrorEnvelope{Error: model.ErrorBody{Code: "PRODUCT_HAS_HISTORY"}})
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) adjustStock(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_ID", "invalid id", nil)
		return
	}
	var req model.StockAdjustRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Type = strings.TrimSpace(strings.ToLower(req.Type))
	errs := map[string]string{}
	if req.Type != "in" && req.Type != "out" {
		errs["type"] = "type must be 'in' or 'out'"
	}
	if req.Qty == nil || *req.Qty <= 0 {
		errs["qty"] = "qty must be > 0"
	}
	if len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, model.ErrorEnvelope{Error: model.ErrorBody{Code: "VALIDATION_ERROR", Message: "validation failed", Details: errs}})
		return
	}
	p, m, err := h.store.AdjustStock(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "product not found", nil)
			return
		}
		var ise *store.InsufficientStockError
		if errors.As(err, &ise) {
			avail := ise.Available
			writeJSON(w, http.StatusConflict, model.ErrorEnvelope{Error: model.ErrorBody{Code: "INSUFFICIENT_STOCK", Available: &avail}})
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"product": p, "movement": m})
}

func (h *Handler) listMovements(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_ID", "invalid id", nil)
		return
	}
	data, err := h.store.ListMovements(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "product not found", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var req model.CreateOrderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Customer = strings.TrimSpace(req.Customer)
	errs := map[string]string{}
	if req.Customer == "" {
		errs["customer"] = "customer is required"
	}
	if len(req.Items) == 0 {
		errs["items"] = "items must not be empty"
	}
	seen := map[string]bool{}
	for i, it := range req.Items {
		sku := strings.TrimSpace(it.SKU)
		req.Items[i].SKU = sku
		if sku == "" {
			errs[fmtField(i, "sku")] = "sku is required"
		} else if seen[sku] {
			errs[fmtField(i, "sku")] = "duplicate sku in order"
		}
		seen[sku] = true
		if it.Qty == nil || *it.Qty <= 0 {
			errs[fmtField(i, "qty")] = "qty must be > 0"
		}
	}
	if len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, model.ErrorEnvelope{Error: model.ErrorBody{Code: "VALIDATION_ERROR", Message: "validation failed", Details: errs}})
		return
	}
	order, err := h.store.CreateOrder(r.Context(), req)
	if err != nil {
		var ise *store.InsufficientStockError
		if errors.As(err, &ise) {
			avail := ise.Available
			writeJSON(w, http.StatusConflict, model.ErrorEnvelope{Error: model.ErrorBody{Code: "INSUFFICIENT_STOCK", SKU: ise.SKU, Available: &avail}})
			return
		}
		if strings.HasPrefix(err.Error(), "unknown sku:") {
			sku := strings.TrimSpace(strings.TrimPrefix(err.Error(), "unknown sku:"))
			writeJSON(w, http.StatusBadRequest, model.ErrorEnvelope{Error: model.ErrorBody{Code: "UNKNOWN_SKU", Message: "unknown sku", Details: map[string]string{"sku": sku}}})
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (h *Handler) cancelOrder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_ID", "invalid id", nil)
		return
	}
	order, err := h.store.CancelOrder(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "order not found", nil)
			return
		}
		if errors.Is(err, store.ErrAlreadyCancelled) {
			writeJSON(w, http.StatusConflict, model.ErrorEnvelope{Error: model.ErrorBody{Code: "ORDER_ALREADY_CANCELLED", Message: "order already cancelled"}})
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	resp, err := h.store.ListOrders(r.Context(), page, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_ID", "invalid id", nil)
		return
	}
	order, err := h.store.GetOrder(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "order not found", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func fmtField(i int, f string) string {
	return "items[" + strconv.Itoa(i) + "]." + f
}
