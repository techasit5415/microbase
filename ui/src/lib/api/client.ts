export const ADMIN_TOKEN_KEY = 'microbase-admin-token';
export const AUTH_EXPIRED_EVENT = 'microbase:auth-expired';

/** Shared HTTP JSON client for Microbase API endpoints (auto-attaches Superuser JWT if present). */
export async function apiRequest<T>(url: string, init?: RequestInit): Promise<T> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json'
	};
	let hadToken = false;
	if (typeof window !== 'undefined') {
		const token = localStorage.getItem(ADMIN_TOKEN_KEY);
		if (token) {
			hadToken = true;
			headers['Authorization'] = `Bearer ${token}`;
		}
	}

	const res = await fetch(url, {
		...init,
		headers: { ...headers, ...(init?.headers as Record<string, string> | undefined) }
	});
	let data: Record<string, unknown> | null = null;
	try {
		data = await res.json();
	} catch {
		data = null;
	}
	if (!res.ok) {
		if (
			res.status === 401 &&
			hadToken &&
			typeof window !== 'undefined' &&
			!url.includes('/api/superusers/login') &&
			!url.includes('/api/superusers/setup')
		) {
			localStorage.removeItem(ADMIN_TOKEN_KEY);
			window.dispatchEvent(new CustomEvent(AUTH_EXPIRED_EVENT));
		}
		const msg = typeof data?.message === 'string' ? data.message : `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return data as T;
}
