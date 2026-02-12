import { handleNonStreaming, handleStreaming } from '@/lib/proxy';
import type { OpenAIRequest } from '@/lib/converter';

export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

export async function POST(request: Request) {
  const body: OpenAIRequest = await request.json();

  if (body.stream) {
    return handleStreaming(body);
  } else {
    return handleNonStreaming(body);
  }
}
