// celld requires ESM `export default { fetch }`. Flareact's webpack worker is
// a Service Worker IIFE (`addEventListener('fetch'|'scheduled')`) with no
// default export — wrangler dry-run leaves that IIFE intact (`default not object`).
// This wrapper loads the IIFE as a side-effect module and bridges listeners
// to the module-worker contract. KV bindings stay on `env`; the app still
// reads the Service Worker global `KV_STATUS_PAGE`.

const listeners = { fetch: [], scheduled: [] };

function makeEventTarget(type, extra) {
  const ev = {
    type,
    waitUntil(p) {
      if (p && typeof p.then === 'function') {
        p.catch((err) => console.error('waitUntil', err));
      }
    },
    ...extra,
  };
  return ev;
}

function dispatch(type, extra) {
  const ev = makeEventTarget(type, extra);
  const list = listeners[type] || [];
  for (const fn of list) fn(ev);
  return ev;
}

function installListenerShim() {
  const prev = globalThis.addEventListener;
  globalThis.addEventListener = (type, fn, ...rest) => {
    if (type === 'fetch' || type === 'scheduled') {
      listeners[type].push(fn);
      return;
    }
    if (typeof prev === 'function') return prev.call(globalThis, type, fn, ...rest);
  };
}

function aliasBindings(env) {
  if (!env) return;
  if (env.KV_STATUS_PAGE && typeof globalThis.KV_STATUS_PAGE === 'undefined') {
    globalThis.KV_STATUS_PAGE = env.KV_STATUS_PAGE;
  }
  if (env.ASSETS && typeof globalThis.ASSETS === 'undefined') {
    globalThis.ASSETS = env.ASSETS;
  }
  for (const key of [
    'SECRET_SLACK_WEBHOOK_URL',
    'SECRET_TELEGRAM_API_TOKEN',
    'SECRET_TELEGRAM_CHAT_ID',
    'SECRET_DISCORD_WEBHOOK_URL',
  ]) {
    if (env[key] !== undefined && typeof globalThis[key] === 'undefined') {
      globalThis[key] = env[key];
    }
  }
}

function installStaticContentFromAssets(env) {
  if (typeof globalThis.__STATIC_CONTENT !== 'undefined') return;
  const assets = env && env.ASSETS;
  if (!assets || typeof assets.fetch !== 'function') return;
  globalThis.__STATIC_CONTENT = {
    async get(key) {
      const path = String(key || '').replace(/^\/+/, '');
      const res = await assets.fetch(new Request('https://assets.local/' + path));
      if (!res.ok) return null;
      return res.arrayBuffer();
    },
  };
  if (typeof globalThis.__STATIC_CONTENT_MANIFEST === 'undefined') {
    globalThis.__STATIC_CONTENT_MANIFEST = '{}';
  }
}

installListenerShim();
await import('./main.js');

export default {
  async fetch(request, env, ctx) {
    aliasBindings(env);
    installStaticContentFromAssets(env);
    let respond;
    const done = new Promise((resolve) => {
      respond = resolve;
    });
    const ev = dispatch('fetch', {
      request,
      respondWith(p) {
        respond(Promise.resolve(p));
      },
    });
    if (ctx && typeof ctx.waitUntil === 'function') {
      const prev = ev.waitUntil.bind(ev);
      ev.waitUntil = (p) => {
        ctx.waitUntil(p);
        prev(p);
      };
    }
    return done;
  },

  async scheduled(controller, env, ctx) {
    aliasBindings(env);
    const ev = dispatch('scheduled', {
      scheduledTime: controller && controller.scheduledTime,
      cron: controller && controller.cron,
    });
    if (ctx && typeof ctx.waitUntil === 'function') {
      const prev = ev.waitUntil.bind(ev);
      ev.waitUntil = (p) => {
        ctx.waitUntil(p);
        prev(p);
      };
    }
    return;
  },
};
