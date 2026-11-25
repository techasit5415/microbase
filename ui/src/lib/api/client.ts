export const ADMIN_TOKEN_KEY = 'microbase-admin-token';

/** Shared HTTP JSON client for Microbase API endpoints (auto-attaches Superuser JWT if present). */
export async function apiRequest<T>(url: string, init?: RequestInit): Promise<T> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json'
	};
	if (typeof window !== 'undefined') {
		const token = localStorage.getItem(ADMIN_TOKEN_KEY);
		if (token) {
			headers['Authorization'] = `Bearer ${token}`;
		}
	}

	const res = await fetch(url, {
		...init,
		headers: { ...headers, ...(init?.headers as Record<string, string> | undefined) }
	});
	const data = await res.json();
	if (!res.ok) {
		throw new Error(data?.message || `HTTP ${res.status}`);
	}
	return data as T;
}
