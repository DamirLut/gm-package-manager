import type { Package, PackageSidebar } from './types';

export type Fetch = typeof fetch;

export async function getPackages(fetch: Fetch): Promise<Package[]> {
	const response = await fetch('/-/verdaccio/data/packages');

	if (!response.ok) {
		throw new Error(`Failed to load packages: ${response.status} ${response.statusText}`);
	}

	return response.json();
}

export async function getPackageReadme(fetch: Fetch, id: string): Promise<string> {
	const response = await fetch(`/-/verdaccio/data/package/readme/${encodeURIComponent(id)}`);

	if (!response.ok) {
		throw new Error(`Failed to load package readme: ${response.status} ${response.statusText}`);
	}

	return response.text();
}

export async function getPackageSidebar(fetch: Fetch, id: string): Promise<PackageSidebar> {
	const response = await fetch(`/-/verdaccio/data/sidebar/${encodeURIComponent(id)}`);

	if (!response.ok) {
		throw new Error(`Failed to load package sidebar: ${response.status} ${response.statusText}`);
	}

	return response.json();
}
