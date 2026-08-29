#!/usr/bin/env bash
# Generate commerce D1 seed: multi-table schema + realistic row counts.
# Usage: commerce-seed.sh <output.db> [products] [customers] [orders]
set -euo pipefail

OUT="${1:?output.db path required}"
PRODUCTS="${2:-500}"
CUSTOMERS="${3:-2000}"
ORDERS="${4:-5000}"

sqlite3 "$OUT" <<'SQL'
PRAGMA foreign_keys = OFF;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS inventory;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS customers;
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
CREATE TABLE customers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT NOT NULL UNIQUE,
  tier TEXT NOT NULL DEFAULT 'standard',
  created_at INTEGER NOT NULL
);
CREATE TABLE products (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sku TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  price_cents INTEGER NOT NULL,
  active INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE inventory (
  product_id INTEGER PRIMARY KEY REFERENCES products(id),
  qty INTEGER NOT NULL DEFAULT 0,
  reserved INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL
);
CREATE TABLE orders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  customer_id INTEGER NOT NULL REFERENCES customers(id),
  status TEXT NOT NULL DEFAULT 'pending',
  total_cents INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE TABLE order_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  order_id INTEGER NOT NULL REFERENCES orders(id),
  product_id INTEGER NOT NULL REFERENCES products(id),
  qty INTEGER NOT NULL,
  unit_price_cents INTEGER NOT NULL
);
CREATE TABLE audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity TEXT NOT NULL,
  entity_id INTEGER NOT NULL,
  action TEXT NOT NULL,
  at INTEGER NOT NULL
);
CREATE INDEX idx_orders_customer ON orders(customer_id);
CREATE INDEX idx_orders_created ON orders(created_at DESC);
CREATE INDEX idx_order_items_order ON order_items(order_id);
CREATE INDEX idx_audit_at ON audit_log(at DESC);
SQL

now=$(date +%s)

for i in $(seq 1 "$PRODUCTS"); do
  price=$(( (RANDOM % 5000) + 199 ))
  sqlite3 "$OUT" "INSERT INTO products (sku, name, price_cents) VALUES ('SKU-${i}', 'Product ${i}', ${price});"
  sqlite3 "$OUT" "INSERT INTO inventory (product_id, qty, updated_at) VALUES (${i}, $(( (RANDOM % 200) + 50 )), ${now});"
done

for i in $(seq 1 "$CUSTOMERS"); do
  tier=$([[ $((i % 10)) -eq 0 ]] && echo premium || echo standard)
  sqlite3 "$OUT" "INSERT INTO customers (email, tier, created_at) VALUES ('user${i}@example.com', '${tier}', $((now - i)));"
done

for o in $(seq 1 "$ORDERS"); do
  cust=$(( (RANDOM % CUSTOMERS) + 1 ))
  items=$(( (RANDOM % 3) + 1 ))
  total=0
  sqlite3 "$OUT" "INSERT INTO orders (customer_id, status, total_cents, created_at) VALUES (${cust}, 'fulfilled', 0, $((now - o)));"
  for _ in $(seq 1 "$items"); do
    pid=$(( (RANDOM % PRODUCTS) + 1 ))
    qty=$(( (RANDOM % 3) + 1 ))
    price=$(sqlite3 "$OUT" "SELECT price_cents FROM products WHERE id=${pid};")
    total=$((total + price * qty))
    sqlite3 "$OUT" "INSERT INTO order_items (order_id, product_id, qty, unit_price_cents) VALUES (${o}, ${pid}, ${qty}, ${price});"
  done
  sqlite3 "$OUT" "UPDATE orders SET total_cents=${total} WHERE id=${o};"
  sqlite3 "$OUT" "INSERT INTO audit_log (entity, entity_id, action, at) VALUES ('order', ${o}, 'seed', $((now - o)));"
done

echo "seeded products=${PRODUCTS} customers=${CUSTOMERS} orders=${ORDERS} -> ${OUT}"
