import { getPackageReadme, getPackageSidebar } from '$lib/api/client';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	const id = params.id;

	const [readme, sidebar] = await Promise.all([
		getPackageReadme(fetch, id),
		getPackageSidebar(fetch, id)
	]);

	return { id, readme, sidebar };
};
