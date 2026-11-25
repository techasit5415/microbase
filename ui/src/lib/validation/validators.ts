import type { FieldDef, IndexDef } from '#lib/api/types.ts';

const IDENT_RE = /^[a-z][a-z0-9_]{0,62}$/;
const RESERVED_COLLECTIONS = new Set(['users', 'superusers', 'system', 'logs', 'settings', 'health', 'files', 'auth', 'oauth', 'webhooks', 'schema']);
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
			return `Collection name "${cleanName}" must start with a lowercase letter (a-z) and contain only a-z, 0-9, or _.`;
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
			return `Field "${fn}" must start with a lowercase letter and contain only a-z, 0-9, or _.`;
		}
		if (RESERVED_COLUMNS.has(fn)) {
			return `Field "${fn}" is reserved (id, created_at, updated_at are managed automatically).`;
		}
		if (seenFields.has(fn)) {
			return `Duplicate field name "${fn}". Each column must have a unique name.`;
		}
		seenFields.add(fn);

		if (f.type === 'vector') {
			const dim = Number(f.dimension);
			if (!Number.isInteger(dim) || dim < 1 || dim > 4096) {
				return `Vector field "${fn}" must have a dimension between 1 and 4096.`;
			}
		}
		if (f.type === 'file' && f.max_size !== undefined && (Number.isNaN(Number(f.max_size)) || Number(f.max_size) < 0)) {
			return `File field "${fn}" max size must be a positive number of bytes.`;
		}
	}

	const validIndexCols = new Set<string>(['id', 'created_at', 'updated_at', ...seenFields]);
	const seenIndexes = new Set<string>();
	for (const idx of indexes) {
		const iname = idx.name.trim();
		if (!iname || !IDENT_RE.test(iname)) {
			return `Index name "${iname}" is invalid (must start with a-z and use a-z, 0-9, _).`;
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
			if (field.required) {
				return { error: `Field "${field.name}" is required.` };
			}
			continue;
		}

		if (field.type === 'email' && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(raw)) {
			return { error: `Field "${field.name}" must be a valid email address.` };
		}
		if (field.type === 'url' && !/^https?:\/\/.+/.test(raw)) {
			return { error: `Field "${field.name}" must be a valid URL starting with http:// or https://.` };
		}
		if (field.type === 'integer' || field.type === 'relation') {
			const n = Number(raw);
			if (!Number.isInteger(n)) {
				return { error: `Field "${field.name}" must be a valid integer.` };
			}
			payload[field.name] = n;
		} else if (field.type === 'float' || field.type === 'number') {
			const n = Number(raw);
			if (Number.isNaN(n)) {
				return { error: `Field "${field.name}" must be a valid number.` };
			}
			payload[field.name] = n;
		} else if (field.type === 'json' || field.type === 'geopoint') {
			try {
				payload[field.name] = JSON.parse(raw);
			} catch {
				return { error: `Field "${field.name}" contains invalid JSON.` };
			}
		} else if (field.type === 'vector') {
			try {
				const parsed = JSON.parse(raw);
				if (!Array.isArray(parsed) || !parsed.every((v) => typeof v === 'number')) {
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
