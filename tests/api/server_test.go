package api_test

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    api "order-matching-engine/src/api"
    "order-matching-engine/src/engine"
)

func newTestServer() *api.Server {
    return api.NewServer(engine.NewMatchingEngine())
}

func TestCreateOrder_Accepted(t *testing.T) {
    srv := newTestServer()

    body := []byte(`{"symbol":"AAPL","side":"BUY","type":"LIMIT","price":10000,"quantity":10}`)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()

    srv.ServeHTTP(rr, req)

    if rr.Code != http.StatusCreated {
        t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
    }
    var got map[string]interface{}
    _ = json.Unmarshal(rr.Body.Bytes(), &got)
    if got["status"] != "ACCEPTED" {
        t.Fatalf("expected status ACCEPTED, got %v", got["status"])
    }
    if got["order_id"] == "" || got["order_id"] == nil {
        t.Fatalf("expected order_id to be present")
    }
}

func TestCreateOrder_PartialFill(t *testing.T) {
    srv := newTestServer()

    // Seed book: adds sells so incoming buy partially fills and remains
    seedSell1 := []byte(`{"id":"s1","symbol":"AAPL","side":"SELL","type":"LIMIT","price":15050,"quantity":300}`)
    seedSell2 := []byte(`{"id":"s2","symbol":"AAPL","side":"SELL","type":"LIMIT","price":15052,"quantity":400}`)
    doPost(t, srv, seedSell1, http.StatusCreated)
    doPost(t, srv, seedSell2, http.StatusCreated)

    // Incoming buy that walks two levels, leaves remainder
    body := []byte(`{"symbol":"AAPL","side":"BUY","type":"LIMIT","price":15053,"quantity":800}`)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)

    if rr.Code != http.StatusAccepted {
        t.Fatalf("expected 202, got %d body=%s", rr.Code, rr.Body.String())
    }
    var got map[string]interface{}
    _ = json.Unmarshal(rr.Body.Bytes(), &got)
    if got["status"] != "PARTIAL_FILL" {
        t.Fatalf("expected status PARTIAL_FILL, got %v", got["status"])
    }
    if got["filled_quantity"].(float64) != 700 {
        t.Fatalf("expected filled_quantity 700, got %v", got["filled_quantity"])
    }
    if got["remaining_quantity"].(float64) != 100 {
        t.Fatalf("expected remaining_quantity 100, got %v", got["remaining_quantity"])
    }
    trades, ok := got["trades"].([]interface{})
    if !ok || len(trades) != 2 {
        t.Fatalf("expected 2 trades, got %v", got["trades"])
    }
}

func TestCreateOrder_FullFill(t *testing.T) {
    srv := newTestServer()

    // Seed book: large sell so incoming buy fully fills
    seedSell := []byte(`{"id":"s1","symbol":"AAPL","side":"SELL","type":"LIMIT","price":15050,"quantity":1000}`)
    doPost(t, srv, seedSell, http.StatusCreated)

    body := []byte(`{"symbol":"AAPL","side":"BUY","type":"LIMIT","price":15050,"quantity":500}`)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)

    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
    }
    var got map[string]interface{}
    _ = json.Unmarshal(rr.Body.Bytes(), &got)
    if got["status"] != "FILLED" {
        t.Fatalf("expected status FILLED, got %v", got["status"])
    }
    if got["filled_quantity"].(float64) != 500 {
        t.Fatalf("expected filled_quantity 500, got %v", got["filled_quantity"])
    }
    trades, ok := got["trades"].([]interface{})
    if !ok || len(trades) != 1 {
        t.Fatalf("expected 1 trade, got %v", got["trades"])
    }
}

func TestCreateOrder_InsufficientLiquidity_Market(t *testing.T) {
    srv := newTestServer()

    body := []byte(`{"symbol":"AAPL","side":"BUY","type":"MARKET","quantity":500}`)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)

    if rr.Code != http.StatusBadRequest {
        t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
    }
    var got map[string]interface{}
    _ = json.Unmarshal(rr.Body.Bytes(), &got)
    if _, ok := got["error"]; !ok {
        t.Fatalf("expected error field, got %v", got)
    }
}

func doPost(t *testing.T, srv *api.Server, body []byte, expStatus int) {
    t.Helper()
    req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)
    if rr.Code != expStatus {
        t.Fatalf("seed POST expected %d got %d body=%s", expStatus, rr.Code, rr.Body.String())
    }
}


