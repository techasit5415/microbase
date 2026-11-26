import { apiRequest } from './client.ts';

export interface OAuthProvider {
	name: string;
	display_name: string;
	enabled: boolean;
	client_id: string;
	has_secret: boolean;
	issuer_url?: string;
	auth_url?: string;
	token_url?: string;
	userinfo_url?: string;
	profile_api_url?: string;
	has_api_key?: boolean;
	scopes?: string;
	field_mapping?: Record<string, string>;
	allowed_claims?: Record<string, string[]>;
}

export async function listOAuthProviders(): Promise<OAuthProvider[]> {
	const res = await apiRequest<{ items: OAuthProvider[] }>('/api/oauth/providers');
	return res.items ?? [];
}

export async function updateOAuthProvider(
	name: string,
	payload: {
		display_name?: string;
		enabled: boolean;
		client_id: string;
		client_secret?: string;
		issuer_url?: string;
		auth_url?: string;
		token_url?: string;
		userinfo_url?: string;
		profile_api_url?: string;
		profile_api_key?: string;
		scopes?: string;
		field_mapping?: Record<string, string>;
		allowed_claims?: Record<string, string[]>;
	}
): Promise<OAuthProvider> {
	return apiRequest<OAuthProvider>(`/api/oauth/providers/${encodeURIComponent(name)}`, {
		method: 'PUT',
		body: JSON.stringify(payload)
	});
}
