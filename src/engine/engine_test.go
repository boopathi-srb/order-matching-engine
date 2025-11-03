package engine

import (
    "testing"

    "github.com/stretchr/testify/assert"
)

// setup a new engine for each test
func setupEngine() *MatchingEngine {
	return NewMatchingEngine()
}

// Helper to create a test order
// We use a manual timestamp to control FIFO order
func newTestOrder(id, symbol string, side Side, oType OrderType, price, quantity int64, ts int64) *Order {
	return &Order{
		ID:        id,
		Symbol:    symbol,
		Side:      side,
		Type:      oType,
		Price:     price,
		Quantity:  quantity,
		FilledQuantity: 0,
		Status:    StatusAccepted,
		Timestamp: ts,
	}
}

// TestExample1_SimpleFullMatch tests a simple full match [cite: 137-156]
func TestExample1_SimpleFullMatch(t *testing.T) {
	eng := setupEngine()
	assert := assert.New(t)

	// 1. Initial Order Book State [cite: 139]
	sellOrder := newTestOrder("order-001", "AAPL", Sell, Limit, 15050, 1000, 1000)
	buyOrder1 := newTestOrder("order-002", "AAPL", Buy, Limit, 15045, 500, 1001)

	_, err := eng.SubmitOrder(sellOrder)
	assert.NoError(err)
	_, err = eng.SubmitOrder(buyOrder1)
	assert.NoError(err)

	// 2. New Order Arrives [cite: 140-146]
	newBuyOrder := newTestOrder("order-new", "AAPL", Buy, Limit, 15050, 500, 1002)
	resp, err := eng.SubmitOrder(newBuyOrder)

	// 3. Check Result [cite: 152-154]
	assert.NoError(err)
	assert.Equal(1, len(resp.Trades), "Should have executed 1 trade")
	assert.Equal(int64(0), newBuyOrder.RemainingQuantity(), "Incoming order should be fully filled")
	assert.Equal(StatusFilled, newBuyOrder.Status)
	assert.Equal(int64(500), resp.Trades[0].Quantity)
	assert.Equal(int64(15050), resp.Trades[0].Price, "Execution price must be the resting order's price")
	assert.Equal("order-001", resp.Trades[0].RestingOrderID)

	// 4. Check Final Order Book State [cite: 155-156]
	// The original sell order should have 500 remaining
	status, err := eng.GetOrderStatus("order-001")
	assert.NoError(err)
	assert.Equal(StatusPartialFill, status.Status)
	assert.Equal(int64(500), status.RemainingQuantity())
}

// TestExample2_MultiplePriceLevels tests "walking the book" [cite: 157-188]
func TestExample2_MultiplePriceLevels(t *testing.T) {
	eng := setupEngine()
	assert := assert.New(t)

	// 1. Initial Order Book State [cite: 159]
	sell1 := newTestOrder("order-003", "AAPL", Sell, Limit, 15050, 300, 1000)
	sell2 := newTestOrder("order-004", "AAPL", Sell, Limit, 15052, 400, 1001)
	sell3 := newTestOrder("order-005", "AAPL", Sell, Limit, 15055, 600, 1002)
	buy1 := newTestOrder("order-006", "AAPL", Buy, Limit, 15045, 500, 1003)

	_, _ = eng.SubmitOrder(sell1)
	_, _ = eng.SubmitOrder(sell2)
	_, _ = eng.SubmitOrder(sell3)
	_, _ = eng.SubmitOrder(buy1)

	// 2. New Order Arrives [cite: 160-166]
	newBuyOrder := newTestOrder("order-new", "AAPL", Buy, Limit, 15053, 800, 1004)
	resp, err := eng.SubmitOrder(newBuyOrder)

	// 3. Check Result [cite: 182-186]
	assert.NoError(err)
	assert.Equal(2, len(resp.Trades), "Should have executed 2 trades at different price levels")
	assert.Equal(int64(100), newBuyOrder.RemainingQuantity(), "Incoming order should have 100 remaining")
	assert.Equal(StatusPartialFill, newBuyOrder.Status)
	assert.True(resp.OrderInBook, "Remaining order should be in the book")

	// Trade 1: 300 shares @ 150.50
	assert.Equal(int64(300), resp.Trades[0].Quantity)
	assert.Equal(int64(15050), resp.Trades[0].Price)
	assert.Equal("order-003", resp.Trades[0].RestingOrderID)

	// Trade 2: 400 shares @ 150.52
	assert.Equal(int64(400), resp.Trades[1].Quantity)
	assert.Equal(int64(15052), resp.Trades[1].Price)
	assert.Equal("order-004", resp.Trades[1].RestingOrderID)

	// 4. Check Final Order Book State [cite: 187-188]
	// order-003 and order-004 should be filled
    _, err = eng.GetOrderStatus("order-003")
    assert.Error(err, "Order-003 should be filled and not in active book")
    _, err = eng.GetOrderStatus("order-004")
    assert.Error(err, "Order-004 should be filled and not in active book")
	
	// order-005 (sell) and order-006 (buy) should be untouched
	status5, _ := eng.GetOrderStatus("order-005")
	assert.Equal(int64(600), status5.RemainingQuantity())
	status6, _ := eng.GetOrderStatus("order-006")
	assert.Equal(int64(500), status6.RemainingQuantity())

	// order-new should be in the book as the new best bid
	statusNew, _ := eng.GetOrderStatus("order-new")
	assert.Equal(int64(100), statusNew.RemainingQuantity())
}

