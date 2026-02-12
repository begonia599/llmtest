export function cleanSchemaForGemini(schema: Record<string, any> | undefined): Record<string, any> | undefined {
  if (!schema) return undefined;
  return cleanRecursive(schema, new Set());
}

function cleanRecursive(schema: Record<string, any>, visited: Set<any>): Record<string, any> {
  if (visited.has(schema)) return schema;
  visited.add(schema);
  const result: Record<string, any> = {};

  for (const [k, v] of Object.entries(schema)) {
    switch (k) {
      case 'type':
        result.type = convertType(v);
        break;
      case 'properties':
        if (typeof v === 'object' && v) {
          const cleaned: Record<string, any> = {};
          for (const [pk, pv] of Object.entries(v))
            cleaned[pk] = typeof pv === 'object' && pv ? cleanRecursive(pv as Record<string, any>, visited) : pv;
          result.properties = cleaned;
        }
        break;
      case 'items':
        if (typeof v === 'object' && v) result.items = cleanRecursive(v as Record<string, any>, visited);
        break;
      case 'allOf':
        if (Array.isArray(v)) Object.assign(result, mergeAllOf(v, visited));
        break;
      case 'anyOf':
        if (Array.isArray(v)) { const e = extractEnum(v); if (e) result.enum = e; }
        break;
      case 'default':
        result.description = result.description ? `${result.description} (Default: ${v})` : `(Default: ${v})`;
        break;
      case 'required': case 'description': case 'enum': case 'format': case 'nullable':
        result[k] = v;
        break;
      case '$defs': case 'definitions': case '$schema': case '$id': case 'const': case 'oneOf': case 'strict':
        break;
      default:
        result[k] = v;
    }
  }
  return result;
}

function convertType(v: any): string {
  if (typeof v === 'string') return typeMap(v);
  if (Array.isArray(v)) { const t = v.find((x: string) => x !== 'null'); return t ? typeMap(t) : 'STRING'; }
  return 'STRING';
}

function typeMap(t: string): string {
  const m: Record<string, string> = { string:'STRING', number:'NUMBER', integer:'INTEGER', boolean:'BOOLEAN', array:'ARRAY', object:'OBJECT' };
  return m[t.toLowerCase()] || 'STRING';
}

function mergeAllOf(arr: any[], visited: Set<any>): Record<string, any> {
  const merged: Record<string, any> = {}, props: Record<string, any> = {}, req: any[] = [];
  for (const item of arr) {
    if (typeof item !== 'object' || !item) continue;
    const c = cleanRecursive(item, visited);
    for (const [k, v] of Object.entries(c)) {
      if (k === 'properties' && typeof v === 'object') Object.assign(props, v);
      else if (k === 'required' && Array.isArray(v)) req.push(...v);
      else merged[k] = v;
    }
  }
  if (Object.keys(props).length) merged.properties = props;
  if (req.length) merged.required = req;
  return merged;
}

function extractEnum(arr: any[]): any[] | null {
  const enums: any[] = [];
  for (const item of arr) {
    if (typeof item === 'object' && item && 'const' in item) enums.push(item.const);
    else return null;
  }
  return enums;
}
