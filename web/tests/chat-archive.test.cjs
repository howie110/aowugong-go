const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const ts = require('../node_modules/typescript');
const jsx = require('../node_modules/react/jsx-runtime');

function load(path, deps, globals = {}) {
 const code=ts.transpileModule(fs.readFileSync(require.resolve(path),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020,jsx:ts.JsxEmit.ReactJSX}}).outputText;
 const module={exports:{}};
 vm.runInNewContext(code,{module,exports:module.exports,URLSearchParams,Blob,URL,Error,...globals,require(name){if(name in deps)return deps[name];if(name==='react/jsx-runtime')return jsx;throw new Error(name);}});
 return module.exports;
}

test('API keeps query encoded',async()=>{
 const calls=[];
 const api=load('../src/lib/chat-archive.ts',{'@/lib/request':{requestJSON:async path=>{calls.push(path);return [];}}});
 await api.listChatAudit({q:'a&b',source:'codex',offset:20,conversation_id:'s&1',message_id:'id:m',order:'asc',snapshot:9});
 assert.equal(calls[0].split('?')[0],'/api/v1/chat-archive/messages');
 assert.equal(new URLSearchParams(calls[0].split('?')[1]).get('conversation_id'),'s&1');
 assert.equal(new URLSearchParams(calls[0].split('?')[1]).get('snapshot'),'9');
 assert.equal(new URLSearchParams(calls[0].split('?')[1]).get('q'),'a&b');
 assert.equal(new URLSearchParams(calls[0].split('?')[1]).get('offset'),'20');
});

test('statistics uses one dedicated request without message pagination',async()=>{
 const calls=[];const response={totals:{user:100,assistant:90},days:[{date:'2026-10-17',user:1,assistant:2}],before:'2026-10-11',has_more:true,unknown:{user:0,assistant:0},snapshot:19};
 const api=load('../src/lib/chat-archive.ts',{'@/lib/request':{requestJSON:async path=>{calls.push(path);return response;}}});
 assert.equal(await api.listChatStatistics(19,'2026-10-18'),response);
 assert.equal(calls.length,1);assert.equal(calls[0],'/api/v1/chat-archive/statistics?snapshot=19&before=2026-10-18');
});

test('archive page permission cannot be granted to non-admin',()=>{
 const {canViewChatArchive}=load('../src/lib/chat-archive.ts',{'@/lib/request':{},'@/lib/auth':{}});
 assert.equal(canViewChatArchive({roles:['investor'],permissions:['page:chat_archive']}),false);
 assert.equal(canViewChatArchive({roles:['admin'],permissions:[]}),true);
});

