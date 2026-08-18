// Minimal gbr-agent Bot API emulator — matches the real v0.5.3 response shapes
// captured from the live agent on 2026-08-17, including its quirks.
import http from 'node:http';
const MB='gbr-linuxsim';
const sessions=[{session_id:'linux-shell-1',title:'ProjectX — grok · agent',shell:'bash',cwd:'/home/dev/projectx',os:'linux'}];
const devices=[{id:'local',kind:'local',name:'this PC',os:'linux',mailbox_id:MB,has_key:true},
               {id:'studio-linux',kind:'remote',name:'studio-linux',os:'linux',mailbox_id:'gbr-remote1',has_key:true}];
const outputs=new Map();
const now=()=>new Date().toISOString();
http.createServer((req,res)=>{
  const u=new URL(req.url,'http://x');
  const J=(c,o)=>{res.writeHead(c,{'Content-Type':'application/json'});res.end(JSON.stringify(o));};
  if(u.pathname==='/'||u.pathname==='/v1')
    return J(200,{ok:true,proto:'gbr/1',version:'v0.5.3',bind:'127.0.0.1',port:8788,
      mailbox_id:MB,require_key:false,service:'gbr-agent-bot',uptime_s:42});
  if(u.pathname==='/v1/status')
    return J(200,{ok:true,agent_online:true,agent_version:'v0.5.3',os:'linux',mailbox_id:MB,
      now:now(),session_count:sessions.length,sessions,uptime_s:42});
  if(u.pathname==='/v1/sessions') return J(200,{ok:true,mailbox_id:MB,now:now(),replace:true,sessions});
  if(u.pathname==='/v1/devices'&&req.method==='GET') return J(200,{ok:true,mailbox_id:MB,devices});
  if(u.pathname==='/v1/inject'&&req.method==='POST'){
    let b='';req.on('data',d=>b+=d);req.on('end',()=>{
      const p=JSON.parse(b||'{}');
      // real agent quirk: HTTP 200 even on refusal
      if(!p.session_id) return J(200,{ok:false,error:'inject: empty session_id refused',
        command_id:'x',device:{id:'local'},local:true,queued:false,session_id:''});
      // real agent quirk: unknown device silently falls back to local
      const dev=devices.find(d=>d.id===p.device)||devices[0];
      const cid='cmd-'+Math.random().toString(36).slice(2,8);
      const lines=[`$ ${p.text}`,`running on ${dev.id} (${dev.os})`,'exit 0'];
      outputs.set(cid,lines.map((c,i)=>({ts:new Date(Date.now()+i*10).toISOString(),
        session_id:p.session_id,command_id:cid,stream:'stdout',chunk:c,eof:i===lines.length-1})));
      J(200,{ok:true,command_id:cid,queued:true,device:{id:dev.id,kind:dev.kind,os:dev.os,mailbox_id:dev.mailbox_id},
        local:dev.id==='local',phone_status:true,session_id:p.session_id});
    });return;
  }
  if(u.pathname==='/v1/output'){
    const cid=u.searchParams.get('command_id');
    const after=u.searchParams.get('after');
    let items=outputs.get(cid)||[];
    if(after) items=items.filter(i=>i.ts>after);
    return J(200,{ok:true,mailbox_id:MB,now:now(),items});
  }
  J(404,{error:'not_found'});
}).listen(8788,'127.0.0.1',()=>console.error('fake gbr-agent on :8788'));
