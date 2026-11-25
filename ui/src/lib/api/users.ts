import type { RecordRow } from '#lib/api/types.ts';
import { apiRequest } from './client.ts';
import type { ListResult } from './types.ts';

export async function listUsers(params?: {
	page?: number;
	perPage?: number;
	search?: string;
}): Promise<ListResult> {
	const qs = new URLSearchParams();
	if (params?.page) qs.set('page', String(params.page));
	if (params?.perPage) qs.set('per_page', String(params.perPage));
	if (params?.search) qs.set('search', params.search);
	const query = qs.toString() ? `?${qs.toString()}` : '';
	return apiRequest<ListResult>(`/api/users${query}`);
}

export async function createUser(email: string, password: string): Promise<void> {
	await apiRequest('/api/auth/register', {
		method: 'POST',
		body: JSON.stringify({ email, password })
	});
}

export async function deleteUser(id: number): Promise<void> {
	await apiRequest(`/api/users/${id}`, { method: 'DELETE' });
}

export function createSuperuser(email: string, password: string): Promise<RecordRow> {
	return apiRequest<RecordRow>('/api/superusers', {
		method: 'POST',
		body: JSON.stringify({ email, password })
	});
}

export function deleteSuperuser(id: number): Promise<{ deleted: number }> {
	return apiRequest<{ deleted: number }>(`/api/superusers/${id}`, { method: 'DELETE' });
}

export function updateUser(
	id: number,
	payload: Record<string, unknown>
): Promise<RecordRow> {
	return apiRequest<RecordRow>(`/api/users/${id}`, {
		method: 'PATCH',
		body: JSON.stringify(payload)
	});
}

export function updateSuperuser(
	id: number,
	payload: { email: string; password?: string }
): Promise<RecordRow> {
	return apiRequest<RecordRow>(`/api/superusers/${id}`, {
		method: 'PATCH',
		body: JSON.stringify(payload)
	});
}
