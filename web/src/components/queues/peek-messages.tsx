import type { QueuePeekMessage } from "@/lib/cellp-api";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

function peekBodyBase64(msg: QueuePeekMessage): string {
  if (typeof msg.bodyBase64 === "string") return msg.bodyBase64;
  if (typeof msg.body === "string") return msg.body;
  return "";
}

function decodeUtf8(base64: string): string | null {
  if (!base64) return null;
  try {
    const binary = atob(base64);
    const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
    const utf8 = new TextDecoder("utf-8", { fatal: false }).decode(bytes);
    if (utf8.includes("\uFFFD")) return null;
    return utf8;
  } catch {
    return null;
  }
}

export function PeekMessages({ messages }: { messages: QueuePeekMessage[] }) {
  if (messages.length === 0) {
    return (
      <p className="px-4 py-8 text-center text-sm text-muted-foreground">
        No messages in the peek window.
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead>ID</TableHead>
          <TableHead>Type</TableHead>
          <TableHead>Body (base64)</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {messages.map((msg, index) => {
          const id = typeof msg.id === "string" ? msg.id : `msg-${index}`;
          const contentType =
            typeof msg.contentType === "string" ? msg.contentType : "—";
          const base64 = peekBodyBase64(msg);
          const utf8 = decodeUtf8(base64);
          return (
            <TableRow key={id}>
              <TableCell className="font-mono text-xs">{id}</TableCell>
              <TableCell className="text-muted-foreground">{contentType}</TableCell>
              <TableCell>
                <code className="block break-all font-mono text-xs">{base64 || "—"}</code>
                {utf8 != null && utf8 !== "" && (
                  <p className="mt-1 break-all text-xs text-muted-foreground">
                    {utf8}
                  </p>
                )}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
