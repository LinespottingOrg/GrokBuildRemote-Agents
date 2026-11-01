import { agentProcessLines } from '../src/agentproc.js';

let failed = 0;
const check = (name, cond) => {
  if (cond) console.log(`  PASS  ${name}`);
  else {
    failed++;
    console.log(`  FAIL  ${name}`);
  }
};

const mac = agentProcessLines('darwin', '17656 /usr/local/bin/gbr-agent -log=info run\n999 grok\n');
check('mac keeps the run line', mac.length === 1 && mac[0].includes('gbr-agent'));

const win = agentProcessLines('win32', '"gbr-agent.exe","17656","Console","1","20,000 K"\r\nINFO: no tasks\r\n');
check('windows keeps the image row', win.length === 1 && /gbr-agent\.exe/i.test(win[0]));

const none = agentProcessLines('win32', 'INFO: No tasks are running which match the specified criteria.');
check('windows info line is not an agent', none.length === 0);

if (failed) process.exit(1);
