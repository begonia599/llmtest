import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter, Rate } from 'k6/metrics';

// Custom metrics
export const ttfb = new Trend('ttfb', true);
export const requestDuration = new Trend('request_duration', true);
export const successRate = new Rate('success_rate');
export const errorCount = new Counter('error_count');
export const sseChunks = new Counter('sse_chunks');

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';

export function makeRequest(preset, stream, latency, errorRate) {
  const body = JSON.stringify({
    ...preset.body,
    stream: stream,
  });

  const headers = {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer test-key',
  };

  if (latency) headers['X-Mock-Latency'] = latency;
  if (errorRate > 0) headers['X-Mock-Error-Rate'] = String(errorRate);

  const params = {
    headers: headers,
    timeout: '60s',
    tags: {
      preset: preset.name,
      stream: String(stream),
    },
  };

  const startTime = Date.now();
  const res = http.post(`${GATEWAY_URL}/v1/chat/completions`, body, params);
  const duration = Date.now() - startTime;

  requestDuration.add(duration);

  if (res.status === 200) {
    successRate.add(1);

    if (stream) {
      // Count SSE chunks
      const chunks = (res.body || '').split('data: ').length - 1;
      sseChunks.add(chunks);
      ttfb.add(res.timings.waiting);
    } else {
      ttfb.add(res.timings.waiting);
      // Validate response structure
      try {
        const data = JSON.parse(res.body);
        check(data, {
          'has choices': (d) => d.choices && d.choices.length > 0,
          'has usage': (d) => d.usage !== undefined,
        });
      } catch (e) {
        // JSON parse failed
      }
    }
  } else {
    successRate.add(0);
    errorCount.add(1);
  }

  check(res, {
    'status is 200': (r) => r.status === 200,
  });

  return res;
}

export function pickRandom(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}
