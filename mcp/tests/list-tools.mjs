// list-tools.mjs — pide tools/list al server fijado y lo imprime (sin credenciales de escritura).
import { spawn } from 'node:child_process';
const ver = process.argv[2] ?? '0.6.0';
const p = spawn('npx', ['-y', `@google-cloud/storage-mcp@${ver}`], { stdio: ['pipe', 'pipe', 'inherit'] });
let buf = '';
p.stdout.on('data', d => {
  buf += d;
  for (const l of buf.split('\n')) {
    try {
      const m = JSON.parse(l);
      if (m.id === 2) {
        for (const t of m.result.tools) console.log(t.name.padEnd(28), (t.description ?? '').split('\n')[0].slice(0, 90));
        p.kill(); process.exit(0);
      }
    } catch {}
  }
});
const send = o => p.stdin.write(JSON.stringify(o) + '\n');
send({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'list-tools', version: '0' } } });
setTimeout(() => { send({ jsonrpc: '2.0', method: 'notifications/initialized' }); send({ jsonrpc: '2.0', id: 2, method: 'tools/list' }); }, 4000);
setTimeout(() => { console.error('timeout'); p.kill(); process.exit(1); }, 60000);
