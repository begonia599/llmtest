import type { GeminiRequest, GeminiResponse } from '@/lib/converter';
import { openaiToGemini, geminiToOpenai, geminiChunkToOpenaiChunk, extractTextContent } from '@/lib/converter';
import { CredentialManager, type Credential } from '@/lib/credential';
import { TokenStats, estimateInputTokens } from '@/lib/token';
import type { OpenAIRequest } from '@/lib/converter';

const UPSTREAM_URL = process.env.UPSTREAM_URL || 'http://localhost:8081';
const CRED_COUNT = parseInt(process.env.CRED_COUNT || '20', 10);
const REFRESH_URL = `${UPSTREAM_URL}/oauth2/token`;

const MAX_RETRIES = 3;
const MAX_CONTINUATIONS = 3;
const DONE_MARKER = '[done]';
const COOLDOWN_REGEX = /(?:try again in|retry after|wait)\s+(\d+)\s*(?:seconds?|s)/i;

function getCredManager() { return CredentialManager.getInstance(CRED_COUNT, REFRESH_URL); }
function getTokenStats() { return TokenStats.getInstance(); }

function backoff(attempt: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, 100 * (1 << attempt)));
}

function isRetryable(status: number) { return status === 429 || status === 503; }

function parseCooldown(msg: string): number {
  const m = COOLDOWN_REGEX.exec(msg);
  return m ? parseInt(m[1], 10) : 0;
}

function injectAntiTruncation(req: GeminiRequest): void {
  const inst = `When you have completed your full response, you must output ${DONE_MARKER} on a separate line at the very end. Only output ${DONE_MARKER} when your answer is complete.`;
  if (req.systemInstruction) {
    if (req.systemInstruction.parts.length) req.systemInstruction.parts[0].text = (req.systemInstruction.parts[0].text || '') + '\n\n' + inst;
  } else {
    req.systemInstruction = { parts: [{ text: inst }], role: 'user' };
  }
}

function cleanDoneMarker(resp: GeminiResponse): void {
  for (const c of resp.candidates) for (const p of c.content.parts) if (p.text) p.text = p.text.replaceAll(DONE_MARKER, '');
}

function extractChunkText(resp: GeminiResponse): string {
  return resp.candidates.flatMap(c => c.content.parts).filter(p => p.text).map(p => p.text!).join('');
}

function buildContinuation(original: GeminiRequest, collected: string): GeminiRequest {
  const suffix = collected.length > 100 ? collected.slice(-100) : collected;
  return {
    ...original,
    contents: [
      ...original.contents,
      { parts: [{ text: collected }], role: 'model' },
      { parts: [{ text: `Continue from where you left off. You have already output approximately ${collected.length} characters ending with:\n"...${suffix}"\n\nContinue:` }], role: 'user' },
    ],
  };
}

