export class TokenStats {
  private byCredential = new Map<string, { input: number; output: number; requests: number }>();
  private byModel = new Map<string, { input: number; output: number; requests: number }>();
  private globalInput = 0;
  private globalOutput = 0;
  private globalRequests = 0;

  record(credID: string, model: string, inputTokens: number, outputTokens: number): void {
    this.globalInput += inputTokens;
    this.globalOutput += outputTokens;
    this.globalRequests++;

    const cred = this.byCredential.get(credID) || { input: 0, output: 0, requests: 0 };
    cred.input += inputTokens;
    cred.output += outputTokens;
    cred.requests++;
    this.byCredential.set(credID, cred);

    const mdl = this.byModel.get(model) || { input: 0, output: 0, requests: 0 };
    mdl.input += inputTokens;
    mdl.output += outputTokens;
    mdl.requests++;
    this.byModel.set(model, mdl);
  }

  getSummary(): Record<string, any> {
    const credStats: Record<string, any> = {};
    for (const [k, v] of this.byCredential) {
      credStats[k] = { input_tokens: v.input, output_tokens: v.output, requests: v.requests };
    }
    const modelStats: Record<string, any> = {};
    for (const [k, v] of this.byModel) {
      modelStats[k] = { input_tokens: v.input, output_tokens: v.output, requests: v.requests };
    }
    return {
      global: { input_tokens: this.globalInput, output_tokens: this.globalOutput, requests: this.globalRequests },
      by_credential: credStats,
      by_model: modelStats,
    };
  }
}

export function estimateInputTokens(text: string, imageCount: number = 0): number {
  return Math.max(1, Math.floor(text.length / 4) + imageCount * 300);
}
