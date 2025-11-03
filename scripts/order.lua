-- scripts/order.lua
wrk.method = "POST"
wrk.body   = '{"symbol": "AAPL", "side": "BUY", "type": "LIMIT", "price": 10001, "quantity": 10}'
wrk.headers["Content-Type"] = "application/json"