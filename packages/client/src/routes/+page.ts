import { getPackages } from '$lib/api/client';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	return { packages: await getPackages(fetch) };
};
