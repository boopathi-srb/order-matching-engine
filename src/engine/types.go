package engine

import (
	"container/list"
	"time"
)

// Side defines the side of an order (BUY or SELL).
type Side string
type OrderType string
type OrderStatus string

const (
	Buy  Side = "BUY"
	Sell Side = "SELL"
)

const (
	Limit  OrderType = "LIMIT"
	Market OrderType = "MARKET"
)

// NEW CONSTANTS for order status
const (
	StatusAccepted     OrderStatus = "ACCEPTED"
	StatusPartialFill  OrderStatus = "PARTIAL_FILL"
	StatusFilled       OrderStatus = "FILLED"
	StatusCancelled    OrderStatus = "CANCELLED"
)

// Order represents a single order in the matching engine.
type Order struct {
	ID        string      `json:"id"`
	Symbol    string      `json:"symbol"`
    Side      Side        `json:"side"`
	Type      OrderType   `json:"type"`
	Price     int64       `json:"price"`     // Stored as integer (cents)
	Quantity  int64       `json:"quantity"`  // Original quantity
	FilledQuantity int64  `json:"filled_quantity"`
	Status    OrderStatus `json:"status"`
	Timestamp int64       `json:"timestamp"` // Unix milliseconds

	// Internal field to store its place in the PriceLevel queue.
	element *list.Element
}

// RemainingQuantity calculates the unfilled quantity.
func (o *Order) RemainingQuantity() int64 {
	return o.Quantity - o.FilledQuantity
}

// Trade represents a single trade that has been executed.
type Trade struct {
	TradeID        string `json:"trade_id"`
	AggressorOrderID string `json:"aggressor_order_id"` // The ID of the incoming order
	RestingOrderID string `json:"resting_order_id"`   // The ID of the order that was in the book
	Price          int64  `json:"price"`
	Quantity       int64  `json:"quantity"`
	Timestamp      int64  `json:"timestamp"`
}

// ProcessOrderResponse is the result of processing an order
type ProcessOrderResponse struct {
	Trades            []Trade
	FilledRestingOrders []*Order
	OrderInBook       bool
	IsMarketOrder     bool
}

// NewOrder creates a new Order with a timestamp.
func NewOrder(id, symbol string, side Side, orderType OrderType, price, quantity int64) *Order {
	return &Order{
		ID:        id,
		Symbol:    symbol,
		Side:      side,
		Type:      orderType,
		Price:     price,
		Quantity:  quantity,
		FilledQuantity: 0,
		Status:    StatusAccepted, // Default status
		Timestamp: time.Now().UnixNano() / 1_000_000, // Unix Milliseconds
	}
}