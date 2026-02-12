// Singleton token stats for Next.js
let _instance: TokenStats | null = null;

export class TokenStats {
  private byCredential = new Map<string, { input: number; output: number; requests: number }>();
  private byModel = new Map<string, { input: number; output: number; requests: number }>();
  private globalInput = 0;
  private globalOutput = 0;
  private globalRequests = 0;

  static getInstance(): TokenStats {
    if (!_instance) _instance = new TokenStats();
    return _instance;
  }

  record(credID: string, model: string, inputTokens: number, outputTokens: number): void {
    this.globalInput += inputTokens;
    this.globalOutput += outputTokens;
    this.globalRequests++;

    const c = this.byCredential.get(credID) || { input: 0, output: 0, requests: 0 };
    c.input += inputTokens; c.output += outputTokens; c.requests++;
    this.byCredential.set(credID, c);

    const m = this.byModel.get(model) || { input: 0, output: 0, requests: 0 };
    m.input += inputTokens; m.output += outputTokens; m.requests++;
    this.byModel.set(model, m);
  }

  getSummary(): Record<string, any> {
    const cs: Record<string, any> = {}, ms: Record<string, any> = {};
    for (const [k, v] of this.byCredential) cs[k] = { input_tokens: v.input, output_tokens: v.output, requests: v.requests };
    for (const [k, v] of this.byModel) ms[k] = { input_tokens: v.input, output_tokens: v.output, requests: v.requests };
    return { global: { input_tokens: this.globalInput, output_tokens: this.globalOutput, requests: this.globalRequests }, by_credential: cs, by_model: ms };
  }
}

export function estimateInputTokens(text: string, imageCount: number = 0): number {
  return Math.max(1, Math.floor(text.length / 4) + imageCount * 300);
}
