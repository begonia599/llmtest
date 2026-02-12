import type {
  OpenAIRequest, OpenAIMessage, OpenAIResponse, OpenAIChoice,
  GeminiRequest, GeminiContent, GeminiPart, GeminiResponse,
} from './types.js';
import { cleanSchemaForGemini } from './schema.js';

export function extractTextContent(content: any): string {
  if (typeof content === 'string') return content;
  if (!content) return '';
  if (Array.isArray(content)) {
    return content.filter((p: any) => p.text).map((p: any) => p.text).join('');
  }
  return String(content);
}

export function openaiToGemini(req: OpenAIRequest): GeminiRequest {
  const gemReq: GeminiRequest = { contents: [] };

  for (const msg of req.messages) {
    switch (msg.role) {
      case 'system':
        gemReq.systemInstruction = {
          parts: [{ text: extractTextContent(msg.content) }],
          role: 'user',
        };
        break;
      case 'user':
        gemReq.contents.push({
          parts: [{ text: extractTextContent(msg.content) }],
          role: 'user',
        });
        break;
      case 'assistant': {
        const parts: GeminiPart[] = [];
        const text = extractTextContent(msg.content);
        if (text) parts.push({ text });
        if (msg.tool_calls) {
          for (const tc of msg.tool_calls) {
            let args: Record<string, any> = {};
            try { args = JSON.parse(tc.function.arguments); } catch {}
            parts.push({ functionCall: { name: tc.function.name, args } });
          }
        }
        if (parts.length > 0) gemReq.contents.push({ parts, role: 'model' });
        break;
      }
      case 'tool': {
        const text = extractTextContent(msg.content);
        let respData: Record<string, any>;
        try { respData = JSON.parse(text); } catch { respData = { result: text }; }
        gemReq.contents.push({
          parts: [{ functionResponse: { name: msg.name || '', response: respData } }],
          role: 'user',
        });
        break;
      }
    }
  }

  // Generation config
  const genConfig: Record<string, any> = {};
  if (req.temperature !== undefined) genConfig.temperature = req.temperature;
  if (req.top_p !== undefined) genConfig.topP = req.top_p;
  if (req.max_tokens !== undefined) genConfig.maxOutputTokens = req.max_tokens;
  if (req.stop?.length) genConfig.stopSequences = req.stop;
  if (Object.keys(genConfig).length > 0) gemReq.generationConfig = genConfig;

  // Tools
  if (req.tools?.length) {
    const decls = req.tools.map(t => ({
      name: t.function.name,
      description: t.function.description,
      parameters: cleanSchemaForGemini(t.function.parameters),
    }));
    gemReq.tools = [{ functionDeclarations: decls }];
  }

  // Tool choice
  if (req.tool_choice) {
    let mode = 'AUTO';
    if (typeof req.tool_choice === 'string') {
      const map: Record<string, string> = { auto: 'AUTO', none: 'NONE', required: 'ANY' };
      mode = map[req.tool_choice] || 'AUTO';
    }
    gemReq.toolConfig = { functionCallingConfig: { mode } };
  }

  return gemReq;
}

function mapFinishReason(r?: string): string | null {
  if (!r) return null;
  const map: Record<string, string> = {
    STOP: 'stop', MAX_TOKENS: 'length', SAFETY: 'content_filter', RECITATION: 'content_filter',
  };
  return map[r] || 'stop';
}

export function geminiToOpenai(gem: GeminiResponse, model: string, reqID: string): OpenAIResponse {
  const choices: OpenAIChoice[] = gem.candidates.map(cand => {
    const msg: OpenAIMessage = { role: 'assistant', content: null };
    const texts: string[] = [];

    for (const part of cand.content.parts) {
      if (part.text) texts.push(part.text);
      if (part.functionCall) {
        if (!msg.tool_calls) msg.tool_calls = [];
        msg.tool_calls.push({
          id: `call_${part.functionCall.name}`,
          type: 'function',
          function: { name: part.functionCall.name, arguments: JSON.stringify(part.functionCall.args) },
        });
      }
    }
    if (texts.length > 0) msg.content = texts.join('');

    return {
      index: cand.index,
      message: msg,
      finish_reason: mapFinishReason(cand.finishReason),
    };
  });

  const resp: OpenAIResponse = {
    id: reqID, object: 'chat.completion', created: Math.floor(Date.now() / 1000), model, choices,
  };
  if (gem.usageMetadata) {
    resp.usage = {
      prompt_tokens: gem.usageMetadata.promptTokenCount,
      completion_tokens: gem.usageMetadata.candidatesTokenCount,
      total_tokens: gem.usageMetadata.totalTokenCount,
    };
  }
  return resp;
}

export function geminiChunkToOpenaiChunk(gem: GeminiResponse, model: string, reqID: string): OpenAIResponse {
  const choices: OpenAIChoice[] = gem.candidates.map(cand => {
    const delta: OpenAIMessage = { role: 'assistant', content: null };
    const texts: string[] = [];

    for (const part of cand.content.parts) {
      if (part.text) texts.push(part.text);
      if (part.functionCall) {
        if (!delta.tool_calls) delta.tool_calls = [];
        delta.tool_calls.push({
          id: `call_${part.functionCall.name}`,
          type: 'function',
          function: { name: part.functionCall.name, arguments: JSON.stringify(part.functionCall.args) },
        });
      }
    }
    if (texts.length > 0) delta.content = texts.join('');

    return {
      index: cand.index,
      delta,
      finish_reason: mapFinishReason(cand.finishReason),
    };
  });

  const resp: OpenAIResponse = {
    id: reqID, object: 'chat.completion.chunk', created: Math.floor(Date.now() / 1000), model, choices,
  };
  if (gem.usageMetadata) {
    resp.usage = {
      prompt_tokens: gem.usageMetadata.promptTokenCount,
      completion_tokens: gem.usageMetadata.candidatesTokenCount,
      total_tokens: gem.usageMetadata.totalTokenCount,
    };
  }
  return resp;
}

export type { OpenAIRequest, OpenAIResponse, GeminiRequest, GeminiResponse, GeminiContent, GeminiPart };
