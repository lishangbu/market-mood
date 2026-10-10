#!/usr/bin/env node
// Run real collection and read-only observation together in a dedicated directory.
// The UI fixture server is deliberately not usable as the collector.
import { spawn } from 'node:child_process';
import { mkdir, open } from 'node:fs/promises';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
const args = process.argv.slice(2);
if (args.includes('--help')) {
  console.log('node scripts/acceptance-session.mjs [directory] [binary] [--once]\nDefault: data/acceptance, bin/market-mood. Keep running through market close; Ctrl+C stops both processes. --once only checks startup/observation.');
  process.exit(0);
}
const once = args.includes('--once');
const positional = args.filter(arg => arg !== '--once');
const directory = resolve(positional[0] ?? 'data/acceptance');
const binary = resolve(positional[1] ?? 'bin/market-mood');
await mkdir(directory, {recursive: true});
const log = await open(resolve(directory, 'collector.log'), 'a');
const children = new Set();
let stopping = false;
const stop = () => {stopping = true;for (const child of children) child.kill('SIGTERM');};
process.on('SIGINT', stop);
process.on('SIGTERM', stop);
function start(command, arguments_, stdio) {
  const child = spawn(command, arguments_, {stdio});
  children.add(child);
  const done = new Promise((resolveDone, reject) => {
    child.once('error', reject);
    child.once('exit', (code, signal) => {
      children.delete(child);
      resolveDone({code, signal});
    });
  });
  // Attach rejection handling immediately, including during readiness checks.
  done.catch(() => {});
  return {child, done};
}
try {
  // Go HTTP service uses a dedicated loopback port and isolated SQLite file.
  const port = Number(process.env.MARKET_MOOD_ACCEPTANCE_PORT ?? 18080);
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw Error('Invalid MARKET_MOOD_ACCEPTANCE_PORT');
  const base = `http://127.0.0.1:${port}`;
  // Refuse any existing listener; do not observe an unrelated old server on bind failure.
  let occupied = false;
  try {await fetch(`${base}/api/health`, {signal: AbortSignal.timeout(1000)});occupied = true;} catch {}
  if (occupied) throw Error(`Port ${port} is already in use; choose MARKET_MOOD_ACCEPTANCE_PORT`);
  const collector = start(binary, ['--bind',`127.0.0.1:${port}`,'--db',resolve(directory,'market-mood.db')], ['ignore',log.fd,log.fd]);
  let ready = false;
  for (let attempt = 0; attempt < 60 && !stopping; attempt++) {
    if (collector.child.exitCode !== null || collector.child.signalCode !== null) throw Error('Collector exited; inspect collector.log');
    try {
      const response = await fetch(`${base}/api/health`, {signal:AbortSignal.timeout(1000)});
      if (response.ok) {ready = true;break;}
    } catch {}
    await new Promise(resolveWait => setTimeout(resolveWait, 500));
  }
  if (!ready) throw Error('Collector did not become ready; inspect collector.log');
  const observer = start(process.execPath, [fileURLToPath(new URL('./observe.mjs', import.meta.url)),base,resolve(directory,'observations'),...(once?['--once']:[])], ['ignore','inherit','inherit']);
  console.log(`Real collection: ${base}; evidence: ${directory}`);
  const result = await Promise.race([collector.done.then(result => ({...result,role:'collector'})),observer.done.then(result => ({...result,role:'observer'}))]);
  if (!stopping && !(once && result.role === 'observer' && result.code === 0))
    throw Error(`${result.role} exited unexpectedly (${result.code ?? result.signal})`);
} catch (error) {
  console.error(String(error));
  process.exitCode = 1;
} finally {
  stop();
  await Promise.all([...children].map(child => new Promise(resolveDone => {
    const timeout = setTimeout(() => child.kill('SIGKILL'), 5000);
    child.once('exit', () => {clearTimeout(timeout);resolveDone();});
  })));
  await log.close();
}
