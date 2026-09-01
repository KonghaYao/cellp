#!/usr/bin/env bash
# Patch Tempik static web for cellp Gateway prefix + relative assets.
set -euo pipefail
WEB="${1:?src/web directory}"

perl -i -pe '
  s|href="/styles.css"|href="styles.css"|;
  s|src="/app.js"|src="app.js"|;
' "${WEB}/index.html"

APP="${WEB}/app.js"
if grep -q TEMPik_API_ROOT "$APP"; then
  echo "already patched ${WEB}"
  exit 0
fi

cat > "${WEB}/app.js.new" <<'JS'
/** cellp gateway: API lives under /support-tempik/vN/ */
const TEMPik_API_ROOT = (() => {
  const m = location.pathname.match(/^(\/support-tempik\/v\d+)/);
  return m ? m[1] : "";
})();
function apiUrl(p) {
  return `${TEMPik_API_ROOT}${p.startsWith("/") ? p : `/${p}`}`;
}

JS
cat "$APP" >> "${WEB}/app.js.new"
mv "${WEB}/app.js.new" "$APP"

perl -i -pe '
  s/async function fetchJson\(url, options = \{\}\) \{/async function fetchJson(url, options = {}) {\n  if (typeof url === "string" \&\& url.startsWith("\/api")) url = apiUrl(url);/ if $. == 1 .. 1;
' "$APP" 2>/dev/null || true

# fallback: insert after fetchJson line
if ! grep -q 'url.startsWith("/api")' "$APP"; then
  perl -i -0pe 's/(async function fetchJson\(url, options = \{\}\) \{\n)/$1  if (typeof url === "string" \&\& url.startsWith("\/api")) url = apiUrl(url);\n/s' "$APP"
fi

echo "patched ${WEB}"
