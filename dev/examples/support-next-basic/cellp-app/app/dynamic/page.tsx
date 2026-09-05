export const dynamic = "force-dynamic";

export default function DynamicPage() {
  const ts = new Date().toISOString();
  return (
    <main>
      <h1>S40 dynamic route</h1>
      <p data-cellp-ts={ts}>Rendered at: {ts}</p>
    </main>
  );
}
