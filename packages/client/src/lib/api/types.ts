export type PackageIcon = {
	mime: string;
	data: string;
};

export type GmMetadata = {
	destination: string;
	displayName: string;
	shortDescription: string;
	visible: boolean;
	ide_versions: Record<string, string>;
	supportedArchitecture: string;
	tool: Record<string, string>;
	removeDependenciesOnUninstall: string; /// "always" union?
	notifyOfUpdates: boolean;
	requiresRestart: boolean;
	icon: PackageIcon;
};

export type Package = {
	name: string;
	version: string;
	description: string;
	keywords?: string[];
	author: {
		name: string;
		avatar: string;
	};
	license: string;
	files: string[];
	gm: GmMetadata;
	_id: string;
	readme: string;
	dist: {
		integrity: string;
		shasum: string;
		tarball: string;
		fileCount: number;
		unpackedSize: number;
	};
	contributors: string[];
	time: string;
};

export type PackageAuthor = {
	name: string;
	email: string;
	url: string;
	_avatar: string;
};

export type PackageVersion = {
	name: string;
	version: string;
	description: string;
	main?: string;
	scripts?: Record<string, string>;
	keywords?: string[];
	author: PackageAuthor;
	license: string;
	files: string[];
	dependencies?: Record<string, string>;
	gm: GmMetadata;
	readmeFilename: string;
	gitHead?: string;
	_id: string;
	_nodeVersion?: string;
	_npmVersion?: string;
	dist: {
		integrity: string;
		shasum: string;
		tarball: string;
	};
	contributors: string[];
};

export type PackageSidebar = {
	versions: Record<string, PackageVersion>;
	time: { modified: string; created: string } & Record<string, string>;
	'dist-tags': Record<string, string>;
	_id: string;
	latest: PackageVersion;
};
