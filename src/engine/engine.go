package engine

import (
	"fmt"
	"sync"
)

// MatchingEngine is the top-level, thread-safe component for all symbols.
type MatchingEngine struct {
	Books map[string]*OrderBook
	Locks map[string]*sync.RWMutex
	globalMutex sync.RWMutex

	// Global, thread-safe store for ALL orders
	orderStore      map[string]*Order
	orderStoreMutex sync.RWMutex
}

// NewMatchingEngine creates a new, thread-safe engine.
func NewMatchingEngine() *MatchingEngine {
	return &MatchingEngine{
		Books:       make(map[string]*OrderBook),
		Locks:       make(map[string]*sync.RWMutex),
		orderStore:  make(map[string]*Order),
	}
}

// getBookAndLock is a thread-safe way to get/create the book and lock.
func (me *MatchingEngine) getBookAndLock(symbol string) (*OrderBook, *sync.RWMutex) {
	me.globalMutex.RLock()
	lock, ok := me.Locks[symbol]
	book, okBook := me.Books[symbol]
	me.globalMutex.RUnlock()

	if ok && okBook {
		return book, lock
	}

	me.globalMutex.Lock()
	defer me.globalMutex.Unlock()
	if lock, ok = me.Locks[symbol]; ok {
		return me.Books[symbol], lock
	}

	newLock := &sync.RWMutex{}
	newBook := NewOrderBook()
	me.Locks[symbol] = newLock
	me.Books[symbol] = newBook
	return newBook, newLock
}

// SubmitOrder is the thread-safe entry point for all new orders.
func (me *MatchingEngine) SubmitOrder(order *Order) (ProcessOrderResponse, error) {
	book, lock := me.getBookAndLock(order.Symbol)

	// Add order to global store first
	me.orderStoreMutex.Lock()
	me.orderStore[order.ID] = order
	me.orderStoreMutex.Unlock()

	lock.Lock()
	defer lock.Unlock()

	if order.Type == Market {
		totalQty, ok := book.checkMarketOrderLiquidity(order)
		if !ok {
			// Reject the order.
			// We must also remove it from the global store.
			me.orderStoreMutex.Lock()
			delete(me.orderStore, order.ID)
			me.orderStoreMutex.Unlock()
			return ProcessOrderResponse{}, fmt.Errorf("insufficient liquidity: only %d shares available, requested %d", totalQty, order.Quantity)
		}
	}

	response := book.ProcessOrder(order)

	return response, nil
}

// CancelOrder is the thread-safe entry point for cancelling an order.
func (me *MatchingEngine) CancelOrder(orderID string) (*Order, error) {
	// Find the order in the global store
	me.orderStoreMutex.Lock()
	order, ok := me.orderStore[orderID]
	if !ok {
		me.orderStoreMutex.Unlock()
		return nil, fmt.Errorf("order not found") // 404
	}

	// Check if it's already filled or cancelled
	if order.Status == StatusFilled || order.Status == StatusCancelled {
		me.orderStoreMutex.Unlock()
		return nil, fmt.Errorf("cannot cancel order already filled or cancelled") // 400
	}

	// Mark as cancelled
	order.Status = StatusCancelled
	me.orderStoreMutex.Unlock()

	// Now remove it from the active book
	book, lock := me.getBookAndLock(order.Symbol)
	lock.Lock()
	defer lock.Unlock()
	
	book.CancelOrder(order.ID) // This just removes it from the book

	return order, nil
}

// GetOrderStatus retrieves an order by its ID from the global store.
func (me *MatchingEngine) GetOrderStatus(orderID string) (*Order, error) {
	me.orderStoreMutex.RLock()
	defer me.orderStoreMutex.RUnlock()
	
	order, ok := me.orderStore[orderID]
	if !ok {
		return nil, fmt.Errorf("order not found") // 404
	}
	
	// Return a copy to avoid data races
	orderCopy := *order
	return &orderCopy, nil
}


// GetOrderBookSnapshot is a thread-safe way to get the book data.
type AggregatedPriceLevel struct {
	Price    int64 `json:"price"`
	Quantity int64 `json:"quantity"`
}

func (me *MatchingEngine) GetOrderBookSnapshot(symbol string, depth int) (bids []AggregatedPriceLevel, asks []AggregatedPriceLevel) {
	book, lock := me.getBookAndLock(symbol)

	lock.RLock()
	defer lock.RUnlock()

	if book == nil {
		return
	}

	// --- Get Asks (Lowest price first) ---
	askCount := 0
	book.asks.Ascend(func(l *PriceLevel) bool {
		if depth > 0 && askCount >= depth {
			return false
		}
		var totalQuantity int64
		for e := l.Orders.Front(); e != nil; e = e.Next() {
			totalQuantity += e.Value.(*Order).RemainingQuantity()
		}
		// Quantities at each price level are aggregated
		if totalQuantity > 0 {
			asks = append(asks, AggregatedPriceLevel{Price: l.Price, Quantity: totalQuantity})
			askCount++
		}
		return true
	})

	// --- Get Bids (Highest price first) ---
	bidCount := 0
	book.bids.Ascend(func(l *PriceLevel) bool {
		if depth > 0 && bidCount >= depth {
			return false
		}
		var totalQuantity int64
		for e := l.Orders.Front(); e != nil; e = e.Next() {
			totalQuantity += e.Value.(*Order).RemainingQuantity()
		}
		// Quantities at each price level are aggregated
		if totalQuantity > 0 {
			bids = append(bids, AggregatedPriceLevel{Price: l.Price, Quantity: totalQuantity})
			bidCount++
		}
		return true
	})

	return bids, asks
}