// TestExample3_TimePriorityFIFO tests FIFO at the same price level [cite: 189-213]
func TestExample3_TimePriorityFIFO(t *testing.T) {
	eng := setupEngine()
	assert := assert.New(t)

	// 1. Initial Order Book State (note the timestamps) [cite: 191]
	sell1 := newTestOrder("order-007", "AAPL", Sell, Limit, 15050, 200, 1000) // First
	sell2 := newTestOrder("order-008", "AAPL", Sell, Limit, 15050, 300, 1001) // Second
	sell3 := newTestOrder("order-009", "AAPL", Sell, Limit, 15050, 400, 1002) // Third

	_, _ = eng.SubmitOrder(sell1)
	_, _ = eng.SubmitOrder(sell2)
	_, _ = eng.SubmitOrder(sell3)
	
	// 2. New Order Arrives [cite: 192-198]
	newBuyOrder := newTestOrder("order-new", "AAPL", Buy, Limit, 15050, 500, 1003)
	resp, err := eng.SubmitOrder(newBuyOrder)

	// 3. Check Result [cite: 208-211]
	assert.NoError(err)
	assert.Equal(2, len(resp.Trades), "Should have executed 2 trades")
	assert.Equal(int64(0), newBuyOrder.RemainingQuantity(), "Incoming order should be fully filled")
	assert.Equal(StatusFilled, newBuyOrder.Status)
	assert.False(resp.OrderInBook)

	// Trade 1: Fills order-007 (oldest) first [cite: 201]
	assert.Equal(int64(200), resp.Trades[0].Quantity)
	assert.Equal("order-007", resp.Trades[0].RestingOrderID)

	// Trade 2: Fills order-008 (second oldest) next [cite: 204]
	assert.Equal(int64(300), resp.Trades[1].Quantity)
	assert.Equal("order-008", resp.Trades[1].RestingOrderID)
	
	// 4. Check Final Order Book State [cite: 212-213]
	// order-009 should be the only one left
	status9, err := eng.GetOrderStatus("order-009")
	assert.NoError(err)
	assert.Equal(int64(400), status9.RemainingQuantity())
	assert.Equal(StatusAccepted, status9.Status)

    _, err = eng.GetOrderStatus("order-007")
    assert.Error(err, "Order-007 should be filled")
}

