/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_CELLP_API_URL?: string;
  readonly VITE_CELLP_ADMIN_TOKEN?: string;
  readonly VITE_CELLP_GATEWAY_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