function harness(globals = {}){
 const states=[];let cursor=0,effect;const calls=[];
 const hooks={useState(init){const i=cursor++;if(!(i in states))states[i]=init;return[states[i],v=>states[i]=typeof v==='function'?v(states[i]):v];},useRef(init){const i=cursor++;return states[i]??(states[i]={current:init});},useEffect(fn){effect=fn;}};
 const api={listChatStatistics:async (snapshot,before)=>{calls.push(['daily',snapshot,before]);return {snapshot,totals:{user:300,assistant:210},unknown:{user:0,assistant:0},days:[{date:'2026-10-17',user:30,assistant:21}],before:'2026-10-11',has_more:true};},listChatAudit:async f=>{calls.push(['list',f]);return {messages:[{id:1,message_key:'id:m',body:'<img src=x onerror=alert(1)>',role:'user',source:'codex',source_id:'session-1',device_id:'mac-1',source_time:null,observed_at:'2026-10-01T00:00:00Z'}],counts:{conversations:12,messages:24,failed_sync_states:2},total:40,has_more:f.offset===0,snapshot:9};}};
 const page=load('../src/pages/chat-archive.tsx',{'react':hooks,'@/lib/chat-archive':api,'@/components/ui/button':{Button:'button'},'@/components/ui/input':{Input:'input'}},globals);
 return{calls,render(){cursor=0;return page.ChatArchivePage();},async mount(){this.render();effect();await new Promise(r=>setImmediate(r));return this.render();}};
}
function all(node,fn){if(!node||typeof node!=='object')return[];if(Array.isArray(node))return node.flatMap(n=>all(n,fn));return[...(fn(node)?[node]:[]),...all(node.props?.children,fn)];}
const button=(tree,label)=>all(tree,n=>n.type==='button'&&n.props.children===label)[0];
test('compact list keeps text and audit identity inside collapsed details',async()=>{
 const h=harness();let tree=await h.mount();
 const disclosures=all(tree,n=>n.type==='details');
 assert.equal(disclosures.length,1);
 assert.ok(!disclosures[0].props.open);
 assert.equal(all(disclosures[0],n=>n.type==='summary').length,1);
 assert.equal(all(disclosures[0],n=>n.type==='pre').length,1);
 const summary=all(disclosures[0],n=>n.type==='summary')[0];
 assert.ok(!JSON.stringify(summary).includes('session-1'));
 assert.ok(JSON.stringify(disclosures[0]).includes('session-1'));
 assert.equal(all(tree,n=>n.type==='article').length,0);
 assert.ok(all(tree,n=>n.type==='pre'&&n.props.children==='<img src=x onerror=alert(1)>').length);
 assert.equal(all(tree,n=>n.type==='img'||n.props?.dangerouslySetInnerHTML).length,0);
 assert.equal(all(tree,n=>n.type==='pre'&&typeof n.props.children==='string'&&n.props.children.includes('reference_only')).length,0);
 const rendered=JSON.stringify(tree);
 for(const value of ['id:m','session-1','mac-1','来源时间未知','observed_at']) assert.ok(rendered.includes(value),value);
 assert.equal(button(tree,'查看版本与来源'),undefined);
 assert.equal(all(tree,n=>n.type==='aside').length,0);
 assert.ok(all(tree,n=>n.type==='button'&&n.props['aria-label']?.startsWith('复制')).length>=3);
 assert.equal(button(tree,'导出消息'),undefined);
 assert.equal(button(tree,'导出原始记录'),undefined);
});
test('copy buttons copy original identifiers and report clipboard errors',async()=>{
 const copied=[];
 const h=harness({navigator:{clipboard:{writeText:async value=>copied.push(value)}}});
 let tree=await h.mount();
 for(const control of all(tree,n=>n.type==='button'&&n.props['aria-label']?.startsWith('复制'))) await control.props.onClick();
 assert.deepEqual(copied,['id:m','session-1','mac-1']);
 assert.ok(JSON.stringify(h.render()).includes('已复制 device_id'));
 const failed=harness({navigator:{clipboard:{writeText:async()=>{throw new Error('denied');}}}});
 tree=await failed.mount();
 await all(tree,n=>n.type==='button'&&n.props['aria-label']?.startsWith('复制'))[0].props.onClick();
 assert.ok(JSON.stringify(failed.render()).includes('复制失败'));
});
test('daily statistics replace filters and remain unchanged when paging',async()=>{
 const h=harness();let tree=await h.mount();
 assert.equal(all(tree,n=>n.type==='form').length,0);
 const stats=all(tree,n=>n.props?.['aria-label']==='每日消息统计')[0];
 assert.ok(JSON.stringify(stats).includes('2026-10-17'));
 assert.ok(all(stats,n=>n.type==='td'&&n.props.children===30).length);
 assert.ok(all(stats,n=>n.type==='td'&&n.props.children===21).length);
 assert.equal(h.calls.filter(c=>c[0]==='daily').length,1);
 assert.equal(h.calls.find(c=>c[0]==='daily')[1],9);
 await button(tree,'下一页').props.onClick();
 assert.equal(h.calls.at(-1)[1].offset,20);
 assert.equal(h.calls.at(-1)[1].snapshot,9);
 assert.equal(h.calls.filter(c=>c[0]==='daily').length,1);
});

test('total card uses server totals rather than seven day counts and survives paging',async()=>{
 const h=harness();let tree=await h.mount();
 const totals=t=>all(all(t,n=>n.props?.['aria-label']==='消息总量统计')[0],n=>n.type==='dd').map(n=>n.props.children);
 assert.deepEqual(totals(tree),[510,300,210]);
 await button(tree,'下一页').props.onClick();
 assert.deepEqual(totals(h.render()),[510,300,210]);
 assert.equal(h.calls.filter(c=>c[0]==='daily').length,1);
});

test('older dates load only on demand',async()=>{const h=harness();const tree=await h.mount();await button(tree,'更早 7 天').props.onClick();assert.equal(h.calls.filter(c=>c[0]==='daily').length,2);assert.deepEqual(h.calls.at(-1),['daily',9,'2026-10-11']);assert.equal(h.calls.filter(c=>c[0]==='list').length,1);});
