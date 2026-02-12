import { CredentialManager } from '@/lib/credential';
import { TokenStats } from '@/lib/token';

export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

const UPSTREAM_URL = process.env.UPSTREAM_URL || 'http://localhost:8081';
const CRED_COUNT = parseInt(process.env.CRED_COUNT || '20', 10);

export async function GET() {
  const credManager = CredentialManager.getInstance(CRED_COUNT, `${UPSTREAM_URL}/oauth2/token`);
  const tokenStats = TokenStats.getInstance();

  return Response.json({
    tokens: tokenStats.getSummary(),
    credentials: credManager.getStats(),
  });
}
