import { WorkflowEntrypoint } from "cloudflare:workers";
import { STOREFRONT_HTML } from "./storefront.js";

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

const JSON_HEADERS = { "content-type": "application/json; charset=utf-8" };

export class OrderReport extends WorkflowEntrypoint {
  async run(event, step) {
    return await step.do("summarize orders", async () => {
      const row = await this.env.DB.prepare(
        `SELECT count(*) AS orders, coalesce(sum(total_cents),0) AS revenue_cents FROM orders`,
      ).first();
      return { ...row, generated_at: Date.now() };
    });
  }
}

async function ensureSchema(db) {
  await db.exec(SCHEMA);
}

function json(data, status = 200) {
  return new Response(JSON.stringify(data), { status, headers: JSON_HEADERS });
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
  return json(row);
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
  return json({ products: results });
}

async function listCustomers(db, limit) {
  const { results } = await db
    .prepare(`SELECT id, email, tier FROM customers ORDER BY id ASC LIMIT ?`)
    .bind(limit)
    .all();
  return json({ customers: results });
}

async function listAudit(db, limit) {
  const { results } = await db
    .prepare(
      `SELECT entity, entity_id, action, at FROM audit_log ORDER BY at DESC LIMIT ?`,
    )
    .bind(limit)
    .all();
  return json({ entries: results });
}

async function addProduct(db, body) {
  const sku = String(body.sku || "").trim();
  const name = String(body.name || "").trim();
  const priceCents = Number(body.price_cents);
  const qty = Number(body.qty ?? 0);
  if (!sku || !name || !priceCents || priceCents < 1) {
    return json({ error: "sku, name, price_cents required" }, 400);
  }
  const now = Math.floor(Date.now() / 1000);
  const product = await db
    .prepare("INSERT INTO products (sku, name, price_cents) VALUES (?, ?, ?)")
    .bind(sku, name, priceCents)
    .run();
  const productId = product.meta.last_row_id;
  await db
    .prepare(
      "INSERT INTO inventory (product_id, qty, updated_at) VALUES (?, ?, ?)",
    )
    .bind(productId, Math.max(0, qty), now)
    .run();
  await db
    .prepare(
      "INSERT INTO audit_log (entity, entity_id, action, at) VALUES ('product', ?, 'create', ?)",
    )
    .bind(productId, now)
    .run();
  return json({ product_id: productId }, 201);
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
  return json({ order, items });
}

