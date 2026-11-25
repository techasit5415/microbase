/** Shared HTTP JSON client for Microbase API endpoints. */
export async function apiRequest<T>(url: string, init?: RequestInit): Promise<T> {
	const res = await fetch(url, {
		headers: { 'Content-Type': 'application/json', ...init?.headers },
		...init
	});
	const data = await res.json();
	if (!res.ok) {
		throw new Error(data?.message || `HTTP ${res.status}`);
	}
	return data as T;
}