import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const three = resolve(process.env.THREE_DIR ?? join(here, 'node_modules/three'));
const entry = (name, input) => ({
  input: join(here, '../src', input),
  resolve: { alias: { 'three/addons': join(three, 'examples/jsm'), three: join(three, 'build/three.module.js') } },
  output: { file: join(here, `../dist/${name}.js`), format: 'iife', minify: true, codeSplitting: false },
});

export default [
  entry('world', 'main.js'),
  ...['time', 'situation', 'scheduler', 'authority'].map((n) => entry(`lab-${n}`, `labs/${n}.js`)),
].filter((c) => process.env.ONLY ? c.output.file.endsWith(`${process.env.ONLY}.js`) : true);
