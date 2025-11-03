package api

import (
    "encoding/json"
    "errors"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/google/uuid"
    "order-matching-engine/src/engine"
)

type Server struct {
    eng *engine.MatchingEngine
    mux *http.ServeMux
}

func NewServer(eng *engine.MatchingEngine) *Server {
    s := &Server{eng: eng, mux: http.NewServeMux()}
    s.registerRoutes()
    return s
}

func (s *Server) Start(addr string) error {
    return http.ListenAndServe(addr, s.mux)
}

// ServeHTTP allows Server to satisfy http.Handler, delegating to its mux.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
    // API v1 aliases
    s.mux.HandleFunc("/api/v1/orders", s.handleOrders)
    s.mux.HandleFunc("/api/v1/orders/", s.handleOrderByID)
    s.mux.HandleFunc("/api/v1/orderbook", s.handleOrderBookGeneral)
    s.mux.HandleFunc("/api/v1/orderbook/", s.handleOrderBookGeneral)
    // simple health check
    s.mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        _ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "healthy"})
    })
}

type createOrderRequest struct {
    ID       string `json:"id"`
    Symbol   string `json:"symbol"`
    Side     string `json:"side"`
    Type     string `json:"type"`
    Price    int64  `json:"price"`
    Quantity int64  `json:"quantity"`
}

func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodPost:
        s.createOrder(w, r)
    default:
        s.writeErrorPlain(w, http.StatusMethodNotAllowed, "method not allowed")
    }
}

func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) {
    var req createOrderRequest
    decoder := json.NewDecoder(r.Body)
    decoder.DisallowUnknownFields()
    if err := decoder.Decode(&req); err != nil {
        s.writeErrorPlain(w, http.StatusBadRequest, "Invalid json")
        return
    }
    if req.Symbol == "" {
        s.writeErrorPlain(w, http.StatusBadRequest, "Invalid order: symbol is required")
        return
    }
    if req.Quantity <= 0 {
        s.writeErrorPlain(w, http.StatusBadRequest, "Invalid order: quantity must be positive")
        return
    }
    otype, err := parseOrderType(req.Type)
    if err != nil {
        s.writeErrorPlain(w, http.StatusBadRequest, "Invalid order: "+err.Error())
        return
    }
    side, err := parseSide(req.Side)
    if err != nil {
        s.writeErrorPlain(w, http.StatusBadRequest, "Invalid order: "+err.Error())
        return
    }
    if otype == engine.Limit && req.Price <= 0 {
        s.writeErrorPlain(w, http.StatusBadRequest, "Invalid order: price must be > 0 for limit orders")
        return
    }
    // Always generate a new ID server side
    id := uuid.New().String()
    order := engine.NewOrder(id, req.Symbol, side, otype, req.Price, req.Quantity)
    resp, err := s.eng.SubmitOrder(order)
    if err != nil {
        s.writeErrorPlain(w, http.StatusBadRequest, err.Error())
        return
    }
    w.Header().Set("Content-Type", "application/json")
    switch order.Status {
    case engine.StatusAccepted:
        w.WriteHeader(http.StatusCreated)
        _ = json.NewEncoder(w).Encode(map[string]interface{}{
            "order_id": order.ID,
            "status":   string(order.Status),
            "message":  "Order added to book",
        })
        return
    case engine.StatusPartialFill:
        w.WriteHeader(http.StatusAccepted)
        _ = json.NewEncoder(w).Encode(map[string]interface{}{
            "order_id":           order.ID,
            "status":             string(order.Status),
            "filled_quantity":    order.FilledQuantity,
            "remaining_quantity": order.RemainingQuantity(),
            "trades":             resp.Trades,
        })
        return
    case engine.StatusFilled:
        w.WriteHeader(http.StatusOK)
        _ = json.NewEncoder(w).Encode(map[string]interface{}{
            "order_id":        order.ID,
            "status":          string(order.Status),
            "filled_quantity": order.FilledQuantity,
            "trades":          resp.Trades,
        })
        return
    default:
        w.WriteHeader(http.StatusCreated)
        _ = json.NewEncoder(w).Encode(map[string]interface{}{
            "order_id": order.ID,
            "status":   string(order.Status),
            "message":  "Order added to book",
        })
        return
    }
}

