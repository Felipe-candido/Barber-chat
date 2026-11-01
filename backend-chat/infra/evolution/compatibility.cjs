const fs = require('node:fs');
// Upstream sanitizes route identifiers on all requests. Creation has no route
// identifier; it must pass its schema-validated InstanceDto, not empty params.
// All authentication, licensing and non-creation parameter protections remain.
const path = 'src/api/routes/instance.router.ts';
const before = 'execute: (instance) => instanceController.createInstance(instance),';
const after = 'execute: (_instance, data) => instanceController.createInstance(data),';
const source = fs.readFileSync(path, 'utf8');
if (source.split(before).length !== 2) {
  throw new Error('Pinned Evolution creation contract changed; review the compatibility patch.');
}
fs.writeFileSync(path, source.replace(before, after));
