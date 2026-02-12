export interface Credential {
  id: string;
  accessToken: string;
  refreshToken: string;
  expiry: number;
  disabled: boolean;
  preview: boolean;
  modelCooldowns: Map<string, number>;
  callCount: number;
  errorCount: number;
}

const REFRESH_THRESHOLD_MS = 120_000;

// Singleton credential manager for Next.js (persists across requests)
let _instance: CredentialManager | null = null;

export class CredentialManager {
  private credentials: Credential[];
  private refreshURL: string;

  static getInstance(count: number, refreshURL: string): CredentialManager {
    if (!_instance) _instance = new CredentialManager(count, refreshURL);
    return _instance;
  }

  private constructor(count: number, refreshURL: string) {
    this.refreshURL = refreshURL;
    this.credentials = [];
    for (let i = 0; i < count; i++) {
      this.credentials.push({
        id: `cred_${String(i + 1).padStart(3, '0')}`,
        accessToken: `mock_token_${String(i + 1).padStart(3, '0')}`,
        refreshToken: `mock_refresh_${String(i + 1).padStart(3, '0')}`,
        expiry: Date.now() + (60 + Math.random() * 3540) * 1000,
        disabled: false, preview: false,
        modelCooldowns: new Map(), callCount: 0, errorCount: 0,
      });
    }
  }

  async getCredential(model: string): Promise<Credential> {
    const now = Date.now();
    const available = this.credentials.filter(c => {
      if (c.disabled) return false;
      const cd = c.modelCooldowns.get(model);
      return !(cd && now < cd);
    });
    if (!available.length) throw new Error(`No available credentials for model ${model}`);
    const chosen = available[Math.floor(Math.random() * available.length)];
    if (chosen.expiry - now <= REFRESH_THRESHOLD_MS) await this.refreshToken(chosen);
    chosen.callCount++;
    return chosen;
  }

  async preWarmCredential(model: string, excludeId: string): Promise<Credential> {
    const now = Date.now();
    const available = this.credentials.filter(c => {
      if (c.disabled || c.id === excludeId) return false;
      const cd = c.modelCooldowns.get(model);
      return !(cd && now < cd);
    });
    if (!available.length) throw new Error(`No available credentials`);
    const chosen = available[Math.floor(Math.random() * available.length)];
    if (chosen.expiry - now <= REFRESH_THRESHOLD_MS) await this.refreshToken(chosen);
    chosen.callCount++;
    return chosen;
  }

  private async refreshToken(cred: Credential): Promise<void> {
    const resp = await fetch(this.refreshURL, { method: 'POST' });
    if (!resp.ok) {
      if ([400, 401, 403].includes(resp.status)) { cred.disabled = true; throw new Error('Permanent refresh failure'); }
      throw new Error(`Temporary refresh failure (${resp.status})`);
    }
    const data = await resp.json() as any;
    if (data.access_token) cred.accessToken = data.access_token;
    if (data.expires_in) cred.expiry = Date.now() + data.expires_in * 1000;
  }

  recordError(cred: Credential, statusCode: number, model: string, cooldownSeconds: number): void {
    cred.errorCount++;
    if (statusCode === 429 || statusCode === 503) {
      cred.modelCooldowns.set(model, Date.now() + (cooldownSeconds > 0 ? cooldownSeconds : 30) * 1000);
    } else if (statusCode === 400 || statusCode === 403) {
      cred.disabled = true;
    }
  }

  getStats(): Record<string, any>[] {
    return this.credentials.map(c => ({
      id: c.id, disabled: c.disabled, call_count: c.callCount,
      error_count: c.errorCount, expiry: new Date(c.expiry).toISOString(), cooldowns: c.modelCooldowns.size,
    }));
  }
}
