import { error } from '@sveltejs/kit';
import { ApiError, getPackageReadme, getPackageSidebar } from '$lib/api/client';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	const id = params.id;

	try {
		const [readme, sidebar] = await Promise.all([
			getPackageReadme(fetch, id),
			getPackageSidebar(fetch, id)
		]);

		return { id, readme, sidebar };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			error(404);
		}
		throw err;
	}
};
