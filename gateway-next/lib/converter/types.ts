// Shared types - identical to Node gateway
export interface OpenAIRequest {
  model: string;
  messages: OpenAIMessage[];
  temperature?: number;
  top_p?: number;
  max_tokens?: number;
  stop?: string[];
  stream: boolean;
  tools?: OpenAITool[];
  tool_choice?: any;
}

export interface OpenAIMessage {
  role: string;
  content: any;
  tool_calls?: OpenAIToolCall[];
  tool_call_id?: string;
  name?: string;
}

export interface OpenAITool {
  type: string;
  function: { name: string; description?: string; parameters?: Record<string, any> };
}

export interface OpenAIToolCall {
  id: string;
  type: string;
  function: { name: string; arguments: string };
}

export interface OpenAIResponse {
  id: string;
  object: string;
  created: number;
  model: string;
  choices: OpenAIChoice[];
  usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number };
}

export interface OpenAIChoice {
  index: number;
  message?: OpenAIMessage;
  delta?: OpenAIMessage;
  finish_reason: string | null;
}

export interface GeminiRequest {
  contents: GeminiContent[];
  generationConfig?: Record<string, any>;
  tools?: { functionDeclarations: GeminiFuncDecl[] }[];
  toolConfig?: { functionCallingConfig?: { mode: string } };
  systemInstruction?: GeminiContent;
}

export interface GeminiContent {
  parts: GeminiPart[];
  role: string;
}

export interface GeminiPart {
  text?: string;
  functionCall?: { name: string; args: Record<string, any> };
  functionResponse?: { name: string; response: Record<string, any> };
}

export interface GeminiFuncDecl {
  name: string;
  description?: string;
  parameters?: Record<string, any>;
}

export interface GeminiResponse {
  candidates: {
    content: GeminiContent;
    finishReason?: string;
    index: number;
  }[];
  usageMetadata?: {
    promptTokenCount: number;
    candidatesTokenCount: number;
    totalTokenCount: number;
  };
}
