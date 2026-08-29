const SCHEMA = `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS customers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT NOT NULL UNIQUE,
  tier TEXT NOT NULL DEFAULT 'standard',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS products (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sku TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  price_cents INTEGER NOT NULL,
  active INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS inventory (
  product_id INTEGER PRIMARY KEY REFERENCES products(id),
  qty INTEGER NOT NULL DEFAULT 0,
  reserved INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS orders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  customer_id INTEGER NOT NULL REFERENCES customers(id),
  status TEXT NOT NULL DEFAULT 'pending',
  total_cents INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS order_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  order_id INTEGER NOT NULL REFERENCES orders(id),
  product_id INTEGER NOT NULL REFERENCES products(id),
  qty INTEGER NOT NULL,
  unit_price_cents INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity TEXT NOT NULL,
  entity_id INTEGER NOT NULL,
  action TEXT NOT NULL,
  at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_orders_created ON orders(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_id);
CREATE INDEX IF NOT EXISTS idx_audit_at ON audit_log(at DESC);
`;

async function ensureSchema(db) {
  await db.exec(SCHEMA);
}

async function stats(db) {
  const row = await db
    .prepare(
      `SELECT
        (SELECT count(*) FROM customers) AS customers,
        (SELECT count(*) FROM products) AS products,
        (SELECT count(*) FROM orders) AS orders,
        (SELECT count(*) FROM order_items) AS order_items,
        (SELECT coalesce(sum(qty),0) FROM inventory) AS inventory_units`,
    )
    .first();
  return Response.json(row);
}

async function listProducts(db, limit) {
  const { results } = await db
    .prepare(
      `SELECT p.id, p.sku, p.name, p.price_cents, i.qty
       FROM products p
       JOIN inventory i ON i.product_id = p.id
       WHERE p.active = 1
       ORDER BY p.id ASC LIMIT ?`,
    )
    .bind(limit)
    .all();
  return Response.json({ products: results });
}

async function getOrder(db, id) {
  const order = await db
    .prepare(
      `SELECT o.id, o.customer_id, o.status, o.total_cents, o.created_at
       FROM orders o WHERE o.id = ?`,
    )
    .bind(id)
    .first();
  if (!order) return new Response("not found", { status: 404 });
  const { results: items } = await db
    .prepare(
      `SELECT product_id, qty, unit_price_cents FROM order_items WHERE order_id = ?`,
    )
    .bind(id)
    .all();
  return Response.json({ order, items });
}

async function createOrder(db, body) {
  const customerId = Number(body.customer_id);
  const items = Array.isArray(body.items) ? body.items : [];
  if (!customerId || items.length === 0) {
    return Response.json({ error: "customer_id and items required" }, { status: 400 });
  }
  const now = Date.now();
  let total = 0;
  const priced = [];
  for (const it of items) {
    const pid = Number(it.product_id);
    const qty = Number(it.qty);
    if (!pid || !qty || qty < 1) {
      return Response.json({ error: "invalid line item" }, { status: 400 });
    }
    const product = await db
      .prepare("SELECT id, price_cents FROM products WHERE id = ? AND active = 1")
      .bind(pid)
      .first();
    if (!product) {
      return Response.json({ error: `product ${pid} missing` }, { status: 404 });
    }
    const inv = await db
      .prepare("SELECT qty FROM inventory WHERE product_id = ?")
      .bind(pid)
      .first("qty");
    if (inv === null || inv < qty) {
      return Response.json({ error: `insufficient inventory for ${pid}` }, { status: 409 });
    }
    const line = qty * product.price_cents;
    total += line;
    priced.push({ product_id: pid, qty, unit_price_cents: product.price_cents });
  }

  const order = await db
    .prepare(
      "INSERT INTO orders (customer_id, status, total_cents, created_at) VALUES (?, 'placed', ?, ?)",
    )
    .bind(customerId, total, now)
    .run();
  const orderId = order.meta.last_row_id;

  for (const it of priced) {
    await db
      .prepare(
        "INSERT INTO order_items (order_id, product_id, qty, unit_price_cents) VALUES (?, ?, ?, ?)",
      )
      .bind(orderId, it.product_id, it.qty, it.unit_price_cents)
      .run();
    await db
      .prepare(
        "UPDATE inventory SET qty = qty - ?, updated_at = ? WHERE product_id = ?",
      )
      .bind(it.qty, now, it.product_id)
      .run();
  }
  await db
    .prepare(
      "INSERT INTO audit_log (entity, entity_id, action, at) VALUES ('order', ?, 'create', ?)",
    )
    .bind(orderId, now)
    .run();

  return Response.json({ order_id: orderId, total_cents: total }, { status: 201 });
}

export default {
  async fetch(request, env) {
    await ensureSchema(env.DB);
    const url = new URL(request.url);
    const path = url.pathname.replace(/\/+$/, "") || "/";

    if (path === "/health") {
      return Response.json({ ok: true, service: "commerce" });
    }
    if (path === "/stats" && request.method === "GET") {
      return stats(env.DB);
    }
    if (path === "/products" && request.method === "GET") {
      const limit = Math.min(Number(url.searchParams.get("limit") || 20), 100);
      return listProducts(env.DB, limit);
    }
    if (path.startsWith("/orders/") && request.method === "GET") {
      const id = path.split("/")[2];
      return getOrder(env.DB, Number(id));
    }
    if (path === "/orders" && request.method === "POST") {
      const body = await request.json();
      return createOrder(env.DB, body);
    }

    return Response.json({ error: "not found", path }, { status: 404 });
  },
};
