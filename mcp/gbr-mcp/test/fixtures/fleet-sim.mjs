// Full MCP-protocol fleet simulation over stdio, exactly as Claude/Grok would drive it.
import { spawn } from 'node:child_process';
const child = spawn('node', ['bin/gbr-mcp.js'], { stdio:['pipe','pipe','pipe'],
  env:{...process.env, GBR_MCP_LOG_LEVEL:'info', GBR_MCP_LOG_DIR:'/tmp/gbrlogs'} });
let buf='', id=1; const pend=new Map();
child.stdout.on('data',d=>{buf+=d;let i;while((i=buf.indexOf('\n'))>=0){const l=buf.slice(0,i).trim();buf=buf.slice(i+1);
  if(!l)continue;let m;try{m=JSON.parse(l)}catch{continue};if(m.id&&pend.has(m.id)){pend.get(m.id)(m);pend.delete(m.id)}}});
const send=(method,params={})=>new Promise(r=>{const n=id++;pend.set(n,r);
  child.stdin.write(JSON.stringify({jsonrpc:'2.0',id:n,method,params})+'\n')});
const call=async(name,args={})=>{const r=await send('tools/call',{name,arguments:args});
  return {isError:!!r.result?.isError, data:JSON.parse(r.result?.content?.[0]?.text??'{}')}};

await send('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'fleet-sim',version:'1'}});
child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized',params:{}})+'\n');

const line=(t)=>console.log('\n\x1b[1m'+t+'\x1b[0m');

line('1. gbr_devices — what can this hub dispatch to?');
const dv=(await call('gbr_devices')).data;
for(const d of dv.devices) console.log(`   ${d.id.padEnd(14)} ${d.kind.padEnd(7)} ${d.os.padEnd(7)} key=${d.has_key}`);

line('2. gbr_sessions — real injectable sessions?');
const ss=(await call('gbr_sessions')).data;
for(const s of ss.sessions) console.log(`   ${s.session_id.padEnd(16)} "${s.title}"  ${s.shell}  ${s.cwd}`);
console.log(`   _note: ${ss._note ?? '(none — real sessions present)'}`);

line('3. gbr_inject_and_wait — dispatch to LOCAL and read the output back');
const r1=(await call('gbr_inject_and_wait',{session_id:'linux-shell-1',text:'make test',device:'local',wait_ms:8000,poll_ms:300})).data;
console.log(`   command_id=${r1.command_id} completed=${r1.completed} items=${r1.item_count} truncated=${r1.truncated}`);
for(const i of r1.items) console.log(`     [${i.stream}] ${i.chunk}`);

line('4. gbr_inject_and_wait — dispatch to the REMOTE fleet device');
const r2=(await call('gbr_inject_and_wait',{session_id:'linux-shell-1',text:'npm run build',device:'studio-linux',wait_ms:8000,poll_ms:300})).data;
console.log(`   device echoed: ${r2.device?.id}  completed=${r2.completed} items=${r2.item_count}`);
for(const i of r2.items) console.log(`     [${i.stream}] ${i.chunk}`);
console.log(`   _warning: ${r2._warning ?? '(none — dispatched to the requested device)'}`);

line('5. GOTCHA — unknown device name silently falls back to local');
const r3=(await call('gbr_inject_and_wait',{session_id:'linux-shell-1',text:'echo hi',device:'typo-box',wait_ms:6000,poll_ms:300})).data;
console.log(`   requested: typo-box   actual: ${r3.device?.id}`);
console.log(`   _warning present: ${r3._warning ? 'YES' : 'NO — BUG'}`);

line('6. GOTCHA — logical error inside HTTP 200');
const r4=await call('gbr_inject',{text:'no session id'});
console.log(`   isError=${r4.isError} code=${r4.data.code}`);
console.log(`   hint: ${r4.data.hint}`);

line('7. gbr_output — page results by command_id');
const r5=(await call('gbr_output',{command_id:r1.command_id,limit:10})).data;
console.log(`   ${r5.items.length} items re-read for ${r1.command_id}`);

child.kill();
console.log('\n\x1b[1;32mFLEET SIMULATION COMPLETE\x1b[0m\n');
