import { error } from '@sveltejs/kit';
import { ApiError, getPackageReadme, getPackageSidebar } from '$lib/api/client';
import type { PageLoad } from './$types';

// splitId parses the catch-all route param into a package name and an
// optional version: "/packages/@scope/name" and "/packages/@scope/name/v/1.0.0".
// The last "/v/" wins so a package literally named "v" keeps working.
function splitId(id: string): { name: string; version?: string } {
	const i = id.lastIndexOf('/v/');
	if (i === -1) return { name: id };

	const version = id.slice(i + 3);
	if (version === '') {
		return { name: id };
	}
	return { name: id.slice(0, i), version };
}

export const load: PageLoad = async ({ fetch, params }) => {
	const { name, version } = splitId(params.id);

	try {
		const [readme, sidebar] = await Promise.all([
			getPackageReadme(fetch, name, version),
			getPackageSidebar(fetch, name, version)
		]);

		return { id: name, version, readme, sidebar };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			error(404);
		}
		throw err;
	}
};
