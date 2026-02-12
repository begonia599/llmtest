export interface Credential {
  id: string;
  accessToken: string;
  refreshToken: string;
  expiry: number; // Unix timestamp ms
  disabled: boolean;
  preview: boolean;
  modelCooldowns: Map<string, number>;
  callCount: number;
  errorCount: number;
}

const REFRESH_THRESHOLD_MS = 120_000;

export class CredentialManager {
  private credentials: Credential[];
  private refreshURL: string;

  constructor(count: number, refreshURL: string) {
    this.refreshURL = refreshURL;
    this.credentials = [];
    for (let i = 0; i < count; i++) {
      this.credentials.push({
        id: `cred_${String(i + 1).padStart(3, '0')}`,
        accessToken: `mock_token_${String(i + 1).padStart(3, '0')}`,
        refreshToken: `mock_refresh_${String(i + 1).padStart(3, '0')}`,
        expiry: Date.now() + (60 + Math.random() * 3540) * 1000,
        disabled: false,
        preview: false,
        modelCooldowns: new Map(),
        callCount: 0,
        errorCount: 0,
      });
    }
  }

  async getCredential(model: string): Promise<Credential> {
    const now = Date.now();
    const available = this.credentials.filter(c => {
      if (c.disabled) return false;
      const cooldown = c.modelCooldowns.get(model);
      if (cooldown && now < cooldown) return false;
      return true;
    });

    if (available.length === 0) {
      throw new Error(`No available credentials for model ${model}`);
    }

    const chosen = available[Math.floor(Math.random() * available.length)];

    if (chosen.expiry - now <= REFRESH_THRESHOLD_MS) {
      await this.refreshToken(chosen);
    }

    chosen.callCount++;
    return chosen;
  }

  async preWarmCredential(model: string, excludeId: string): Promise<Credential> {
    const now = Date.now();
    const available = this.credentials.filter(c => {
      if (c.disabled || c.id === excludeId) return false;
      const cooldown = c.modelCooldowns.get(model);
      if (cooldown && now < cooldown) return false;
      return true;
    });

    if (available.length === 0) {
      throw new Error(`No available credentials for model ${model} (excluding ${excludeId})`);
    }

    const chosen = available[Math.floor(Math.random() * available.length)];

    if (chosen.expiry - now <= REFRESH_THRESHOLD_MS) {
      await this.refreshToken(chosen);
    }

    chosen.callCount++;
    return chosen;
  }

  private async refreshToken(cred: Credential): Promise<void> {
    try {
      const resp = await fetch(this.refreshURL, { method: 'POST' });
      if (!resp.ok) {
        if ([400, 401, 403].includes(resp.status)) {
          cred.disabled = true;
          throw new Error(`Permanent refresh failure (status ${resp.status}), credential disabled`);
        }
        throw new Error(`Temporary refresh failure (status ${resp.status})`);
      }
      const data = await resp.json() as any;
      if (data.access_token) cred.accessToken = data.access_token;
      if (data.expires_in) cred.expiry = Date.now() + data.expires_in * 1000;
    } catch (err) {
      throw err;
    }
  }

  recordError(cred: Credential, statusCode: number, model: string, cooldownSeconds: number): void {
    cred.errorCount++;
    if (statusCode === 429 || statusCode === 503) {
      const cooldownMs = (cooldownSeconds > 0 ? cooldownSeconds : 30) * 1000;
      cred.modelCooldowns.set(model, Date.now() + cooldownMs);
    } else if (statusCode === 400 || statusCode === 403) {
      cred.disabled = true;
    }
  }

  getStats(): Record<string, any>[] {
    return this.credentials.map(c => ({
      id: c.id,
      disabled: c.disabled,
      call_count: c.callCount,
      error_count: c.errorCount,
      expiry: new Date(c.expiry).toISOString(),
      cooldowns: c.modelCooldowns.size,
    }));
  }
}
