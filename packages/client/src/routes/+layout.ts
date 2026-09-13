import { getSession } from '$lib/api/client';
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = async ({ fetch }) => {
	// the session cookie is forwarded on same-origin SSR fetches, so the
	// header renders with the user already resolved
	return { session: await getSession(fetch) };
};
