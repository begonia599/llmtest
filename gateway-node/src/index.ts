import Fastify from 'fastify';
import { CredentialManager } from './credential/index.js';
import { TokenStats } from './token/index.js';
import { ProxyHandler } from './proxy/index.js';
import type { OpenAIRequest } from './converter/index.js';

const port = parseInt(process.env.PORT || '8080', 10);
const upstreamURL = process.env.UPSTREAM_URL || 'http://localhost:8081';
const credCount = parseInt(process.env.CRED_COUNT || '20', 10);

const refreshURL = `${upstreamURL}/oauth2/token`;
const credManager = new CredentialManager(credCount, refreshURL);
const tokenStats = new TokenStats();
const proxyHandler = new ProxyHandler(upstreamURL, credManager, tokenStats);

const app = Fastify({ logger: false });

// OpenAI-compatible chat completions
app.post('/v1/chat/completions', async (request, reply) => {
  const req = request.body as OpenAIRequest;
  const reqID = `chatcmpl-${Date.now()}`;

  if (req.stream) {
    await proxyHandler.handleStreaming(req, reqID, reply);
  } else {
    await proxyHandler.handleNonStreaming(req, reqID, reply);
  }
});

// Model list
app.get('/v1/models', async () => ({
  object: 'list',
  data: [
    { id: 'gemini-2.0-flash', object: 'model', owned_by: 'google' },
    { id: 'gemini-1.5-pro', object: 'model', owned_by: 'google' },
    { id: 'gemini-2.0-flash-thinking', object: 'model', owned_by: 'google' },
  ],
}));

// Metrics
app.get('/metrics', async () => ({
  tokens: tokenStats.getSummary(),
  credentials: credManager.getStats(),
}));

// Health check
app.get('/health', async () => ({ status: 'ok', gateway: 'node' }));

app.listen({ port, host: '0.0.0.0' }).then(() => {
  console.log(`Node.js LLM Gateway starting on :${port}`);
  console.log(`Upstream: ${upstreamURL}`);
  console.log(`Credentials: ${credCount}`);
}).catch(err => {
  console.error(err);
  process.exit(1);
});
