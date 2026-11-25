import { apiRequest } from './client';
import type { Collection, FieldDef } from './types';

export async function listCollections(): Promise<Collection[]> {
	const res = await apiRequest<{ items: Collection[] }>('/api/collections');
	return res.items ?? [];
}

export async function createCollection(name: string, fields: FieldDef[]): Promise<Collection> {
	return apiRequest<Collection>('/api/collections', {
		method: 'POST',
		body: JSON.stringify({ name, fields })
	});
}

export async function deleteCollection(name: string): Promise<void> {
	await apiRequest(`/api/collections/${encodeURIComponent(name)}`, { method: 'DELETE' });
}

export function getSchemaExportURL(): string {
	return '/api/schema/export';
}