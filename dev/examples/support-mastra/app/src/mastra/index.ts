import { installOpenCodeFetchShim } from './opencode-fetch-shim';

installOpenCodeFetchShim();

import { env } from 'cloudflare:workers';
import { Mastra } from '@mastra/core';
import { D1Store } from '@mastra/cloudflare-d1';
import { CloudflareDeployer } from '@mastra/deployer-cloudflare';
import { weatherAgent } from './agents/weather-agent';
import { forecastCacheTool, weatherWorkflow } from './workflows/weather-workflow';

type WorkerEnv = {
  DB: D1Database;
};

export const mastra = new Mastra({
  deployer: new CloudflareDeployer({
    name: 'support-mastra',
    vars: {
      NODE_ENV: 'production',
    },
  }),
  storage: new D1Store({
    id: 'support-mastra-storage',
    binding: (env as WorkerEnv).DB,
  }),
  agents: { weatherAgent },
  tools: { forecastCacheTool },
  workflows: { weatherWorkflow },
  server: {
    build: {
      openAPIDocs: false,
      swaggerUI: false,
    },
  },
});
