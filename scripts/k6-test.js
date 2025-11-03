import http from "k6/http";
import { check } from "k6";

export const options = {
  vus: 400,
  duration: "60s",

  // ADD THIS BLOCK TO SHOW p99 and p999
  summaryTrendStats: [
    "avg",
    "min",
    "med",
    "max",
    "p(90)",
    "p(95)",
    "p(99)",
    "p(99.9)",
  ],
};

// This is your JSON body
const payload = JSON.stringify({
  symbol: "AAPL",
  side: "BUY",
  type: "LIMIT",
  price: 10001,
  quantity: 10,
});

const params = {
  headers: {
    "Content-Type": "application/json",
  },
};

// This is the main function that runs in a loop
export default function () {
  const res = http.post("http://localhost:8080/api/v1/orders", payload, params);

  // This checks for successful 2xx responses
  check(res, { "status was 2xx": (r) => r.status >= 200 && r.status < 300 });
}
