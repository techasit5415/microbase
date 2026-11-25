import { apiRequest } from './client.ts';

export interface OAuthProvider {
	name: string;
	display_name: string;
	enabled: boolean;
	client_id: string;
	has_secret: boolean;
	issuer_url?: string;
}

export interface UpdateOAuthProviderInput {
	enabled: boolean;
	client_id: string;
	client_secret: string;
	issuer_url: string;
}

export async function listOAuthProviders(): Promise<OAuthProvider[]> {
	const res = await apiRequest<{ items: OAuthProvider[] }>('/api/oauth/providers');
	return res.items ?? [];
}

export async function updateOAuthProvider(
	name: string,
	input: UpdateOAuthProviderInput
): Promise<OAuthProvider> {
	return apiRequest<OAuthProvider>(`/api/oauth/providers/${encodeURIComponent(name)}`, {
		method: 'PUT',
		body: JSON.stringify(input)
	});
}