export async function handleNonStreaming(oaiReq: OpenAIRequest): Promise<Response> {
  const credManager = getCredManager();
  const tokenStats = getTokenStats();
  const gemReq = openaiToGemini(oaiReq);
  injectAntiTruncation(gemReq);

  const inputText = oaiReq.messages.map(m => extractTextContent(m.content)).join('');
  const inputTokens = estimateInputTokens(inputText);
  const model = oaiReq.model;
  const reqID = `chatcmpl-${Date.now()}`;

  let lastErr = '';
  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    let cred: Credential;
    try { cred = await credManager.getCredential(model); } catch (e: any) { lastErr = e.message; continue; }

    const resp = await fetch(`${UPSTREAM_URL}/v1/models/${model}:generateContent`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${cred.accessToken}` },
      body: JSON.stringify(gemReq),
    });

    if (!resp.ok) {
      const body = await resp.text();
      if (isRetryable(resp.status)) {
        credManager.recordError(cred, resp.status, model, parseCooldown(body));
        await backoff(attempt);
        lastErr = body;
        continue;
      }
      if (resp.status === 400 || resp.status === 403) credManager.recordError(cred, resp.status, model, 0);
      return Response.json({ error: { message: body, type: 'gateway_error', code: resp.status } }, { status: resp.status });
    }

    const gemResp: GeminiResponse = await resp.json() as any;
    cleanDoneMarker(gemResp);
    const oaiResp = geminiToOpenai(gemResp, model, reqID);
    tokenStats.record(cred.id, model, inputTokens, gemResp.usageMetadata?.candidatesTokenCount || 0);
    return Response.json(oaiResp);
  }

  return Response.json({ error: { message: `all retries exhausted: ${lastErr}`, type: 'gateway_error', code: 502 } }, { status: 502 });
}

export async function handleStreaming(oaiReq: OpenAIRequest): Promise<Response> {
  const credManager = getCredManager();
  const tokenStats = getTokenStats();
  const gemReq = openaiToGemini(oaiReq);
  injectAntiTruncation(gemReq);

  const inputText = oaiReq.messages.map(m => extractTextContent(m.content)).join('');
  const inputTokens = estimateInputTokens(inputText);
  const model = oaiReq.model;
  const reqID = `chatcmpl-${Date.now()}`;

  const encoder = new TextEncoder();

  const stream = new ReadableStream({
    async start(controller) {
      let collectedText = '';
      let foundDone = false;
      let totalOutputTokens = 0;
      let currentCred: Credential | null = null;

      for (let continuation = 0; continuation <= MAX_CONTINUATIONS; continuation++) {
        let cred: Credential;
        if (currentCred && continuation > 0) {
          cred = currentCred;
        } else {
          let acquired = false;
          for (let a = 0; a <= MAX_RETRIES; a++) {
            try { cred = await credManager.getCredential(model); acquired = true; break; } catch { await backoff(a); }
          }
          if (!acquired!) { controller.enqueue(encoder.encode(`data: {"error":"no credentials"}\n\n`)); controller.close(); return; }
          cred = cred!;
        }
        currentCred = cred;

        const currentGemReq = continuation > 0 ? buildContinuation(gemReq, collectedText) : gemReq;

        let resp: globalThis.Response | null = null;
        for (let a = 0; a <= MAX_RETRIES; a++) {
          try {
            resp = await fetch(`${UPSTREAM_URL}/v1/models/${model}:streamGenerateContent`, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${cred.accessToken}` },
              body: JSON.stringify(currentGemReq),
            });
            if (!resp.ok) {
              const body = await resp.text();
              if (isRetryable(resp.status)) {
                credManager.recordError(cred, resp.status, model, parseCooldown(body));
                try { cred = await credManager.preWarmCredential(model, cred.id); currentCred = cred; } catch {}
                await backoff(a);
                resp = null;
                continue;
              }
              if (resp.status === 400 || resp.status === 403) credManager.recordError(cred, resp.status, model, 0);
              controller.enqueue(encoder.encode(`data: {"error":"upstream error: ${body}"}\n\n`));
              controller.close();
              return;
            }
            break;
          } catch { await backoff(a); resp = null; }
        }

        if (!resp?.body) { controller.enqueue(encoder.encode(`data: {"error":"request failed"}\n\n`)); controller.close(); return; }

        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';

          for (const line of lines) {
            if (!line.startsWith('data: ')) continue;
            let gemResp: GeminiResponse;
            try { gemResp = JSON.parse(line.slice(6)); } catch { continue; }

            const chunkText = extractChunkText(gemResp);
            if (chunkText.includes(DONE_MARKER)) { foundDone = true; cleanDoneMarker(gemResp); }
            collectedText += chunkText.replace(DONE_MARKER, '');
            if (gemResp.usageMetadata) totalOutputTokens = gemResp.usageMetadata.candidatesTokenCount;

            const oaiChunk = geminiChunkToOpenaiChunk(gemResp, model, reqID);
            controller.enqueue(encoder.encode(`data: ${JSON.stringify(oaiChunk)}\n\n`));
          }
        }

        if (foundDone) break;
      }

      controller.enqueue(encoder.encode('data: [DONE]\n\n'));
      controller.close();

      if (currentCred) tokenStats.record(currentCred.id, model, inputTokens, totalOutputTokens);
    },
  });

  return new Response(stream, {
    headers: {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      'Connection': 'keep-alive',
      'X-Accel-Buffering': 'no',
    },
  });
}
