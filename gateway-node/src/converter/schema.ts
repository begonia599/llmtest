export function cleanSchemaForGemini(schema: Record<string, any> | undefined): Record<string, any> | undefined {
  if (!schema) return undefined;
  const visited = new Set<any>();
  return cleanRecursive(schema, visited);
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
        if (typeof v === 'object' && v !== null) {
          const cleaned: Record<string, any> = {};
          for (const [pk, pv] of Object.entries(v)) {
            cleaned[pk] = typeof pv === 'object' && pv !== null ? cleanRecursive(pv as Record<string, any>, visited) : pv;
          }
          result.properties = cleaned;
        }
        break;
      case 'items':
        if (typeof v === 'object' && v !== null) {
          result.items = cleanRecursive(v as Record<string, any>, visited);
        }
        break;
      case 'allOf':
        if (Array.isArray(v)) {
          const merged = mergeAllOf(v, visited);
          Object.assign(result, merged);
        }
        break;
      case 'anyOf':
        if (Array.isArray(v)) {
          const enums = extractEnumFromAnyOf(v);
          if (enums) result.enum = enums;
        }
        break;
      case 'default':
        result.description = result.description
          ? `${result.description} (Default: ${v})`
          : `(Default: ${v})`;
        break;
      case 'required':
      case 'description':
      case 'enum':
      case 'format':
      case 'nullable':
        result[k] = v;
        break;
      case '$defs':
      case 'definitions':
      case '$schema':
      case '$id':
      case 'const':
      case 'oneOf':
      case 'strict':
        break;
      default:
        result[k] = v;
    }
  }
  return result;
}

function convertType(v: any): string {
  if (typeof v === 'string') return typeToGemini(v);
  if (Array.isArray(v)) {
    const nonNull = v.find((t: string) => t !== 'null');
    return nonNull ? typeToGemini(nonNull) : 'STRING';
  }
  return 'STRING';
}

function typeToGemini(t: string): string {
  const map: Record<string, string> = {
    string: 'STRING', number: 'NUMBER', integer: 'INTEGER',
    boolean: 'BOOLEAN', array: 'ARRAY', object: 'OBJECT',
  };
  return map[t.toLowerCase()] || 'STRING';
}

function mergeAllOf(arr: any[], visited: Set<any>): Record<string, any> {
  const merged: Record<string, any> = {};
  const mergedProps: Record<string, any> = {};
  const mergedRequired: any[] = [];

  for (const item of arr) {
    if (typeof item === 'object' && item !== null) {
      const cleaned = cleanRecursive(item, visited);
      for (const [k, v] of Object.entries(cleaned)) {
        if (k === 'properties' && typeof v === 'object') {
          Object.assign(mergedProps, v);
        } else if (k === 'required' && Array.isArray(v)) {
          mergedRequired.push(...v);
        } else {
          merged[k] = v;
        }
      }
    }
  }
  if (Object.keys(mergedProps).length > 0) merged.properties = mergedProps;
  if (mergedRequired.length > 0) merged.required = mergedRequired;
  return merged;
}

function extractEnumFromAnyOf(arr: any[]): any[] | null {
  const enums: any[] = [];
  for (const item of arr) {
    if (typeof item === 'object' && item !== null && 'const' in item) {
      enums.push(item.const);
    } else {
      return null;
    }
  }
  return enums;
}