func (s *Server) handleOrderByID(w http.ResponseWriter, r *http.Request) {
    base := "/api/v1/orders/"
    id := strings.TrimPrefix(r.URL.Path, base)
    if id == "" || id == r.URL.Path {
        s.writeErrorPlain(w, http.StatusBadRequest, "Invalid order: order id required")
        return
    }
    switch r.Method {
    case http.MethodGet:
        s.getOrder(w, r, id)
    case http.MethodDelete:
        s.cancelOrder(w, r, id)
    default:
        s.writeErrorPlain(w, http.StatusMethodNotAllowed, "method not allowed")
    }
}

func (s *Server) getOrder(w http.ResponseWriter, _ *http.Request, id string) {
    o, err := s.eng.GetOrderStatus(id)
    if err != nil {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusNotFound)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "Order not found"})
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    _ = json.NewEncoder(w).Encode(map[string]interface{}{
        "order_id":        o.ID,
        "symbol":          o.Symbol,
        "side":            string(o.Side),
        "type":            string(o.Type),
        "price":           o.Price,
        "quantity":        o.Quantity,
        "filled_quantity": o.FilledQuantity,
        "status":          string(o.Status),
        "timestamp":       o.Timestamp,
    })
}

func (s *Server) cancelOrder(w http.ResponseWriter, _ *http.Request, id string) {
    o, err := s.eng.CancelOrder(id)
    if err != nil {
        w.Header().Set("Content-Type", "application/json")
        if strings.Contains(err.Error(), "order not found") {
            w.WriteHeader(http.StatusNotFound)
            _ = json.NewEncoder(w).Encode(map[string]string{"error": "Order not found"})
            return
        }
        if strings.Contains(err.Error(), "cannot cancel order") {
            w.WriteHeader(http.StatusBadRequest)
            _ = json.NewEncoder(w).Encode(map[string]string{"error": "Cannot cancel: order already filled"})
            return
        }
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    _ = json.NewEncoder(w).Encode(map[string]interface{}{
        "order_id": o.ID,
        "status":   string(o.Status),
    })
}

// Unified handler for both /api/v1/orderbook and /api/v1/orderbook/{symbol}
func (s *Server) handleOrderBookGeneral(w http.ResponseWriter, r *http.Request) {
    var symbol string
    base := "/api/v1/orderbook/"
    if strings.HasPrefix(r.URL.Path, base) {
        symbol = strings.TrimPrefix(r.URL.Path, base)
        if i := strings.Index(symbol, "/"); i != -1 {
            symbol = symbol[:i] // Defensive
        }
    } else {
        symbol = r.URL.Query().Get("symbol")
    }
    if symbol == "" {
        s.writeErrorPlain(w, http.StatusBadRequest, "symbol is required")
        return
    }
    depth := 0
    if v := r.URL.Query().Get("depth"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n >= 0 {
            depth = n
        } else {
            s.writeErrorPlain(w, http.StatusBadRequest, "invalid depth")
            return
        }
    }
    bids, asks := s.eng.GetOrderBookSnapshot(symbol, depth)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    _ = json.NewEncoder(w).Encode(map[string]interface{}{
        "symbol":    symbol,
        "timestamp": time.Now().UnixNano() / 1_000_000,
        "bids":      bids,
        "asks":      asks,
    })
}

func parseSide(s string) (engine.Side, error) {
    switch strings.ToUpper(strings.TrimSpace(s)) {
    case string(engine.Buy):
        return engine.Buy, nil
    case string(engine.Sell):
        return engine.Sell, nil
    default:
        return "", errors.New("invalid side; must be BUY or SELL")
    }
}

func parseOrderType(s string) (engine.OrderType, error) {
    switch strings.ToUpper(strings.TrimSpace(s)) {
    case string(engine.Limit):
        return engine.Limit, nil
    case string(engine.Market):
        return engine.Market, nil
    default:
        return "", errors.New("invalid type; must be LIMIT or MARKET")
    }
}
// helper for spec-style simple error bodies
func (s *Server) writeErrorPlain(w http.ResponseWriter, status int, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
