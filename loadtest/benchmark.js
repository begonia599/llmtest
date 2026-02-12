// Unified parameterized benchmark script
// All parameters controlled via environment variables:
//
//   k6 run -e GATEWAY_URL=http://server-b:8080 \
//          -e VUS=100 \
//          -e DURATION=60s \
//          -e STREAM_RATIO=0.7 \
//          -e LATENCY=medium \
//          -e ERROR_RATE=0.05 \
//          -e PRESET=all \
//          -e RAMP=false \
//          benchmark.js
//
// Parameters:
//   GATEWAY_URL   - Gateway endpoint (default: http://localhost:8080)
//   VUS           - Number of virtual users / concurrency (default: 10)
//   DURATION      - Test duration (default: 60s)
//   STREAM_RATIO  - 0.0 = all non-streaming, 1.0 = all streaming (default: 0.7)
//   LATENCY       - fast|medium|slow|realistic (default: medium)
//   ERROR_RATE    - 0.0-1.0 mock error injection rate (default: 0)
//   PRESET        - all|short_qa|code_generation|long_text|tool_call|multi_turn (default: all)
//   RAMP          - true = ramp from 1 to VUS over duration (default: false)
//
// Examples:
//   # 10 users, mostly streaming, medium latency
//   k6 run -e VUS=10 -e DURATION=60s benchmark.js
//
//   # 1000 users, all streaming, realistic latency, 5% errors
//   k6 run -e VUS=1000 -e DURATION=120s -e STREAM_RATIO=1.0 -e LATENCY=realistic -e ERROR_RATE=0.05 benchmark.js
//
//   # Ramp from 1 to 5000 users over 5 minutes
//   k6 run -e VUS=5000 -e DURATION=300s -e RAMP=true benchmark.js
//
//   # Only long text preset, 50 users, slow latency
//   k6 run -e VUS=50 -e PRESET=long_text -e LATENCY=slow benchmark.js

import http from 'k6/http';
import { check } from 'k6';
import { Trend, Counter, Rate } from 'k6/metrics';

// ── Metrics ──────────────────────────────────────────────
const ttfb = new Trend('ttfb', true);
const requestDuration = new Trend('request_duration', true);
const successRate = new Rate('success_rate');
const errorCount = new Counter('error_count');
const sseChunks = new Counter('sse_chunks');

// ── Config ───────────────────────────────────────────────
const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';
const VUS = parseInt(__ENV.VUS || '10', 10);
const DURATION = __ENV.DURATION || '60s';
const STREAM_RATIO = parseFloat(__ENV.STREAM_RATIO || '0.7');
const LATENCY = __ENV.LATENCY || 'medium';
const ERROR_RATE = parseFloat(__ENV.ERROR_RATE || '0');
const PRESET_FILTER = __ENV.PRESET || 'all';
const RAMP = (__ENV.RAMP || 'false') === 'true';

// ── Presets ──────────────────────────────────────────────
const ALL_PRESETS = [
  {
    name: 'short_qa',
    body: {
      model: 'gemini-2.0-flash',
      messages: [
        { role: 'system', content: 'You are a helpful assistant.' },
        { role: 'user', content: 'What is the capital of France?' },
      ],
      temperature: 0.7,
      max_tokens: 256,
    },
  },
  {
    name: 'code_generation',
    body: {
      model: 'gemini-2.0-flash',
      messages: [
        { role: 'system', content: 'You are an expert programmer. Write clean, efficient code.' },
        { role: 'user', content: 'Write a Go function that implements a concurrent-safe LRU cache with TTL support.' },
      ],
      temperature: 0.7,
      max_tokens: 2048,
    },
  },
  {
    name: 'long_text',
    body: {
      model: 'gemini-2.0-flash',
      messages: [
        { role: 'system', content: 'You are a knowledgeable writer. Provide detailed, comprehensive responses.' },
        { role: 'user', content: 'Explain the history and evolution of computer networking from ARPANET to modern cloud computing.' },
      ],
      temperature: 0.7,
      max_tokens: 4096,
    },
  },
  {
    name: 'tool_call',
    body: {
      model: 'gemini-2.0-flash',
      messages: [
        { role: 'system', content: 'You are a helpful assistant with access to tools. Use tools when appropriate.' },
        { role: 'user', content: "What's the current weather in Tokyo and New York?" },
      ],
      temperature: 0.7,
      max_tokens: 1024,
      tools: [
        {
          type: 'function',
          function: {
            name: 'get_weather',
            description: 'Get the current weather for a location',
            parameters: {
              type: 'object',
              properties: {
                locations: { type: 'array', items: { type: 'string' }, description: 'List of city names' },
                units: { type: 'string', enum: ['celsius', 'fahrenheit'] },
              },
              required: ['locations'],
            },
          },
        },
      ],
    },
  },
  {
    name: 'multi_turn',
    body: {
      model: 'gemini-2.0-flash',
      messages: [
        { role: 'system', content: 'You are an expert data analyst. Help users understand their data.' },
        { role: 'user', content: 'I have a dataset of 10,000 customer transactions. How should I start analyzing it?' },
        { role: 'assistant', content: 'Great question! Start with exploratory data analysis (EDA).' },
        { role: 'user', content: "I found that 15% of records have missing values in the 'category' field. What should I do?" },
        { role: 'assistant', content: '15% is significant. Investigate if the missingness is random.' },
        { role: 'user', content: "The missing values seem random. I'll use mode imputation. Now I want to segment customers. What clustering approach do you recommend?" },
      ],
      temperature: 0.7,
      max_tokens: 2048,
    },
  },
];

