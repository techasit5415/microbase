import { apiRequest } from './client';
import type { ListResult, RecordRow } from './types';

export interface ListRecordsParams {
	page?: number;
	perPage?: number;
	search?: string;
	sort?: string;
}

export async function listRecords(
	colName: string,
	params?: ListRecordsParams
): Promise<ListResult> {
	const qs = new URLSearchParams();
	if (params?.page) qs.set('page', String(params.page));
	if (params?.perPage) qs.set('per_page', String(params.perPage));
	if (params?.search) qs.set('search', params.search);
	if (params?.sort) qs.set('sort', params.sort);

	const query = qs.toString() ? `?${qs.toString()}` : '';
	return apiRequest<ListResult>(`/api/collections/${encodeURIComponent(colName)}/records${query}`);
}

export async function createRecord(
	colName: string,
	data: Record<string, unknown>
): Promise<RecordRow> {
	return apiRequest<RecordRow>(`/api/collections/${encodeURIComponent(colName)}/records`, {
		method: 'POST',
		body: JSON.stringify(data)
	});
}

export async function updateRecord(
	colName: string,
	id: number,
	data: Record<string, unknown>
): Promise<RecordRow> {
	return apiRequest<RecordRow>(`/api/collections/${encodeURIComponent(colName)}/records/${id}`, {
		method: 'PATCH',
		body: JSON.stringify(data)
	});
}

export async function deleteRecord(colName: string, id: number): Promise<void> {
	await apiRequest(`/api/collections/${encodeURIComponent(colName)}/records/${id}`, {
		method: 'DELETE'
	});
}