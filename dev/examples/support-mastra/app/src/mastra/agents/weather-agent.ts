import { createOpenAI } from '@ai-sdk/openai';
import { Agent } from '@mastra/core/agent';
import { Memory } from '@mastra/memory';
import { weatherTool } from '../tools/weather-tool';

const rawModel =
  (typeof process !== 'undefined' && process.env.OPENAI_MODEL?.trim()) ||
  'big-pickle';
const modelId = rawModel.startsWith('openai/')
  ? rawModel.slice('openai/'.length)
  : rawModel;
const openai = createOpenAI();

export const weatherAgent = new Agent({
  id: 'weather-agent',
  name: 'Weather Agent',
  instructions: `You are a helpful weather assistant.
Use weatherTool for current conditions. Keep answers concise.`,
  model: openai.chat(modelId),
  maxRetries: 0,
  tools: { weatherTool },
  memory: new Memory(),
});
