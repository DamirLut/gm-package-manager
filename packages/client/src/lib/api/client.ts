import type { AccountEvent, Package, PackageSidebar, Session } from './types';

export type Fetch = typeof fetch;

export class ApiError extends Error {
	constructor(
		public readonly status: number,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

// The session endpoint answers 200 with {"user":null} for anonymous
// visitors, and network hiccups must not break the site header — so this
// call never throws.
export async function getSession(fetch: Fetch): Promise<Session> {
	try {
		const response = await fetch('/-/auth/session');
		if (!response.ok) return { user: null };
		return response.json();
	} catch {
		return { user: null };
	}
}

export async function getAccountLogins(fetch: Fetch, limit = 20): Promise<AccountEvent[]> {
	return accountEvents(fetch, `/-/account/logins?limit=${limit}`, 'logins');
}

export async function getAccountActivity(fetch: Fetch, limit = 20): Promise<AccountEvent[]> {
	return accountEvents(fetch, `/-/account/activity?limit=${limit}`, 'activity');
}

export async function unlinkIdentity(fetch: Fetch, provider: string): Promise<void> {
	const response = await fetch(`/-/account/identities/${encodeURIComponent(provider)}`, {
		method: 'DELETE'
	});
	if (!response.ok) throw await apiError(response, `Failed to unlink ${provider}`);
}

export async function logout(fetch: Fetch): Promise<void> {
	const response = await fetch('/-/auth/logout', { method: 'POST' });
	if (!response.ok) throw await apiError(response, 'Failed to sign out');
}

export async function deleteAccount(fetch: Fetch, username: string): Promise<void> {
	const response = await fetch('/-/account', {
		method: 'DELETE',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ username })
	});
	if (!response.ok) throw await apiError(response, 'Failed to delete account');
}

async function accountEvents(fetch: Fetch, url: string, what: string): Promise<AccountEvent[]> {
	const response = await fetch(url);
	if (!response.ok)
		throw new ApiError(
			response.status,
			`Failed to load ${what}: ${response.status} ${response.statusText}`
		);
	return response.json();
}

async function apiError(response: Response, prefix: string): Promise<ApiError> {
	let detail = response.statusText;
	try {
		const body = await response.json();
		if (body?.error) detail = body.error;
	} catch {
		// no JSON body: keep the status text
	}
	return new ApiError(response.status, `${prefix}: ${detail}`);
}

export async function getPackages(fetch: Fetch): Promise<Package[]> {
	const response = await fetch('/-/verdaccio/data/packages');

	if (!response.ok) {
		throw new ApiError(
			response.status,
			`Failed to load packages: ${response.status} ${response.statusText}`
		);
	}

	return response.json();
}

export async function searchPackages(fetch: Fetch, query: string): Promise<Package[]> {
	const response = await fetch(`/-/verdaccio/data/search?q=${encodeURIComponent(query)}`);

	if (!response.ok) {
		throw new ApiError(
			response.status,
			`Failed to search packages: ${response.status} ${response.statusText}`
		);
	}

	return response.json();
}

export async function getPackageReadme(
	fetch: Fetch,
	id: string,
	version?: string
): Promise<string> {
	const response = await fetch(
		`/-/verdaccio/data/package/readme/${encodeURIComponent(id)}${versionQuery(version)}`
	);

	if (!response.ok) {
		throw new ApiError(
			response.status,
			`Failed to load package readme: ${response.status} ${response.statusText}`
		);
	}

	return response.text();
}

export async function getPackageSidebar(
	fetch: Fetch,
	id: string,
	version?: string
): Promise<PackageSidebar> {
	const response = await fetch(
		`/-/verdaccio/data/sidebar/${encodeURIComponent(id)}${versionQuery(version)}`
	);

	if (!response.ok) {
		throw new ApiError(
			response.status,
			`Failed to load package sidebar: ${response.status} ${response.statusText}`
		);
	}

	return response.json();
}

// versionQuery mirrors the registry's ?v=<version> selector used by the
// website endpoints (see router/web.go).
function versionQuery(version?: string): string {
	if (!version) return '';
	return `?v=${encodeURIComponent(version)}`;
}
