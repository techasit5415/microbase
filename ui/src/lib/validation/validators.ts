import type { FieldDef, IndexDef } from '#lib/api/types.ts';

const IDENT_RE = /^[a-zA-Z_][a-zA-Z0-9_]{0,62}$/;
const RESERVED_COLLECTIONS = new Set([
	'users',
	'superusers',
	'system',
	'logs',
	'settings',
	'health',
	'files',
	'auth',
	'oauth',
	'webhooks',
	'schema'
]);
const RESERVED_COLUMNS = new Set(['id', 'created_at', 'updated_at']);

/** Validates a collection schema before POST / PATCH and returns an error string or null if valid. */
export function validateCollectionForm(
	name: string,
	fields: FieldDef[],
	indexes: IndexDef[],
	isEdit: boolean
): string | null {
	const cleanName = name.trim();
	if (!isEdit) {
		if (!cleanName) return 'Collection name is required.';
		if (!IDENT_RE.test(cleanName)) {
			return `Collection name "${cleanName}" must start with a letter or _ and contain only a-z, A-Z, 0-9, or _.`;
		}
		if (RESERVED_COLLECTIONS.has(cleanName)) {
			return `Collection name "${cleanName}" is reserved by Microbase.`;
		}
	}

	const seenFields = new Set<string>();
	for (let i = 0; i < fields.length; i++) {
		const f = fields[i];
		const fn = f.name.trim();
		if (!fn) {
			return `Field #${i + 1} (${f.type}) must have a column name.`;
		}
		if (!IDENT_RE.test(fn)) {
			return `Field "${fn}" must start with a letter or _ and contain only a-z, A-Z, 0-9, or _.`;
		}
		if (RESERVED_COLUMNS.has(fn)) {
			return `Field "${fn}" is reserved (id, created_at, updated_at are managed automatically).`;
		}
		if (seenFields.has(fn)) {
			return `Duplicate field name "${fn}". Each column must have a unique name.`;
		}
		seenFields.add(fn);

		if (f.type === 'relation' && !f.collection_name?.trim()) {
			return `Relation field "${fn}" must select a target collection.`;
		}
		if (f.type === 'select' && (!f.values || f.values.length === 0)) {
			return `Select field "${fn}" must have at least one option choice.`;
		}
		if (f.type === 'vector') {
			const dim = Number(f.dimension);
			if (!Number.isInteger(dim) || dim < 1 || dim > 4096) {
				return `Vector field "${fn}" must have a dimension between 1 and 4096.`;
			}
		}
		if (f.min !== undefined && f.max !== undefined && Number(f.min) > Number(f.max)) {
			return `Field "${fn}" min (${f.min}) cannot be greater than max (${f.max}).`;
		}
		if (f.type === 'text' && f.pattern) {
			try {
				new RegExp(f.pattern);
			} catch {
				return `Field "${fn}" has an invalid regex pattern: ${f.pattern}`;
			}
		}
		if (
			(f.type === 'file' || f.type === 'editor' || f.type === 'json') &&
			f.max_size !== undefined &&
			(Number.isNaN(Number(f.max_size)) || Number(f.max_size) < 0)
		) {
			return `Field "${fn}" max size must be a positive number of bytes.`;
		}
	}

	const validIndexCols = new Set<string>(['id', 'created_at', 'updated_at', 'email', 'role', 'verified', 'email_visibility', 'token_key', ...seenFields]);
	const seenIndexes = new Set<string>();
	for (const idx of indexes) {
		const iname = idx.name.trim();
		if (!iname || !IDENT_RE.test(iname)) {
			return `Index name "${iname}" is invalid (must start with a-z, A-Z, or _ and use a-z, A-Z, 0-9, _).`;
		}
		if (seenIndexes.has(iname)) {
			return `Duplicate index name "${iname}".`;
		}
		seenIndexes.add(iname);
		if (!idx.columns || idx.columns.length === 0) {
			return `Index "${iname}" must select at least one column.`;
		}
		for (const col of idx.columns) {
			if (!validIndexCols.has(col)) {
				return `Index "${iname}" references unknown column "${col}".`;
			}
		}
	}

	return null;
}

