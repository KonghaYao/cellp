export const STOREFRONT_HTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Commerce Store</title>
  <base href="./" />
  <style>
    :root {
      color-scheme: light dark;
      --bg: #0f1117;
      --card: #171a22;
      --border: #2a2f3a;
      --text: #e8eaed;
      --muted: #9aa3b2;
      --accent: #6ee7b7;
      --accent-dim: #134e3a;
      --danger: #f87171;
      font-family: ui-sans-serif, system-ui, -apple-system, sans-serif;
    }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--bg); color: var(--text); line-height: 1.5; }
    header {
      border-bottom: 1px solid var(--border);
      padding: 1rem 1.25rem;
      display: flex; flex-wrap: wrap; gap: 1rem; align-items: center; justify-content: space-between;
      background: rgba(0,0,0,.2);
      position: sticky; top: 0; z-index: 10;
    }
    h1 { margin: 0; font-size: 1.25rem; letter-spacing: -0.02em; }
    .sub { color: var(--muted); font-size: .85rem; margin-top: .15rem; }
    main { max-width: 1100px; margin: 0 auto; padding: 1.25rem; display: grid; gap: 1.25rem; }
    .grid { display: grid; gap: 1rem; }
    @media (min-width: 900px) { .grid-2 { grid-template-columns: 1.2fr .8fr; } }
    .card {
      background: var(--card); border: 1px solid var(--border); border-radius: 10px; padding: 1rem 1.1rem;
    }
    .card h2 { margin: 0 0 .75rem; font-size: .95rem; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); }
    .badges { display: flex; flex-wrap: wrap; gap: .4rem; margin-bottom: .75rem; }
    .badge {
      font-size: .7rem; padding: .2rem .5rem; border-radius: 999px;
      border: 1px solid var(--border); color: var(--muted);
    }
    .badge.on { border-color: var(--accent-dim); color: var(--accent); background: rgba(110,231,183,.08); }
    table { width: 100%; border-collapse: collapse; font-size: .875rem; }
    th, td { text-align: left; padding: .45rem .35rem; border-bottom: 1px solid var(--border); }
    th { color: var(--muted); font-weight: 500; font-size: .75rem; }
    button, .btn {
      cursor: pointer; border: 1px solid var(--border); background: #1e2430; color: var(--text);
      border-radius: 8px; padding: .45rem .75rem; font-size: .85rem;
    }
    button.primary { background: var(--accent-dim); border-color: #166534; color: var(--accent); }
    button:disabled { opacity: .5; cursor: not-allowed; }
    input, select, textarea {
      width: 100%; padding: .45rem .55rem; border-radius: 8px; border: 1px solid var(--border);
      background: #12151c; color: var(--text); font-size: .85rem;
    }
    label { display: block; font-size: .75rem; color: var(--muted); margin-bottom: .25rem; }
    .field { margin-bottom: .65rem; }
    .row { display: flex; gap: .5rem; flex-wrap: wrap; align-items: end; }
    .msg { font-size: .8rem; margin-top: .5rem; min-height: 1.2em; }
    .msg.ok { color: var(--accent); }
    .msg.err { color: var(--danger); }
    #banner { padding: .65rem .85rem; border-radius: 8px; background: rgba(110,231,183,.1); border: 1px solid var(--accent-dim); margin-bottom: .5rem; }
    pre { font-size: .75rem; overflow: auto; background: #12151c; padding: .65rem; border-radius: 8px; border: 1px solid var(--border); max-height: 160px; }
    .cart-line { display: flex; justify-content: space-between; gap: .5rem; font-size: .85rem; padding: .35rem 0; border-bottom: 1px dashed var(--border); }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>Commerce Store</h1>
      <p class="sub">D1 · KV · Queue · Workflow · R2 · Cron — live bindings demo</p>
    </div>
    <div id="stats" class="sub">Loading…</div>
  </header>
  <main>
    <div id="banner" hidden></div>

    <div class="grid grid-2">
      <section class="card">
        <h2>Catalog <span class="badge on">D1</span></h2>
        <div id="products"></div>
      </section>

      <section class="card">
        <h2>Cart <span class="badge on">KV</span></h2>
        <div id="cart"></div>
        <div class="field" style="margin-top:.75rem">
          <label>Customer</label>
          <select id="customer"></select>
        </div>
        <div class="row" style="margin-top:.5rem">
          <button class="primary" id="checkout">Checkout → D1 + Queue</button>
          <button id="clear-cart">Clear cart</button>
        </div>
        <p class="msg" id="checkout-msg"></p>
      </section>
    </div>

    <div class="grid grid-2">
      <section class="card">
        <h2>Add product <span class="badge on">D1 write</span></h2>
        <div class="row">
          <div class="field" style="flex:1"><label>SKU</label><input id="p-sku" placeholder="SKU-NEW" /></div>
          <div class="field" style="flex:2"><label>Name</label><input id="p-name" placeholder="New product" /></div>
        </div>
        <div class="row">
          <div class="field" style="flex:1"><label>Price (¢)</label><input id="p-price" type="number" value="999" /></div>
          <div class="field" style="flex:1"><label>Stock</label><input id="p-qty" type="number" value="25" /></div>
          <button class="primary" id="add-product" style="margin-bottom:.65rem">Add</button>
        </div>
        <p class="msg" id="product-msg"></p>
      </section>

      <section class="card">
        <h2>Bindings playground</h2>
        <div class="badges" id="binding-badges"></div>
        <div class="field"><label>KV banner text</label><input id="kv-banner" /></div>
        <button id="save-banner">Save banner (KV)</button>
        <div class="row" style="margin-top:.75rem">
          <button id="queue-test">Enqueue fulfillment task</button>
          <button id="workflow-run">Run order report (Workflow)</button>
        </div>
        <div class="field" style="margin-top:.75rem">
          <label>R2 asset note</label>
          <input id="r2-note" placeholder="Product launch memo" />
        </div>
        <button id="r2-upload">Upload to R2</button>
        <p class="msg" id="play-msg"></p>
        <pre id="play-out" hidden></pre>
      </section>
    </div>

    <section class="card">
      <h2>Audit log <span class="badge on">D1</span></h2>
      <div id="audit"></div>
    </section>
  </main>
  <script>
    const $ = (id) => document.getElementById(id);
    function apiUrl(path) {
      const rel = String(path).replace(/^\\.?\\//, "");
      const base = window.location.href.endsWith("/")
        ? window.location.href
        : window.location.href + "/";
      return new URL("./" + rel, base).href;
    }
    const api = (path, opts) => {
      return fetch(apiUrl(path), opts).then(async (r) => {
        const text = await r.text();
        let body = null;
        try { body = text ? JSON.parse(text) : null; } catch { body = text; }
        if (!r.ok) throw new Error(body?.error || body?.message || r.status + " " + text);
        return body;
      });
    };

    let products = [];
    let cart = { items: [] };

    function renderProducts() {
      if (!products.length) { $("products").innerHTML = '<p class="sub">No products</p>'; return; }
      $("products").innerHTML = '<table><thead><tr><th>SKU</th><th>Name</th><th>Price</th><th>Stock</th><th></th></tr></thead><tbody>' +
        products.map((p) => '<tr><td>' + p.sku + '</td><td>' + p.name + '</td><td>$' + (p.price_cents/100).toFixed(2) +
        '</td><td>' + p.qty + '</td><td><button data-add="' + p.id + '">Add</button></td></tr>').join("") + '</tbody></table>';
      $("products").querySelectorAll("[data-add]").forEach((btn) => {
        btn.onclick = () => addToCart(Number(btn.dataset.add));
      });
    }

    function renderCart() {
      if (!cart.items.length) { $("cart").innerHTML = '<p class="sub">Cart empty (stored in KV)</p>'; return; }
      $("cart").innerHTML = cart.items.map((it) => {
        const p = products.find((x) => x.id === it.product_id);
        return '<div class="cart-line"><span>' + (p?.name || ("#" + it.product_id)) + ' × ' + it.qty + '</span></div>';
      }).join("");
    }

    async function saveCart() {
      cart = await api("api/kv/cart", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(cart) });
      renderCart();
    }

    async function addToCart(productId) {
      const line = cart.items.find((i) => i.product_id === productId);
      if (line) line.qty += 1; else cart.items.push({ product_id: productId, qty: 1 });
      await saveCart();
    }

    async function loadAll() {
      const [stats, meta, banner, cartData, custs, prods, audit] = await Promise.all([
        api("stats"), api("api/meta"), api("api/kv/banner"), api("api/kv/cart"),
        api("api/customers?limit=20"), api("api/products?limit=30"), api("api/audit?limit=8"),
      ]);
      $("stats").textContent = stats.customers + " customers · " + stats.products + " products · " + stats.orders + " orders";
      $("banner").hidden = !banner.value;
      $("banner").textContent = banner.value || "";
      $("kv-banner").value = banner.value || "";
      cart = cartData;
      products = prods.products || [];
      $("customer").innerHTML = (custs.customers || []).map((c) =>
        '<option value="' + c.id + '">' + c.email + " (" + c.tier + ')</option>').join("");
      renderProducts();
      renderCart();
      $("audit").innerHTML = '<table><thead><tr><th>Entity</th><th>Action</th><th>When</th></tr></thead><tbody>' +
        (audit.entries || []).map((e) => '<tr><td>' + e.entity + " #" + e.entity_id + '</td><td>' + e.action +
        '</td><td>' + new Date(e.at * 1000).toLocaleString() + '</td></tr>').join("") + '</tbody></table>';
      const badges = [
        ["D1", true], ["KV", meta.bindings.kv], ["Queue", meta.bindings.queue],
        ["Workflow", meta.bindings.workflow], ["R2", meta.bindings.r2], ["Cron", meta.bindings.cron],
      ];
      $("binding-badges").innerHTML = badges.map(([n, on]) => '<span class="badge' + (on ? " on" : "") + '">' + n + '</span>').join("");
      if (meta.cron_last) $("play-msg").textContent = "Last cron (KV): " + new Date(meta.cron_last * 1000).toLocaleString();
    }

    $("add-product").onclick = async () => {
      $("product-msg").className = "msg";
      try {
        await api("api/products", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            sku: $("p-sku").value, name: $("p-name").value,
            price_cents: Number($("p-price").value), qty: Number($("p-qty").value),
          }),
        });
        $("product-msg").textContent = "Product added to D1 + inventory updated";
        $("product-msg").className = "msg ok";
        await loadAll();
      } catch (e) { $("product-msg").textContent = e.message; $("product-msg").className = "msg err"; }
    };

    $("checkout").onclick = async () => {
      $("checkout-msg").className = "msg";
      try {
        const res = await api("api/orders", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ customer_id: Number($("customer").value), items: cart.items }),
        });
        $("checkout-msg").textContent = "Order #" + res.order_id + " placed · $" + (res.total_cents/100).toFixed(2) + " · queued fulfillment";
        $("checkout-msg").className = "msg ok";
        cart = { items: [] };
        await saveCart();
        await loadAll();
      } catch (e) { $("checkout-msg").textContent = e.message; $("checkout-msg").className = "msg err"; }
    };

    $("clear-cart").onclick = async () => { cart = { items: [] }; await saveCart(); };

    $("save-banner").onclick = async () => {
      try {
        await api("api/kv/banner", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ value: $("kv-banner").value }) });
        await loadAll();
      } catch (e) { $("play-msg").textContent = e.message; $("play-msg").className = "msg err"; }
    };

    $("queue-test").onclick = async () => {
      try {
        const res = await api("api/queue/enqueue", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
        $("play-out").hidden = false; $("play-out").textContent = JSON.stringify(res, null, 2);
        $("play-msg").textContent = "Task enqueued"; $("play-msg").className = "msg ok";
      } catch (e) { $("play-msg").textContent = e.message; $("play-msg").className = "msg err"; }
    };

    $("workflow-run").onclick = async () => {
      try {
        const res = await api("api/workflow/report", { method: "POST" });
        $("play-out").hidden = false; $("play-out").textContent = JSON.stringify(res, null, 2);
        $("play-msg").textContent = "Workflow started"; $("play-msg").className = "msg ok";
      } catch (e) { $("play-msg").textContent = e.message; $("play-msg").className = "msg err"; }
    };

    $("r2-upload").onclick = async () => {
      try {
        const res = await api("api/r2/upload", {
          method: "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ note: $("r2-note").value || "demo asset" }),
        });
        $("play-out").hidden = false; $("play-out").textContent = JSON.stringify(res, null, 2);
        $("play-msg").textContent = "Uploaded to R2"; $("play-msg").className = "msg ok";
      } catch (e) { $("play-msg").textContent = e.message; $("play-msg").className = "msg err"; }
    };

    loadAll().catch((e) => { $("stats").textContent = "Failed: " + e.message; });
  </script>
</body>
</html>`;
