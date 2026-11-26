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