// Filter presets
const PRESETS = PRESET_FILTER === 'all'
  ? ALL_PRESETS
  : ALL_PRESETS.filter(p => p.name === PRESET_FILTER);

if (PRESETS.length === 0) {
  throw new Error(`Unknown preset: ${PRESET_FILTER}. Available: all, short_qa, code_generation, long_text, tool_call, multi_turn`);
}

// ── k6 Options ───────────────────────────────────────────
export const options = (() => {
  if (RAMP) {
    // Parse duration string to seconds
    const durationMatch = DURATION.match(/^(\d+)(s|m|h)?$/);
    const durationSec = durationMatch
      ? parseInt(durationMatch[1]) * ({ s: 1, m: 60, h: 3600 }[durationMatch[2] || 's'] || 1)
      : 60;

    const rampUp = Math.floor(durationSec * 0.3);
    const hold = Math.floor(durationSec * 0.5);
    const rampDown = durationSec - rampUp - hold;

    return {
      scenarios: {
        benchmark: {
          executor: 'ramping-vus',
          startVUs: 1,
          stages: [
            { duration: `${rampUp}s`, target: VUS },
            { duration: `${hold}s`, target: VUS },
            { duration: `${rampDown}s`, target: 1 },
          ],
        },
      },
    };
  }

  return {
    scenarios: {
      benchmark: {
        executor: 'constant-vus',
        vus: VUS,
        duration: DURATION,
      },
    },
  };
})();

// ── Main ─────────────────────────────────────────────────
export default function () {
  const preset = PRESETS[Math.floor(Math.random() * PRESETS.length)];
  const stream = Math.random() < STREAM_RATIO;

  const body = JSON.stringify({
    ...preset.body,
    stream: stream,
  });

  const headers = {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer test-key',
  };

  if (LATENCY) headers['X-Mock-Latency'] = LATENCY;
  if (ERROR_RATE > 0) headers['X-Mock-Error-Rate'] = String(ERROR_RATE);

  const params = {
    headers,
    timeout: '120s',
    tags: { preset: preset.name, stream: String(stream) },
  };

  const res = http.post(`${GATEWAY_URL}/v1/chat/completions`, body, params);

  requestDuration.add(res.timings.duration);

  if (res.status === 200) {
    successRate.add(1);
    ttfb.add(res.timings.waiting);

    if (stream) {
      const chunks = (res.body || '').split('data: ').length - 1;
      sseChunks.add(chunks);
    } else {
      try {
        const data = JSON.parse(res.body);
        check(data, {
          'has choices': (d) => d.choices && d.choices.length > 0,
          'has usage': (d) => d.usage !== undefined,
        });
      } catch (_) {}
    }
  } else {
    successRate.add(0);
    errorCount.add(1);
  }

  check(res, { 'status is 200': (r) => r.status === 200 });
}

// ── Summary ──────────────────────────────────────────────
export function handleSummary(data) {
  const summary = {
    config: { vus: VUS, duration: DURATION, stream_ratio: STREAM_RATIO, latency: LATENCY, error_rate: ERROR_RATE, preset: PRESET_FILTER, ramp: RAMP },
    metrics: {},
  };

  for (const [name, metric] of Object.entries(data.metrics)) {
    if (metric.values) summary.metrics[name] = metric.values;
  }

  const filename = __ENV.SUMMARY_FILE || 'summary.json';
  return {
    [filename]: JSON.stringify(summary, null, 2),
    stdout: textSummary(data),
  };
}

function textSummary(data) {
  const m = data.metrics;
  const lines = [
    '',
    `═══════════════════════════════════════════════════════`,
    `  LLM Gateway Benchmark Results`,
    `  VUS: ${VUS}  Duration: ${DURATION}  Stream: ${(STREAM_RATIO * 100).toFixed(0)}%`,
    `  Latency: ${LATENCY}  ErrorRate: ${(ERROR_RATE * 100).toFixed(0)}%  Ramp: ${RAMP}`,
    `═══════════════════════════════════════════════════════`,
  ];

  if (m.http_reqs?.values) {
    lines.push(`  RPS:            ${m.http_reqs.values.rate?.toFixed(2) || '-'}`);
    lines.push(`  Total requests: ${m.http_reqs.values.count || '-'}`);
  }
  if (m.http_req_duration?.values) {
    const d = m.http_req_duration.values;
    lines.push(`  Latency avg:    ${d.avg?.toFixed(2) || '-'} ms`);
    lines.push(`  Latency P50:    ${d.med?.toFixed(2) || '-'} ms`);
    lines.push(`  Latency P95:    ${d['p(95)']?.toFixed(2) || '-'} ms`);
    lines.push(`  Latency P99:    ${d['p(99)']?.toFixed(2) || '-'} ms`);
  }
  if (m.success_rate?.values) {
    lines.push(`  Success rate:   ${(m.success_rate.values.rate * 100)?.toFixed(2) || '-'}%`);
  }
  if (m.sse_chunks?.values) {
    lines.push(`  SSE chunks:     ${m.sse_chunks.values.count || 0}`);
  }

  lines.push(`═══════════════════════════════════════════════════════`);
  lines.push('');

  return lines.join('\n');
}