async function createOrder(db, body, env) {
  const customerId = Number(body.customer_id);
  const items = Array.isArray(body.items) ? body.items : [];
  if (!customerId || items.length === 0) {
    return json({ error: "customer_id and items required" }, 400);
  }
  const now = Math.floor(Date.now() / 1000);
  let total = 0;
  const priced = [];
  for (const it of items) {
    const pid = Number(it.product_id);
    const qty = Number(it.qty);
    if (!pid || !qty || qty < 1) {
      return json({ error: "invalid line item" }, 400);
    }
    const product = await db
      .prepare("SELECT id, price_cents FROM products WHERE id = ? AND active = 1")
      .bind(pid)
      .first();
    if (!product) {
      return json({ error: `product ${pid} missing` }, 404);
    }
    const inv = await db
      .prepare("SELECT qty FROM inventory WHERE product_id = ?")
      .bind(pid)
      .first("qty");
    if (inv === null || inv < qty) {
      return json({ error: `insufficient inventory for ${pid}` }, 409);
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

  if (env.FULFILLMENT) {
    await env.FULFILLMENT.send({
      type: "fulfillment",
      order_id: orderId,
      customer_id: customerId,
      total_cents: total,
      at: now,
    });
  }

  return json({ order_id: orderId, total_cents: total, queued: Boolean(env.FULFILLMENT) }, 201);
}

async function kvGet(env, key, fallback) {
  if (!env.CACHE) return fallback;
  const val = await env.CACHE.get(key);
  if (val === null) return fallback;
  try {
    return JSON.parse(val);
  } catch {
    return { value: val };
  }
}

async function kvPut(env, key, value) {
  if (!env.CACHE) return false;
  const payload = typeof value === "string" ? value : JSON.stringify(value);
  await env.CACHE.put(key, payload);
  return true;
}

async function meta(env) {
  const cronRaw = env.CACHE ? await env.CACHE.get("cron:last-run") : null;
  return json({
    bindings: {
      d1: Boolean(env.DB),
      kv: Boolean(env.CACHE),
      queue: Boolean(env.FULFILLMENT),
      workflow: Boolean(env.REPORTS),
      r2: Boolean(env.ASSETS),
      cron: true,
    },
    cron_last: cronRaw ? Number(cronRaw) : null,
  });
}

async function r2Upload(env, body) {
  if (!env.ASSETS) return json({ error: "R2 binding unavailable" }, 503);
  const note = String(body.note || "asset").slice(0, 2000);
  const key = `notes/${Date.now()}.txt`;
  await env.ASSETS.put(key, note, {
    httpMetadata: { contentType: "text/plain; charset=utf-8" },
  });
  const listed = await env.ASSETS.list({ prefix: "notes/", limit: 5 });
  return json({
    key,
    bytes: note.length,
    recent: (listed.objects || []).map((o) => o.key),
  });
}

export default {
  async fetch(request, env) {
    await ensureSchema(env.DB);
    const url = new URL(request.url);
    const path = url.pathname.replace(/\/+$/, "") || "/";

    if (path === "/" && request.method === "GET") {
      return new Response(STOREFRONT_HTML, {
        headers: { "content-type": "text/html; charset=utf-8" },
      });
    }
    if (path === "/health") {
      return json({ ok: true, service: "commerce-store" });
    }
    if (path === "/stats" && request.method === "GET") {
      return stats(env.DB);
    }
    if (path === "/api/meta" && request.method === "GET") {
      return meta(env);
    }
    if (path === "/api/products" && request.method === "GET") {
      const limit = Math.min(Number(url.searchParams.get("limit") || 20), 100);
      return listProducts(env.DB, limit);
    }
    if (path === "/api/products" && request.method === "POST") {
      const body = await request.json();
      return addProduct(env.DB, body);
    }
    if (path === "/api/customers" && request.method === "GET") {
      const limit = Math.min(Number(url.searchParams.get("limit") || 20), 100);
      return listCustomers(env.DB, limit);
    }
    if (path === "/api/audit" && request.method === "GET") {
      const limit = Math.min(Number(url.searchParams.get("limit") || 10), 50);
      return listAudit(env.DB, limit);
    }
    if (path.startsWith("/api/orders/") && request.method === "GET") {
      const id = path.split("/")[3];
      return getOrder(env.DB, Number(id));
    }
    if (path === "/api/orders" && request.method === "POST") {
      const body = await request.json();
      return createOrder(env.DB, body, env);
    }
    if (path === "/api/kv/cart") {
      if (request.method === "GET") {
        return json(await kvGet(env, "cart:session", { items: [] }));
      }
      if (request.method === "PUT") {
        const body = await request.json();
        await kvPut(env, "cart:session", body);
        return json(body);
      }
    }
    if (path === "/api/kv/banner") {
      if (request.method === "GET") {
        return json(await kvGet(env, "store:banner", { value: "" }));
      }
      if (request.method === "PUT") {
        const body = await request.json();
        await kvPut(env, "store:banner", body.value ?? "");
        return json({ ok: true });
      }
    }
    if (path === "/api/queue/enqueue" && request.method === "POST") {
      if (!env.FULFILLMENT) return json({ error: "queue binding unavailable" }, 503);
      let body = {};
      try {
        body = await request.json();
      } catch {
        body = {};
      }
      const payload = {
        type: "manual",
        message: body.message || "manual fulfillment task",
        at: Math.floor(Date.now() / 1000),
      };
      await env.FULFILLMENT.send(payload);
      return json({ ok: true, queued: payload }, 202);
    }
    if (path === "/api/workflow/report" && request.method === "POST") {
      if (!env.REPORTS) return json({ error: "workflow binding unavailable" }, 503);
      const instance = await env.REPORTS.create({ params: { scope: "orders" } });
      return json({ id: instance.id });
    }
    if (path === "/api/r2/upload" && request.method === "POST") {
      const body = await request.json();
      return r2Upload(env, body);
    }

    // Legacy JSON routes
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
      return createOrder(env.DB, body, env);
    }

    return json({ error: "not found", path }, 404);
  },

  async scheduled(controller, env) {
    const at = Math.floor(controller.scheduledTime / 1000);
    if (env.CACHE) {
      await env.CACHE.put("cron:last-run", String(at));
    }
    console.log("commerce-store cron", at);
  },
};
