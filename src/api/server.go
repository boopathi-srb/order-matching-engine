package api

import (
    "encoding/json"
    "errors"
    "net/http"
    "strconv"
    "strings"

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
    s.mux.HandleFunc("/orders", s.handleOrders)
    s.mux.HandleFunc("/orders/", s.handleOrderByID)
    s.mux.HandleFunc("/orderbook", s.handleOrderBook)
    // API v1 aliases
    s.mux.HandleFunc("/api/v1/orders", s.handleOrders)
    s.mux.HandleFunc("/api/v1/orders/", s.handleOrderByID)
    s.mux.HandleFunc("/api/v1/orderbook", s.handleOrderBook)
    // simple health check
    s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })
    s.mux.HandleFunc("/api/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
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

type createOrderResponse struct {
    Order  *engine.Order          `json:"order"`
    Trades []engine.Trade         `json:"trades"`
    InBook bool                   `json:"order_in_book"`
    Market bool                   `json:"is_market_order"`
    Error  string                 `json:"error,omitempty"`
}

func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodPost:
        s.createOrder(w, r)
    default:
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    }
}

func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) {
    var req createOrderRequest
    decoder := json.NewDecoder(r.Body)
    decoder.DisallowUnknownFields()
    if err := decoder.Decode(&req); err != nil {
        http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
        return
    }
    if req.Symbol == "" {
        http.Error(w, "symbol is required", http.StatusBadRequest)
        return
    }
    if req.Quantity <= 0 {
        http.Error(w, "quantity must be > 0", http.StatusBadRequest)
        return
    }
    otype, err := parseOrderType(req.Type)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    side, err := parseSide(req.Side)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    if otype == engine.Limit && req.Price <= 0 {
        http.Error(w, "price must be > 0 for limit orders", http.StatusBadRequest)
        return
    }

    id := req.ID
    if id == "" {
        id = uuid.New().String()
    }

    order := engine.NewOrder(id, req.Symbol, side, otype, req.Price, req.Quantity)
    resp, err := s.eng.SubmitOrder(order)
    if err != nil {
        writeJSON(w, http.StatusBadRequest, createOrderResponse{Order: order, Trades: nil, InBook: false, Market: otype == engine.Market, Error: err.Error()})
        return
    }
    writeJSON(w, http.StatusOK, createOrderResponse{Order: order, Trades: resp.Trades, InBook: resp.OrderInBook, Market: resp.IsMarketOrder})
}

func (s *Server) handleOrderByID(w http.ResponseWriter, r *http.Request) {
    id := strings.TrimPrefix(r.URL.Path, "/orders/")
    if id == "" {
        http.Error(w, "order id required", http.StatusBadRequest)
        return
    }
    switch r.Method {
    case http.MethodGet:
        s.getOrder(w, r, id)
    case http.MethodDelete:
        s.cancelOrder(w, r, id)
    default:
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    }
}

func (s *Server) getOrder(w http.ResponseWriter, _ *http.Request, id string) {
    o, err := s.eng.GetOrderStatus(id)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    writeJSON(w, http.StatusOK, o)
}

func (s *Server) cancelOrder(w http.ResponseWriter, _ *http.Request, id string) {
    o, err := s.eng.CancelOrder(id)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    writeJSON(w, http.StatusOK, o)
}

type orderBookResponse struct {
    Bids []engine.AggregatedPriceLevel `json:"bids"`
    Asks []engine.AggregatedPriceLevel `json:"asks"`
}

func (s *Server) handleOrderBook(w http.ResponseWriter, r *http.Request) {
    symbol := r.URL.Query().Get("symbol")
    if symbol == "" {
        http.Error(w, "symbol is required", http.StatusBadRequest)
        return
    }
    depthParam := r.URL.Query().Get("depth")
    depth := 0
    if depthParam != "" {
        if v, err := strconv.Atoi(depthParam); err == nil && v >= 0 {
            depth = v
        } else {
            http.Error(w, "invalid depth", http.StatusBadRequest)
            return
        }
    }
    bids, asks := s.eng.GetOrderBookSnapshot(symbol, depth)
    writeJSON(w, http.StatusOK, orderBookResponse{Bids: bids, Asks: asks})
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

func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}


