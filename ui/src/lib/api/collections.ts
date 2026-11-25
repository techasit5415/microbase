import { apiRequest } from './client.ts';
import type { Collection, FieldDef, IndexDef } from './types.ts';

export async function listCollections(): Promise<Collection[]> {
	const res = await apiRequest<{ items: Collection[] }>('/api/collections');
	return res.items ?? [];
}

export async function createCollection(
	name: string,
	fields: FieldDef[],
	indexes: IndexDef[] = [],
	apiRules: Record<string, string> = {},
	type: 'base' | 'view' | 'auth' = 'base',
	viewQuery = ''
): Promise<Collection> {
	return apiRequest<Collection>('/api/collections', {
		method: 'POST',
		body: JSON.stringify({
			name,
			type,
			view_query: viewQuery,
			fields,
			indexes,
			api_rules: apiRules
		})
	});
}

export async function updateCollection(
	name: string,
	fields: FieldDef[],
	apiRules?: Record<string, string>,
	indexes?: IndexDef[],
	viewQuery?: string
): Promise<Collection> {
	return apiRequest<Collection>(`/api/collections/${encodeURIComponent(name)}`, {
		method: 'PATCH',
		body: JSON.stringify({
			fields,
			api_rules: apiRules,
			indexes,
			view_query: viewQuery
		})
	});
}

export async function deleteCollection(name: string): Promise<void> {
	await apiRequest(`/api/collections/${encodeURIComponent(name)}`, { method: 'DELETE' });
}

export function getSchemaExportURL(): string {
	return '/api/schema/export';
}
