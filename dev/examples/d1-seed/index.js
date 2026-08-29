const SCHEMA = `
CREATE TABLE IF NOT EXISTS entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  message TEXT NOT NULL,
  at INTEGER NOT NULL
);
`;

export default {
  async fetch(request, env) {
    await env.DB.exec(SCHEMA);
    const url = new URL(request.url);
    const count = await env.DB
      .prepare("SELECT count(*) AS n FROM entries")
      .first("n");
    if (url.pathname === "/count") return Response.json({ count });
    return Response.json({ count });
  },
};