// TestExample4_MarketOrderExecution tests a market order walking the book [cite: 215-242]
func TestExample4_MarketOrderExecution(t *testing.T) {
	eng := setupEngine()
	assert := assert.New(t)

	// 1. Initial Order Book State [cite: 217-219]
	sell1 := newTestOrder("order-010", "AAPL", Sell, Limit, 15050, 200, 1000)
	sell2 := newTestOrder("order-011", "AAPL", Sell, Limit, 15052, 300, 1001)
	sell3 := newTestOrder("order-012", "AAPL", Sell, Limit, 15055, 400, 1002)
	
	_, _ = eng.SubmitOrder(sell1)
	_, _ = eng.SubmitOrder(sell2)
	_, _ = eng.SubmitOrder(sell3)

	// 2. New Market Order Arrives [cite: 220-225]
	// Note: Price is 0 for market orders, it's ignored
	newBuyOrder := newTestOrder("order-new", "AAPL", Buy, Market, 0, 600, 1003)
	resp, err := eng.SubmitOrder(newBuyOrder)

	// 3. Check Result [cite: 238-242]
	assert.NoError(err)
	assert.Equal(3, len(resp.Trades), "Should have executed 3 trades")
	assert.Equal(int64(0), newBuyOrder.RemainingQuantity(), "Market order should be fully filled")
	assert.Equal(StatusFilled, newBuyOrder.Status)

	// Trade 1 @ 150.50 [cite: 228]
	assert.Equal(int64(200), resp.Trades[0].Quantity)
	assert.Equal(int64(15050), resp.Trades[0].Price)
	assert.Equal("order-010", resp.Trades[0].RestingOrderID)

	// Trade 2 @ 150.52 [cite: 231]
	assert.Equal(int64(300), resp.Trades[1].Quantity)
	assert.Equal(int64(15052), resp.Trades[1].Price)
	assert.Equal("order-011", resp.Trades[1].RestingOrderID)

	// Trade 3 @ 150.55 [cite: 234]
	assert.Equal(int64(100), resp.Trades[2].Quantity)
	assert.Equal(int64(15055), resp.Trades[2].Price)
	assert.Equal("order-012", resp.Trades[2].RestingOrderID)

	// 4. Check Final Order Book State
	// order-012 should have 300 remaining
	status12, err := eng.GetOrderStatus("order-012")
	assert.NoError(err)
	assert.Equal(int64(300), status12.RemainingQuantity())
	assert.Equal(StatusPartialFill, status12.Status)
}

// TestExample5_InsufficientLiquidity tests market order rejection [cite: 245-261]
func TestExample5_InsufficientLiquidity(t *testing.T) {
	eng := setupEngine()
	assert := assert.New(t)

	// 1. Initial Order Book State [cite: 247]
	sell1 := newTestOrder("order-013", "AAPL", Sell, Limit, 15050, 100, 1000)
	buy1 := newTestOrder("order-014", "AAPL", Buy, Limit, 15045, 500, 1001)

	_, _ = eng.SubmitOrder(sell1)
	_, _ = eng.SubmitOrder(buy1)

	// 2. New Market Order Arrives [cite: 248-253]
	newBuyOrder := newTestOrder("order-new", "AAPL", Buy, Market, 0, 500, 1003)
	resp, err := eng.SubmitOrder(newBuyOrder)

	// 3. Check Result [cite: 257-261]
    assert.Error(err, "Should have returned an error")
    assert.Contains(err.Error(), "insufficient liquidity", "Error message should be correct")
	assert.Equal(0, len(resp.Trades), "No trades should be executed")
	
	// 4. Check Final Order Book State
	// Book should be unchanged
	status13, _ := eng.GetOrderStatus("order-013")
	assert.Equal(int64(100), status13.RemainingQuantity())
	status14, _ := eng.GetOrderStatus("order-014")
	assert.Equal(int64(500), status14.RemainingQuantity())
}

// TestCancelOrder tests the DELETE /api/v1/orders/{order_id} logic [cite: 346]
func TestCancelOrder(t *testing.T) {
	eng := setupEngine()
	assert := assert.New(t)

	// 1. Add an order
	order := newTestOrder("order-tocancel", "AAPL", Buy, Limit, 10000, 100, 1000)
	_, err := eng.SubmitOrder(order)
    assert.NoError(err)

	// 2. Check that it's in the book
	status, err := eng.GetOrderStatus("order-tocancel")
	assert.NoError(err)
	assert.Equal(StatusAccepted, status.Status)

	// 3. Cancel the order
	cancelledOrder, err := eng.CancelOrder("order-tocancel")
	assert.NoError(err)
	assert.Equal(StatusCancelled, cancelledOrder.Status)

	// 4. Verify it's gone from the active book
	bids, asks := eng.GetOrderBookSnapshot("AAPL", 10)
	assert.Equal(0, len(bids))
	assert.Equal(0, len(asks))

	// 5. Verify its global status is Cancelled
	status, err = eng.GetOrderStatus("order-tocancel")
	assert.NoError(err)
	assert.Equal(StatusCancelled, status.Status)
	
	// 6. Test cancelling a non-existent order [cite: 476]
    _, err = eng.CancelOrder("order-does-not-exist")
    assert.Error(err)
	assert.Equal("order not found", err.Error())

	// 7. Test cancelling a filled order [cite: 359]
	_, _ = eng.SubmitOrder(newTestOrder("sell-order", "AAPL", Sell, Limit, 10000, 100, 1001))
    _, err = eng.CancelOrder("order-tocancel") // "order-tocancel" is now filled
    assert.Error(err)
	assert.Equal("cannot cancel order already filled or cancelled", err.Error())
}