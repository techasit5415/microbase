import { apiRequest } from '#lib/api/client.ts';

export interface AppSettingsData {
	app_name: string;
	app_url: string;
	accent_color: string;
	hide_controls: boolean;
	batch: {
		enabled: boolean;
		max_requests: number;
		timeout: number;
	};
	trusted_proxy: {
		enabled: boolean;
		headers: string[];
	};
	superuser_ips: {
		enabled: boolean;
		allowed_ips: string[];
	};
}

const defaults: AppSettingsData = {
	app_name: 'Microbase',
	app_url: 'http://localhost:3000',
	accent_color: '#25272d',
	hide_controls: false,
	batch: { enabled: true, max_requests: 50, timeout: 10 },
	trusted_proxy: { enabled: false, headers: ['X-Forwarded-For', 'X-Real-IP'] },
	superuser_ips: { enabled: false, allowed_ips: ['127.0.0.1', '::1'] }
};

class AppSettingsStore {
	data = $state<AppSettingsData>(structuredClone(defaults));
	loaded = $state(false);

	applyBranding(d: AppSettingsData) {
		if (typeof document === 'undefined') return;
		if (d.app_name) {
			document.title = `${d.app_name} Admin`;
		}
		if (d.accent_color && /^#[0-9a-fA-F]{6}$/.test(d.accent_color)) {
			document.documentElement.style.setProperty('--primary', d.accent_color);
			document.documentElement.style.setProperty('--ring', d.accent_color);
		}
	}

	async load() {
		try {
			const res = await apiRequest<AppSettingsData>('/api/settings/app');
			if (res && res.app_name) {
				this.data = {
					...structuredClone(defaults),
					...res,
					batch: { ...defaults.batch, ...(res.batch ?? {}) },
					trusted_proxy: { ...defaults.trusted_proxy, ...(res.trusted_proxy ?? {}) },
					superuser_ips: { ...defaults.superuser_ips, ...(res.superuser_ips ?? {}) }
				};
				this.applyBranding(this.data);
			}
		} catch {
			// keep defaults
		} finally {
			this.loaded = true;
		}
	}

	async save(next: AppSettingsData): Promise<AppSettingsData> {
		const saved = await apiRequest<AppSettingsData>('/api/settings/app', {
			method: 'PUT',
			body: JSON.stringify(next)
		});
		this.data = saved;
		this.applyBranding(saved);
		return saved;
	}
}

export const appSettings = new AppSettingsStore();
