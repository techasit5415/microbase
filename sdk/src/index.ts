export interface ListOptions {
  page?: number;
  perPage?: number;
  sort?: string;
  search?: string;
  filter?: string;
  expand?: string;
  fields?: string;
}

export interface ListResult<T = Record<string, unknown>> {
  page: number;
  per_page: number;
  total_items: number;
  items: T[];
}

export interface AuthResult<T = Record<string, unknown>> {
  token: string;
  record?: T;
  user?: T;
}

export interface RealtimeEvent<T = Record<string, unknown>> {
  action: 'create' | 'update' | 'delete';
  collection: string;
  record: T;
}

export class AuthStore {
  private storageKey: string;
  token = '';
  record: Record<string, unknown> | null = null;

  constructor(storageKey = 'microbase_auth') {
    this.storageKey = storageKey;
    this.load();
  }

  get isValid(): boolean {
    return this.token.length > 0;
  }

  save(token: string, record: Record<string, unknown> | null = null) {
    this.token = token;
    this.record = record;
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(this.storageKey, JSON.stringify({ token, record }));
    }
  }

  clear() {
    this.token = '';
    this.record = null;
    if (typeof localStorage !== 'undefined') {
      localStorage.removeItem(this.storageKey);
    }
  }

  private load() {
    if (typeof localStorage === 'undefined') return;
    try {
      const raw = localStorage.getItem(this.storageKey);
      if (!raw) return;
      const parsed = JSON.parse(raw);
      this.token = parsed.token ?? '';
      this.record = parsed.record ?? null;
    } catch {
      // ignore corrupted storage
    }
  }
}

export class MicrobaseError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'MicrobaseError';
    this.status = status;
    this.code = code;
  }
}

export class CollectionClient<T = Record<string, unknown>> {
  constructor(
    private client: Microbase,
    private name: string
  ) {}

  private recordsPath(): string {
    return this.name === 'users'
      ? '/api/users'
      : `/api/collections/${encodeURIComponent(this.name)}/records`;
  }

  async getList(page = 1, perPage = 20, opts: ListOptions = {}): Promise<ListResult<T>> {
    const qs = new URLSearchParams({
      page: String(page),
      per_page: String(perPage)
    });
    if (opts.sort) qs.set('sort', opts.sort);
    if (opts.search) qs.set('search', opts.search);
    if (opts.filter) qs.set('filter', opts.filter);
    if (opts.expand) qs.set('expand', opts.expand);
    if (opts.fields) qs.set('fields', opts.fields);
    return this.client.send<ListResult<T>>(`${this.recordsPath()}?${qs.toString()}`);
  }

  async getFullList(opts: ListOptions = {}): Promise<T[]> {
    const items: T[] = [];
    let page = 1;
    while (true) {
      const res = await this.getList(page, 100, opts);
      items.push(...(res.items ?? []));
      if (items.length >= res.total_items || (res.items ?? []).length < 100) break;
      page++;
    }
    return items;
  }

  async getOne(id: string | number, opts: Pick<ListOptions, 'expand' | 'fields'> = {}): Promise<T> {
    const qs = new URLSearchParams();
    if (opts.expand) qs.set('expand', opts.expand);
    if (opts.fields) qs.set('fields', opts.fields);
    const suffix = qs.toString() ? `?${qs.toString()}` : '';
    return this.client.send<T>(`${this.recordsPath()}/${encodeURIComponent(String(id))}${suffix}`);
  }

  async create(body: Partial<T> & Record<string, unknown>): Promise<T> {
    const path = this.name === 'users' ? '/api/auth/register' : this.recordsPath();
    return this.client.send<T>(path, { method: 'POST', body: JSON.stringify(body) });
  }

  async update(id: string | number, body: Partial<T> & Record<string, unknown>): Promise<T> {
    return this.client.send<T>(`${this.recordsPath()}/${encodeURIComponent(String(id))}`, {
      method: 'PATCH',
      body: JSON.stringify(body)
    });
  }

  async delete(id: string | number): Promise<{ deleted: string | number }> {
    return this.client.send<{ deleted: string | number }>(
      `${this.recordsPath()}/${encodeURIComponent(String(id))}`,
      { method: 'DELETE' }
    );
  }

  async vectorSearch(field: string, vector: number[], limit = 10): Promise<T[]> {
    const res = await this.client.send<{ items: T[] }>(
      `/api/collections/${encodeURIComponent(this.name)}/search/vector`,
      { method: 'POST', body: JSON.stringify({ field, vector, limit }) }
    );
    return res.items ?? [];
  }

