import { apiRequest } from './client.ts';
import type { ListResult } from './types.ts';

export async function listSystemTable(tableName: string): Promise<ListResult> {
	return apiRequest<ListResult>(`/api/system/${encodeURIComponent(tableName)}`);
}
