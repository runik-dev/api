#!usr/bin/node
const args = process.argv.slice(2);
const jsonIndex = args.indexOf("--json");
let parsed;

if (jsonIndex !== -1 && jsonIndex + 1 < args.length) {
	const jsonString = args[jsonIndex + 1];
	try {
		parsed = JSON.parse(jsonString);
	} catch (error) {
		throw "failed to parse json";
	}
} else {
	throw "json not provided";
}

const result = [];
for (const item of parsed.tree) {
	const path = item.path.split("/");
	let current = result;
	for (let i = 0; i < path.length; i++) {
		const name = path[i];
		if (i === path.length - 1) {
			if (item.type === "blob") current.push({ name, fullPath: item.path });
			else if (item.type === "tree") current.push({ name, files: [] });
		} else {
			let temp = current.find((x) => x.name === name);
			if (!temp) {
				temp = { name, files: [] };
				current.push(temp);
			}
			current = temp.files;
		}
	}
}

console.log(JSON.stringify(result));
