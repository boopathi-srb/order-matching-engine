package engine

import (
	"container/list"
	"time"

	"github.com/google/btree"
	"github.com/google/uuid"
)

// --- B-Tree Comparators ---

// AsksSort sorts price levels from lowest price to highest price (min-heap)
func AsksSort(a, b *PriceLevel) bool {
	return a.Price < b.Price
}

// BidsSort sorts price levels from highest price to lowest price (max-heap)
func BidsSort(a, b *PriceLevel) bool {
	return a.Price > b.Price
}

// --- PriceLevel ---

// PriceLevel is a FIFO queue of Orders at a specific price.
type PriceLevel struct {
	Price  int64
	Orders *list.List // Queue of *Order
}

// NewPriceLevel creates a new PriceLevel queue
func NewPriceLevel(price int64) *PriceLevel {
	return &PriceLevel{
		Price:  price,
		Orders: list.New(),
	}
}

// AddOrder adds an order to the back of the queue (FIFO).
func (pl *PriceLevel) AddOrder(order *Order) {
	order.element = pl.Orders.PushBack(order)
}

// RemoveOrder removes a specific order from the queue.
func (pl *PriceLevel) RemoveOrder(order *Order) {
	if order.element != nil {
		pl.Orders.Remove(order.element)
		order.element = nil
	}
}

// --- OrderBook (Not Thread-Safe) ---

// OrderBook manages the buy and sell orders for a single symbol.
// It is NOT thread-safe and must be protected by a mutex.
type OrderBook struct {
	bids *btree.BTreeG[*PriceLevel] // Max-heap (highest price first)
	asks *btree.BTreeG[*PriceLevel] // Min-heap (lowest price first)

	bidPriceMap map[int64]*PriceLevel
	askPriceMap map[int64]*PriceLevel
	orderMap    map[string]*list.Element
}

// NewOrderBook creates and initializes a new OrderBook.
func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids:        btree.NewG(2, BidsSort),
		asks:        btree.NewG(2, AsksSort),
		bidPriceMap: make(map[int64]*PriceLevel),
		askPriceMap: make(map[int64]*PriceLevel),
		orderMap:    make(map[string]*list.Element),
	}
}

// checkMarketOrderLiquidity scans the book to find the total available quantity.
// It returns (totalQuantity, isSufficient).
func (ob *OrderBook) checkMarketOrderLiquidity(order *Order) (int64, bool) {
	var totalQuantity int64 = 0
	if order.Side == Buy {
		// Need to buy, so we check the asks (sellers)
		ob.asks.Ascend(func(pl *PriceLevel) bool {
			for e := pl.Orders.Front(); e != nil; e = e.Next() {
				totalQuantity += e.Value.(*Order).RemainingQuantity() // Check remaining
				if totalQuantity >= order.Quantity {
					return false
				}
			}
			return true
		})
	} else {
		// Need to sell, so we check the bids (buyers)
		ob.bids.Ascend(func(pl *PriceLevel) bool {
			for e := pl.Orders.Front(); e != nil; e = e.Next() {
				totalQuantity += e.Value.(*Order).RemainingQuantity() // Check remaining
				if totalQuantity >= order.Quantity {
					return false
				}
			}
			return true
		})
	}
	return totalQuantity, totalQuantity >= order.Quantity
}

// ProcessOrder processes a new order, attempting to match it.
func (ob *OrderBook) ProcessOrder(order *Order) ProcessOrderResponse {
	var trades []Trade
	var filledRestingOrders []*Order

	if order.Side == Buy {
		trades, filledRestingOrders = ob.matchBuyOrder(order)
	} else {
		trades, filledRestingOrders = ob.matchSellOrder(order)
	}

	orderInBook := false
	if order.Type == Limit && order.RemainingQuantity() > 0 {
		ob.addOrder(order)
		orderInBook = true
		if order.FilledQuantity > 0 {
			order.Status = StatusPartialFill
		}
	} else if order.RemainingQuantity() == 0 {
		order.Status = StatusFilled
	}

	return ProcessOrderResponse{
		Trades:              trades,
		FilledRestingOrders: filledRestingOrders,
		OrderInBook:         orderInBook,
		IsMarketOrder:       order.Type == Market,
	}
}

