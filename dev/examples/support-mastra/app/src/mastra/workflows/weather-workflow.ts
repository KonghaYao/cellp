import { createTool } from '@mastra/core/tools';
import { createStep, createWorkflow } from '@mastra/core/workflows';
import { env } from 'cloudflare:workers';
import { z } from 'zod';

const forecastInputSchema = z.object({
  city: z.string().describe('The city to get the weather for'),
  cacheNamespace: z.string().max(80).optional(),
});

const forecastSchema = z.object({
  date: z.string(),
  maxTemp: z.number(),
  minTemp: z.number(),
  precipitationChance: z.number(),
  condition: z.string(),
  location: z.string(),
  cacheHit: z.boolean().optional(),
});

type WorkerEnv = {
  CACHE?: R2Bucket;
};

function getWeatherCondition(code: number): string {
  const conditions: Record<number, string> = {
    0: 'Clear sky',
    1: 'Mainly clear',
    2: 'Partly cloudy',
    3: 'Overcast',
    61: 'Slight rain',
    63: 'Moderate rain',
    95: 'Thunderstorm',
  };
  return conditions[code] ?? 'Unknown';
}

function cacheKeyForCity(city: string, namespace?: string) {
  const prefix = namespace
    ? `${namespace.trim().toLowerCase().replace(/[^a-z0-9-]/g, '-')}/`
    : '';
  return `forecast/${prefix}${city.trim().toLowerCase().replace(/\s+/g, '-')}.json`;
}

async function getCachedForecast(inputData: z.infer<typeof forecastInputSchema>) {
  if (!inputData?.city) {
    throw new Error('city is required');
  }
  const key = cacheKeyForCity(inputData.city, inputData.cacheNamespace);
  const bucket = (env as WorkerEnv).CACHE;
  if (bucket) {
    const cached = await bucket.get(key);
    if (cached) {
      const parsed = forecastSchema.parse(JSON.parse(await cached.text()));
      return { ...parsed, cacheHit: true };
    }
  }

  const geocodingUrl = `https://geocoding-api.open-meteo.com/v1/search?name=${encodeURIComponent(inputData.city)}&count=1`;
  const geocodingResponse = await fetch(geocodingUrl);
  const geocodingData = (await geocodingResponse.json()) as {
    results: { latitude: number; longitude: number; name: string }[];
  };
  if (!geocodingData.results?.[0]) {
    throw new Error(`Location '${inputData.city}' not found`);
  }
  const { latitude, longitude, name } = geocodingData.results[0];
  const weatherUrl = `https://api.open-meteo.com/v1/forecast?latitude=${latitude}&longitude=${longitude}&current=precipitation,weathercode&timezone=auto&hourly=precipitation_probability,temperature_2m`;
  const response = await fetch(weatherUrl);
  const data = (await response.json()) as {
    current: { weathercode: number };
    hourly: { precipitation_probability: number[]; temperature_2m: number[] };
  };

  const forecast = {
    date: new Date().toISOString(),
    maxTemp: Math.max(...data.hourly.temperature_2m),
    minTemp: Math.min(...data.hourly.temperature_2m),
    condition: getWeatherCondition(data.current.weathercode),
    precipitationChance: data.hourly.precipitation_probability.reduce(
      (acc, curr) => Math.max(acc, curr),
      0,
    ),
    location: name,
    cacheHit: false,
  };

  if (bucket) {
    await bucket.put(key, JSON.stringify(forecast), {
      httpMetadata: { contentType: 'application/json' },
    });
  }

  return forecast;
}

export const forecastCacheTool = createTool({
  id: 'get-forecast-cache',
  description: 'Get a weather forecast through the Workflow R2 cache path',
  inputSchema: forecastInputSchema,
  outputSchema: forecastSchema,
  execute: getCachedForecast,
});

const fetchWeather = createStep({
  id: 'fetch-weather',
  description: 'Fetches weather forecast; caches JSON in R2 (CACHE binding)',
  inputSchema: forecastInputSchema,
  outputSchema: forecastSchema,
  execute: async ({ inputData }) => getCachedForecast(inputData),
});

const planningOutputSchema = z.object({
  activities: z.string(),
  cacheHit: z.boolean().optional(),
  planningSource: z.enum(['agent', 'rate-limit-fallback']),
});

function isRateLimitError(error: unknown): boolean {
  if (typeof error === 'object' && error !== null) {
    const statusCode = Reflect.get(error, 'statusCode');
    const status = Reflect.get(error, 'status');
    if (statusCode === 429 || status === 429) {
      return true;
    }
  }
  return error instanceof Error && /\b429\b|rate.?limit/i.test(error.message);
}

function fallbackActivities(location: string): string {
  return `Model quota is temporarily unavailable for ${location}. Consider a short walk if conditions allow, an indoor museum or cafe, and a weather-aware backup activity.`;
}

const planActivities = createStep({
  id: 'plan-activities',
  description: 'Uses weatherAgent to suggest activities from forecast',
  inputSchema: forecastSchema,
  outputSchema: planningOutputSchema,
  execute: async ({ inputData, mastra }) => {
    const forecast = inputData;
    if (!forecast) {
      throw new Error('Forecast data not found');
    }
    const agent = mastra?.getAgent('weatherAgent');
    if (!agent) {
      throw new Error('weatherAgent not found');
    }
    const prompt = `Suggest 3 short outdoor or indoor activities for ${forecast.location} given: ${JSON.stringify(forecast)}. One paragraph.`;
    try {
      const response = await agent.generate(prompt, { toolChoice: 'none' });
      return {
        activities: response.text,
        cacheHit: forecast.cacheHit,
        planningSource: 'agent' as const,
      };
    } catch (error) {
      if (!isRateLimitError(error)) {
        throw error;
      }
      return {
        activities: fallbackActivities(forecast.location),
        cacheHit: forecast.cacheHit,
        planningSource: 'rate-limit-fallback' as const,
      };
    }
  },
});

export const weatherWorkflow = createWorkflow({
  id: 'weather-workflow',
  inputSchema: forecastInputSchema,
  outputSchema: planningOutputSchema,
})
  .then(fetchWeather)
  .then(planActivities);

weatherWorkflow.commit();
