import { getPackages } from '$lib/api/client';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	return { packages: await getPackages(fetch), origin: url.origin };
};
