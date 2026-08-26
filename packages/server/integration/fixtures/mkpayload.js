// Builds an npm publish PUT body for raw-protocol test cases where driving
// the npm CLI is not enough (validation failures, size limits, ...).
//
// usage: node mkpayload.js --name @it/x [--tgz file.tgz] [--key 1.0.0]
//          [--version 1.0.0] [--shasum <hex>|none] [--no-gm]
//          [--data-b64 <base64>] [--out file]
//
// defaults: key=version=1.0.0, shasum computed over the tarball, gm block
// present, attachment data read from the tgz (or empty when no tgz given).

const fs = require("fs");

const args = process.argv.slice(2);
function opt(name) {
	const i = args.indexOf("--" + name);
	return i === -1 ? undefined : args[i + 1];
}
const flag = (name) => args.includes("--" + name);

const name = opt("name");
if (!name) {
	console.error("mkpayload: --name is required");
	process.exit(2);
}
const tgz = opt("tgz");
const version = opt("version") ?? "1.0.0";
const key = opt("key") ?? version;

let data = "";
if (tgz) data = fs.readFileSync(tgz).toString("base64");
if (opt("data-b64") !== undefined) data = opt("data-b64");
const length = Buffer.byteLength(data, "base64");

let shasum;
const shaOpt = opt("shasum");
if (shaOpt === "none") {
	shasum = undefined;
} else if (shaOpt !== undefined) {
	shasum = shaOpt;
} else if (tgz) {
	shasum = require("crypto").createHash("sha1").update(fs.readFileSync(tgz)).digest("hex");
}

const ver = { name, version };
if (!flag("no-gm")) {
	ver.gm = { destination: "packages/" + name.split("/").pop(), displayName: "IT " + name };
}
if (shasum !== undefined) ver.dist = { shasum };

const body = {
	_id: name,
	name,
	"dist-tags": { latest: key },
	versions: { [key]: ver },
	_attachments: { [key + ".tgz"]: { data, length, content_type: "application/octet-stream" } },
};

const json = JSON.stringify(body);
if (opt("out")) fs.writeFileSync(opt("out"), json);
else process.stdout.write(json);