/** Validates and builds a dynamic record payload, returning either { payload } or { error }. */
export function buildAndValidateRecordPayload(
	fields: FieldDef[],
	values: Record<string, string>,
	boolValues: Record<string, boolean>
): { payload?: Record<string, unknown>; error?: string } {
	const payload: Record<string, unknown> = {};

	for (const field of fields) {
		if (field.type === 'boolean') {
			payload[field.name] = Boolean(boolValues[field.name]);
			continue;
		}
		const raw = (values[field.name] ?? '').trim();
		if (raw === '') {
			if (field.required && field.type !== 'autodate') {
				return { error: `Field "${field.name}" is required.` };
			}
			continue;
		}

		if (field.type === 'text') {
			if (field.min !== undefined && raw.length < field.min) {
				return { error: `Field "${field.name}" must be at least ${field.min} characters.` };
			}
			if (field.max !== undefined && raw.length > field.max) {
				return { error: `Field "${field.name}" must be at most ${field.max} characters.` };
			}
			if (field.pattern && !new RegExp(field.pattern).test(raw)) {
				return { error: `Field "${field.name}" does not match pattern /${field.pattern}/.` };
			}
			payload[field.name] = raw;
		} else if (field.type === 'email') {
			if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(raw)) {
				return { error: `Field "${field.name}" must be a valid email address.` };
			}
			const domain = raw.split('@')[1]?.toLowerCase() ?? '';
			if (field.only_domains?.length && !field.only_domains.includes(domain)) {
				return { error: `Email domain "${domain}" is not allowed for "${field.name}".` };
			}
			if (field.except_domains?.length && field.except_domains.includes(domain)) {
				return { error: `Email domain "${domain}" is blocked for "${field.name}".` };
			}
			payload[field.name] = raw;
		} else if (field.type === 'url') {
			if (!/^https?:\/\/.+/.test(raw)) {
				return { error: `Field "${field.name}" must be a valid URL starting with http:// or https://.` };
			}
			payload[field.name] = raw;
		} else if (field.type === 'select') {
   if ((field.max_select ?? 1) > 1) {
    try {
     const choices: unknown = JSON.parse(raw);
     if (!Array.isArray(choices) || choices.length > field.max_select! || !choices.every(v => typeof v === 'string' && (!field.values?.length || field.values.includes(v)))) {
      return { error: `Field "${field.name}" must contain an array of valid choices within its selection limit.` };
     }
     payload[field.name] = choices;
     continue;
    } catch { return { error: `Field "${field.name}" contains invalid JSON.` }; }
   }

			if (field.values?.length && (field.max_select ?? 1) <= 1 && !field.values.includes(raw)) {
				return { error: `Field "${field.name}" must be one of: ${field.values.join(', ')}.` };
			}
			payload[field.name] = raw;
		} else if (field.type === 'relation' && (field.max_select ?? 1) <= 1) {
			// Preserve relation IDs as strings so zero-padded numeric text IDs (e.g. "000000000000006")
			// are never coerced into numbers; PostgreSQL/pgx also accepts numeric strings for BIGINT FKs.
			payload[field.name] = raw;
		} else if (field.type === 'integer') {
			const n = Number(raw);
			if (!Number.isInteger(n)) {
				return { error: `Field "${field.name}" must be a valid integer.` };
			}
			if (field.min !== undefined && n < field.min) {
				return { error: `Field "${field.name}" must be >= ${field.min}.` };
			}
			if (field.max !== undefined && n > field.max) {
				return { error: `Field "${field.name}" must be <= ${field.max}.` };
			}
			payload[field.name] = n;
		} else if (field.type === 'float' || field.type === 'number') {
			const n = Number(raw);
			if (!Number.isFinite(n)) {
				return { error: `Field "${field.name}" must be a valid number.` };
			}
			if (field.only_int && !Number.isInteger(n)) {
				return { error: `Field "${field.name}" only allows integer values.` };
			}
			if (field.min !== undefined && n < field.min) {
				return { error: `Field "${field.name}" must be >= ${field.min}.` };
			}
			if (field.max !== undefined && n > field.max) {
				return { error: `Field "${field.name}" must be <= ${field.max}.` };
			}
			payload[field.name] = n;
		} else if (field.type === 'json' || field.type === 'geopoint' || (field.type === 'relation' && (field.max_select ?? 1) > 1)) {
			try {
				payload[field.name] = JSON.parse(raw);
			} catch {
				return { error: `Field "${field.name}" contains invalid JSON.` };
			}
		} else if (field.type === 'vector') {
			try {
				const parsed = JSON.parse(raw);
				if (!Array.isArray(parsed) || !parsed.every((v) => typeof v === 'number' && Number.isFinite(v))) {
					return { error: `Vector field "${field.name}" must be a JSON array of numbers, e.g. [0.1, 0.2].` };
				}
				if (field.dimension && parsed.length !== field.dimension) {
					return { error: `Vector field "${field.name}" expects ${field.dimension} dimensions, got ${parsed.length}.` };
				}
				payload[field.name] = parsed;
			} catch {
				return { error: `Vector field "${field.name}" contains invalid JSON array.` };
			}
		} else {
			payload[field.name] = raw;
		}
	}

	return { payload };
}