func (ob *OrderBook) matchBuyOrder(order *Order) ([]Trade, []*Order) {
	trades := []Trade{}
	filledOrders := []*Order{}

	for order.RemainingQuantity() > 0 && ob.asks.Len() > 0 {
		bestAskLevel, _ := ob.asks.Min()
		if order.Type == Limit && order.Price < bestAskLevel.Price {
			break
		}

		for bestAskLevel.Orders.Len() > 0 {
			element := bestAskLevel.Orders.Front()
			askOrder := element.Value.(*Order)

			tradeQuantity := min(order.RemainingQuantity(), askOrder.RemainingQuantity())
			tradePrice := askOrder.Price

			trades = append(trades, ob.createTrade(order.ID, askOrder.ID, tradePrice, tradeQuantity))

			order.FilledQuantity += tradeQuantity
			askOrder.FilledQuantity += tradeQuantity

			if askOrder.RemainingQuantity() == 0 {
				askOrder.Status = StatusFilled
				filledOrders = append(filledOrders, askOrder)
				ob.removeOrder(element)
			} else {
				// Partial fill of the resting order
				askOrder.Status = StatusPartialFill
			}

			if order.RemainingQuantity() == 0 {
				return trades, filledOrders
			}
		}
	}
	return trades, filledOrders
}

func (ob *OrderBook) matchSellOrder(order *Order) ([]Trade, []*Order) {
	trades := []Trade{}
	filledOrders := []*Order{}

	for order.RemainingQuantity() > 0 && ob.bids.Len() > 0 {
		bestBidLevel, _ := ob.bids.Min()
		if order.Type == Limit && order.Price > bestBidLevel.Price {
			break
		}

		for bestBidLevel.Orders.Len() > 0 {
			element := bestBidLevel.Orders.Front()
			bidOrder := element.Value.(*Order)

			tradeQuantity := min(order.RemainingQuantity(), bidOrder.RemainingQuantity())
			tradePrice := bidOrder.Price

			trades = append(trades, ob.createTrade(order.ID, bidOrder.ID, tradePrice, tradeQuantity))

			order.FilledQuantity += tradeQuantity
			bidOrder.FilledQuantity += tradeQuantity

			if bidOrder.RemainingQuantity() == 0 {
				bidOrder.Status = StatusFilled
				filledOrders = append(filledOrders, bidOrder)
				ob.removeOrder(element)
			} else {
				// Partial fill of the resting order
				bidOrder.Status = StatusPartialFill
			}

			if order.RemainingQuantity() == 0 {
				return trades, filledOrders
			}
		}
	}
	return trades, filledOrders
}

func (ob *OrderBook) createTrade(aggressorOrderID, restingOrderID string, price, quantity int64) Trade {
	return Trade{
		TradeID:          uuid.New().String(),
		AggressorOrderID: aggressorOrderID,
		RestingOrderID:   restingOrderID,
		Price:            price,
		Quantity:         quantity,
		Timestamp:        time.Now().UnixNano() / 1_000_000, // Unix Milliseconds
	}
}

// addOrder adds a limit order to the book.
func (ob *OrderBook) addOrder(order *Order) {
	if order.Side == Buy {
		ob.addBid(order)
	} else {
		ob.addAsk(order)
	}
}

func (ob *OrderBook) addBid(order *Order) {
	price := order.Price
	level, exists := ob.bidPriceMap[price]

	if !exists {
		level = NewPriceLevel(price)
		ob.bidPriceMap[price] = level
		ob.bids.ReplaceOrInsert(level) // O(log N)
	}

	level.AddOrder(order)
	ob.orderMap[order.ID] = order.element
}

func (ob *OrderBook) addAsk(order *Order) {
	price := order.Price
	level, exists := ob.askPriceMap[price]

	if !exists {
		level = NewPriceLevel(price)
		ob.askPriceMap[price] = level
		ob.asks.ReplaceOrInsert(level) // O(log N)
	}

	level.AddOrder(order)
	ob.orderMap[order.ID] = order.element
}

// removeOrder finds an order by its list element and removes it.
func (ob *OrderBook) removeOrder(element *list.Element) {
	order := element.Value.(*Order)
	delete(ob.orderMap, order.ID)

	var priceMap map[int64]*PriceLevel
	var tree *btree.BTreeG[*PriceLevel]

	if order.Side == Buy {
		priceMap = ob.bidPriceMap
		tree = ob.bids
	} else {
		priceMap = ob.askPriceMap
		tree = ob.asks
	}

	level := priceMap[order.Price]
	level.RemoveOrder(order)
	if level.Orders.Len() == 0 {
		delete(priceMap, order.Price)
		tree.Delete(level)
	}
}

// CancelOrder removes an order from the order book by its ID.
func (ob *OrderBook) CancelOrder(orderID string) bool {
	element, exists := ob.orderMap[orderID]
	if !exists {
		return false
	}
	ob.removeOrder(element)
	return true
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}