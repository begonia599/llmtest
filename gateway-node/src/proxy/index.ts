import type { OpenAIRequest, GeminiRequest, GeminiResponse, GeminiContent, GeminiPart } from '../converter/index.js';
import { openaiToGemini, geminiToOpenai, geminiChunkToOpenaiChunk, extractTextContent } from '../converter/index.js';
import { CredentialManager, type Credential } from '../credential/index.js';
import { TokenStats, estimateInputTokens } from '../token/index.js';
import type { FastifyReply } from 'fastify';

const MAX_RETRIES = 3;
const MAX_CONTINUATIONS = 3;
const DONE_MARKER = '[done]';
const COOLDOWN_REGEX = /(?:try again in|retry after|wait)\s+(\d+)\s*(?:seconds?|s)/i;

export class ProxyHandler {
  constructor(
    private upstreamURL: string,
    private credManager: CredentialManager,
    private tokenStats: TokenStats,
  ) {}

  async handleNonStreaming(req: OpenAIRequest, reqID: string, reply: FastifyReply): Promise<void> {
    const gemReq = openaiToGemini(req);
    injectAntiTruncation(gemReq);

    const inputText = req.messages.map(m => extractTextContent(m.content)).join('');
    const inputTokens = estimateInputTokens(inputText);
    const model = req.model;

    let lastErr: Error | null = null;

    for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
      let cred: Credential;
      try {
        cred = await this.credManager.getCredential(model);
      } catch (e: any) {
        lastErr = e;
        continue;
      }

      try {
        const resp = await fetch(
          `${this.upstreamURL}/v1/models/${model}:generateContent`,
          {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${cred.accessToken}`,
            },
            body: JSON.stringify(gemReq),
          },
        );

        if (!resp.ok) {
          const body = await resp.text();
          if (isRetryable(resp.status)) {
            this.credManager.recordError(cred, resp.status, model, parseCooldown(body));
            await backoff(attempt);
            lastErr = new Error(`upstream error (status ${resp.status}): ${body}`);
            continue;
          }
          if (resp.status === 400 || resp.status === 403) {
            this.credManager.recordError(cred, resp.status, model, 0);
          }
          reply.code(resp.status).send({ error: { message: body, type: 'gateway_error', code: resp.status } });
          return;
        }

        const gemResp: GeminiResponse = await resp.json() as GeminiResponse;
        cleanDoneMarker(gemResp);

        const oaiResp = geminiToOpenai(gemResp, model, reqID);
        const outputTokens = gemResp.usageMetadata?.candidatesTokenCount || 0;
        this.tokenStats.record(cred.id, model, inputTokens, outputTokens);

        reply.send(oaiResp);
        return;
      } catch (e: any) {
        lastErr = e;
        await backoff(attempt);
      }
    }

    reply.code(502).send({ error: { message: `all retries exhausted: ${lastErr?.message}`, type: 'gateway_error', code: 502 } });
  }

  async handleStreaming(req: OpenAIRequest, reqID: string, reply: FastifyReply): Promise<void> {
    const gemReq = openaiToGemini(req);
    injectAntiTruncation(gemReq);

    const inputText = req.messages.map(m => extractTextContent(m.content)).join('');
    const inputTokens = estimateInputTokens(inputText);
    const model = req.model;

    reply.raw.writeHead(200, {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      'Connection': 'keep-alive',
      'X-Accel-Buffering': 'no',
    });

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
        for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
          try {
            cred = await this.credManager.getCredential(model);
            acquired = true;
            break;
          } catch {
            await backoff(attempt);
          }
        }
        if (!acquired!) {
          reply.raw.write(`data: {"error": "no credentials available"}\n\n`);
          reply.raw.end();
          return;
        }
        cred = cred!;
      }
      currentCred = cred;

      const currentGemReq = continuation > 0 ? buildContinuation(gemReq, collectedText) : gemReq;

      let resp: Response | null = null;
      for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
        try {
          resp = await fetch(
            `${this.upstreamURL}/v1/models/${model}:streamGenerateContent`,
            {
              method: 'POST',
              headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${cred.accessToken}`,
              },
              body: JSON.stringify(currentGemReq),
            },
          );

          if (!resp.ok) {
            const body = await resp.text();
            const status = resp.status;
            if (isRetryable(status)) {
              this.credManager.recordError(cred, status, model, parseCooldown(body));
              try {
                cred = await this.credManager.preWarmCredential(model, cred.id);
                currentCred = cred;
              } catch {}
              await backoff(attempt);
              resp = null;
              continue;
            }
            if (status === 400 || status === 403) {
              this.credManager.recordError(cred, status, model, 0);
            }
            reply.raw.write(`data: {"error": "upstream error: ${body}"}\n\n`);
            reply.raw.end();
            return;
          }
          break;
        } catch (e) {
          await backoff(attempt);
          resp = null;
        }
      }

      if (!resp || !resp.body) {
        reply.raw.write(`data: {"error": "upstream request failed"}\n\n`);
        reply.raw.end();
        return;
      }

      // Process SSE stream
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
          const data = line.slice(6);

          let gemResp: GeminiResponse;
          try {
            gemResp = JSON.parse(data);
          } catch {
            continue;
          }

          const chunkText = extractChunkText(gemResp);
          if (chunkText.includes(DONE_MARKER)) {
            foundDone = true;
            cleanDoneMarker(gemResp);
          }
          collectedText += chunkText.replace(DONE_MARKER, '');

          if (gemResp.usageMetadata) {
            totalOutputTokens = gemResp.usageMetadata.candidatesTokenCount;
          }

          const oaiChunk = geminiChunkToOpenaiChunk(gemResp, model, reqID);
          reply.raw.write(`data: ${JSON.stringify(oaiChunk)}\n\n`);
        }
      }

      if (foundDone) break;
    }

    reply.raw.write('data: [DONE]\n\n');
    reply.raw.end();

    if (currentCred) {
      this.tokenStats.record(currentCred.id, model, inputTokens, totalOutputTokens);
    }
  }
}

