export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

export async function GET() {
  return Response.json({
    object: 'list',
    data: [
      { id: 'gemini-2.0-flash', object: 'model', owned_by: 'google' },
      { id: 'gemini-1.5-pro', object: 'model', owned_by: 'google' },
      { id: 'gemini-2.0-flash-thinking', object: 'model', owned_by: 'google' },
    ],
  });
}
