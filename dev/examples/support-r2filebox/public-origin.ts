import type { Context } from 'hono'
import type { Env } from '../types'

/** cellp Gateway: upstream Host is synthetic; browser authority is X-Forwarded-Host. */
export function publicOrigin(c: Context<{ Bindings: Env }>): string {
  const configured = String((c.env as { PUBLIC_BASE_URL?: string }).PUBLIC_BASE_URL || '')
    .trim()
    .replace(/\/$/, '')
  if (configured && !configured.includes('PLACEHOLDER') && !configured.includes('__CELLP')) {
    return configured
  }

  const xfHost = c.req.header('X-Forwarded-Host')?.split(',')[0]?.trim()
  const xfProto = (c.req.header('X-Forwarded-Proto') || 'https').split(',')[0]?.trim() || 'https'
  if (xfHost && !xfHost.startsWith('synthetic.')) {
    return `${xfProto}://${xfHost}`
  }

  const ref = c.req.header('Referer') || c.req.header('Origin') || ''
  if (ref) {
    try {
      const u = new URL(ref)
      return u.origin
    } catch {
      /* ignore */
    }
  }

  try {
    const u = new URL(c.req.url)
    if (!u.hostname.startsWith('synthetic.')) {
      return u.origin
    }
  } catch {
    /* ignore */
  }
  return ''
}

export function publicShareUrl(c: Context<{ Bindings: Env }>, code: string): string {
  const origin = publicOrigin(c)
  if (!origin) {
    return `/#/share/${code}`
  }
  return `${origin}/#/share/${code}`
}