function injectAntiTruncation(req: GeminiRequest): void {
  const instruction = `When you have completed your full response, you must output ${DONE_MARKER} on a separate line at the very end. Only output ${DONE_MARKER} when your answer is complete.`;
  if (req.systemInstruction) {
    if (req.systemInstruction.parts.length > 0) {
      req.systemInstruction.parts[0].text = (req.systemInstruction.parts[0].text || '') + '\n\n' + instruction;
    }
  } else {
    req.systemInstruction = { parts: [{ text: instruction }], role: 'user' };
  }
}

function buildContinuation(original: GeminiRequest, collectedText: string): GeminiRequest {
  const suffix = collectedText.length > 100 ? collectedText.slice(-100) : collectedText;
  const continuation = `Continue from where you left off. You have already output approximately ${collectedText.length} characters ending with:\n"...${suffix}"\n\nContinue outputting the remaining content:`;

  return {
    ...original,
    contents: [
      ...original.contents,
      { parts: [{ text: collectedText }], role: 'model' },
      { parts: [{ text: continuation }], role: 'user' },
    ],
  };
}

function extractChunkText(resp: GeminiResponse): string {
  return resp.candidates.flatMap(c => c.content.parts).filter(p => p.text).map(p => p.text!).join('');
}

function cleanDoneMarker(resp: GeminiResponse): void {
  for (const cand of resp.candidates) {
    for (const part of cand.content.parts) {
      if (part.text) part.text = part.text.replaceAll(DONE_MARKER, '');
    }
  }
}

function isRetryable(status: number): boolean {
  return status === 429 || status === 503;
}

function parseCooldown(msg: string): number {
  const match = COOLDOWN_REGEX.exec(msg);
  return match ? parseInt(match[1], 10) : 0;
}

function backoff(attempt: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, 100 * (1 << attempt)));
}
