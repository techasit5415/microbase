import { apiRequest } from './client.ts';
import type { ListResult } from './types.ts';

export async function listSystemTable(tableName: string): Promise<ListResult> {
	return apiRequest<ListResult>(`/api/system/${encodeURIComponent(tableName)}`);
}

export async function deleteSystemRecord(table: string, id: number | string): Promise<void> {
	await apiRequest(`/api/system/${encodeURIComponent(table)}/${encodeURIComponent(String(id))}`, {
		method: 'DELETE'
	});
}

export async function createSystemRecord(table: string, payload: Record<string, unknown>): Promise<void> {
	await apiRequest(`/api/system/${encodeURIComponent(table)}`, {
		method: 'POST',
		body: JSON.stringify(payload)
	});
}

export async function updateSystemRecord(
	table: string,
	id: number | string,
	payload: Record<string, unknown>
): Promise<void> {
	await apiRequest(`/api/system/${encodeURIComponent(table)}/${encodeURIComponent(String(id))}`, {
		method: 'PATCH',
		body: JSON.stringify(payload)
	});
}
