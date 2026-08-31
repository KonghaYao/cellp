import { defineConfig } from 'vitepress'

const siteUrl = 'https://konghayao.github.io/cellp/'

export default defineConfig({
  title: 'cellp',
  description:
    'Private, versioned Workers platform. Every deploy versions the app and its data. Preview, promote, self-host.',
  lang: 'en-US',
  base: '/cellp/',
  appearance: 'dark',
  cleanUrls: true,
  lastUpdated: true,
  ignoreDeadLinks: 'localhostLinks',
  sitemap: {
    hostname: siteUrl,
  },
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/cellp/logo.svg' }],
    ['meta', { name: 'theme-color', content: '#0b0f14' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: 'cellp — versioned Workers, on your hardware' }],
    [
      'meta',
      {
        property: 'og:description',
        content:
          'Self-hosted control plane for Cloudflare-style Workers. Preview every deploy with forked D1, KV, R2, and Queues. Promote when ready.',
      },
    ],
    ['meta', { property: 'og:url', content: siteUrl }],
    ['meta', { name: 'twitter:card', content: 'summary' }],
  ],
  themeConfig: {
    logo: { src: '/logo.svg', alt: 'cellp' },
    siteTitle: 'cellp',
    nav: [
      { text: 'Docs', link: '/guides/install', activeMatch: '^/(build|get-started|concepts|guides|bindings)/' },
      { text: 'Why cellp', link: '/why' },
      { text: 'Migrate', link: '/migrate/cloudflare', activeMatch: '^/migrate/' },
      { text: 'API', link: '/reference/api' },
      {
        text: 'GitHub',
        link: 'https://github.com/KonghaYao/cellp',
      },
    ],
    sidebar: {
      '/': [
        {
          text: 'Introduction',
          items: [
            { text: 'What is cellp', link: '/what-is-cellp' },
            { text: 'Why cellp', link: '/why' },
            { text: 'How it works', link: '/how-it-works' },
            { text: 'Compare', link: '/compare' },
          ],
        },
        {
          text: 'Get started',
          items: [
            { text: 'Quick start', link: '/get-started/' },
            { text: 'Local stack', link: '/get-started/local' },
            { text: 'Example app', link: '/get-started/example' },
            { text: 'Dashboard', link: '/get-started/dashboard' },
            { text: 'Operator journey', link: '/get-started/operator-journey' },
          ],
        },
        {
          text: 'Build your app',
          items: [
            { text: 'Write a Worker', link: '/build/' },
            { text: 'Configure bindings', link: '/build/wrangler' },
            { text: 'Platform data', link: '/build/data' },
            { text: 'Handlers', link: '/build/handlers' },
          ],
        },
        {
          text: 'Concepts',
          items: [
            { text: 'Projects', link: '/concepts/projects' },
            { text: 'Versions', link: '/concepts/versions' },
            { text: 'Preview & production', link: '/concepts/preview' },
            { text: 'Promote', link: '/concepts/promote' },
            { text: 'Archive & wake', link: '/concepts/archive' },
            { text: 'Bindings', link: '/concepts/bindings' },
          ],
        },
        {
          text: 'Guides',
          items: [
            { text: 'Install', link: '/guides/install' },
            { text: 'cellp dev', link: '/guides/dev' },
            { text: 'Deploy from CI', link: '/guides/ci' },
            { text: 'Environment variables', link: '/guides/environment-variables' },
            { text: 'Self-hosting', link: '/guides/self-hosting' },
            { text: 'Observability', link: '/guides/observability' },
            { text: 'Rollback', link: '/guides/rollback' },
          ],
        },
        {
          text: 'Bindings',
          items: [
            { text: 'D1', link: '/bindings/d1' },
            { text: 'KV', link: '/bindings/kv' },
            { text: 'R2', link: '/bindings/r2' },
            { text: 'Queues', link: '/bindings/queues' },
            { text: 'Workflows', link: '/bindings/workflows' },
            { text: 'Cron', link: '/bindings/cron' },
          ],
        },
        {
          text: 'Migrate',
          items: [
            { text: 'From Cloudflare', link: '/migrate/cloudflare' },
            { text: 'From Vercel', link: '/migrate/vercel' },
            { text: 'Supported stacks', link: '/migrate/stacks' },
          ],
        },
        {
          text: 'Reference',
          items: [
            { text: 'REST API', link: '/reference/api' },
            { text: 'Auth & tokens', link: '/reference/auth' },
            { text: 'Limits', link: '/reference/limits' },
            { text: 'Repository map', link: '/reference/repo' },
          ],
        },
      ],
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/KonghaYao/cellp' },
    ],
    search: {
      provider: 'local',
    },
    editLink: {
      pattern: 'https://github.com/KonghaYao/cellp/edit/main/site/docs/:path',
      text: 'Edit this page',
    },
    outline: { level: [2, 3] },
    footer: {
      message: 'Self-hosted Workers control plane. Not affiliated with Cloudflare or Vercel.',
      copyright: 'cellp · KonghaYao',
    },
  },
})