  async register(body: { email: string; password: string } & Record<string, unknown>): Promise<AuthResult<T>> {
    const path =
      this.name === 'users'
        ? '/api/auth/register'
        : `/api/collections/${encodeURIComponent(this.name)}/register`;
    const res = await this.client.send<AuthResult<T>>(path, {
      method: 'POST',
      body: JSON.stringify(body)
    });
    const rec = (res.record ?? res.user ?? null) as Record<string, unknown> | null;
    if (res.token) this.client.authStore.save(res.token, rec);
    return res;
  }

  async authWithPassword(identity: string, password: string): Promise<AuthResult<T>> {
    const path =
      this.name === '_superusers'
        ? '/api/superusers/login'
        : this.name === 'users'
          ? '/api/auth/login'
          : `/api/collections/${encodeURIComponent(this.name)}/auth-with-password`;
    const res = await this.client.send<AuthResult<T>>(path, {
      method: 'POST',
      body: JSON.stringify({ email: identity, identity, password })
    });
    const rec = (res.record ?? res.user ?? null) as Record<string, unknown> | null;
    if (res.token) this.client.authStore.save(res.token, rec);
    return res;
  }

  async authRefresh(): Promise<AuthResult<T>> {
    const path =
      this.name === 'users'
        ? '/api/auth/refresh'
        : `/api/collections/${encodeURIComponent(this.name)}/auth-refresh`;
    const res = await this.client.send<AuthResult<T>>(path, { method: 'POST' });
    if (res.token) this.client.authStore.save(res.token, this.client.authStore.record);
    return res;
  }

  subscribe(callback: (event: RealtimeEvent<T>) => void): () => void {
    const url = `${this.client.baseUrl}/api/realtime?collection=${encodeURIComponent(this.name)}`;
    const es = new EventSource(url);
    es.onmessage = (e) => {
      try {
        callback(JSON.parse(e.data) as RealtimeEvent<T>);
      } catch {
        // ignore keepalive frames
      }
    };
    return () => es.close();
  }
}

export class Microbase {
  readonly baseUrl: string;
  readonly authStore: AuthStore;

  constructor(baseUrl = 'http://localhost:3000', authStore = new AuthStore()) {
    this.baseUrl = baseUrl.replace(/\/+$/, '');
    this.authStore = authStore;
  }

  collection<T = Record<string, unknown>>(name: string): CollectionClient<T> {
    return new CollectionClient<T>(this, name);
  }

  getOAuth2Url(provider: 'google' | 'discord' | 'oidc', redirectTo?: string): string {
    const qs = redirectTo ? `?redirect_to=${encodeURIComponent(redirectTo)}` : '';
    return `${this.baseUrl}/api/auth/oauth/${encodeURIComponent(provider)}${qs}`;
  }

  async exchangeOAuth2Code<T = Record<string, unknown>>(code: string): Promise<AuthResult<T>> {
    const res = await this.send<AuthResult<T>>('/api/auth/oauth/exchange', {
      method: 'POST',
      body: JSON.stringify({ code })
    });
    const rec = (res.record ?? res.user ?? null) as Record<string, unknown> | null;
    if (res.token) this.authStore.save(res.token, rec);
    return res;
  }

  async uploadFile(file: File | Blob, filename?: string): Promise<{ filename: string; url: string; backend: string }> {
    const form = new FormData();
    form.append('file', file, filename);
    const headers: Record<string, string> = {};
    if (this.authStore.token) headers.Authorization = `Bearer ${this.authStore.token}`;
    const res = await fetch(`${this.baseUrl}/api/files`, { method: 'POST', headers, body: form });
    const json = await res.json();
    if (!res.ok) {
      throw new MicrobaseError(res.status, json?.error?.code ?? 'UPLOAD_ERROR', json?.error?.message ?? res.statusText);
    }
    return json.data ?? json;
  }

  async send<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(init.headers as Record<string, string> | undefined)
    };
    if (this.authStore.token && !headers.Authorization) {
      headers.Authorization = `Bearer ${this.authStore.token}`;
    }
    const res = await fetch(`${this.baseUrl}${path}`, { ...init, headers });
    const json = await res.json().catch(() => ({}));
    if (!res.ok) {
      throw new MicrobaseError(
        res.status,
        json?.error?.code ?? 'REQUEST_FAILED',
        json?.error?.message ?? res.statusText
      );
    }
    return (json.data !== undefined ? json.data : json) as T;
  }
}

export default Microbase;
