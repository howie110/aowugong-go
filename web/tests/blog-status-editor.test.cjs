const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');
const ts = require('../node_modules/typescript');
const jsx = require('../node_modules/react/jsx-runtime');
function load(file, dependencies = {}, globals = {}) {
  const source = fs.readFileSync(path.resolve(__dirname, '../src/pages/blog', file), 'utf8');
  const code = ts.transpileModule(source, {compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020,jsx:ts.JsxEmit.ReactJSX}}).outputText;
  const module = {exports:{}};
  vm.runInNewContext(code,{module,exports:module.exports,Uint8Array,crypto:require('node:crypto').webcrypto,URL,window:{confirm:()=>true},require:(name)=>{if(name in dependencies)return dependencies[name];throw new Error(name);},...globals});
  return module.exports;
}
const images = load('status-images.ts');
const png = new File([new Uint8Array([137,80,78,71,13,10,26,10,0,0,0,0])], 'origin', {type:'application/octet-stream'});
test('微信 origin 按文件内容验证，不依赖扩展名或 MIME；拒绝非图片和超限文件', async()=>{
  await images.validateStatusFile(png);
  await assert.rejects(images.validateStatusFile(new File(['<svg/>'],'fake.jpg',{type:'image/jpeg'})),/仅支持/);
  await assert.rejects(images.validateStatusFile({name:'large',size:10*1024*1024+1}),/10 MiB/);
});
test('上传中断后记住成功图片，重试只传失败项并保留排序',async()=>{
  const existing={key:'old',url:'old'};const first={key:'new1',url:'new1'};const second={key:'new2',url:'new2'};
  const items=[{id:'old',image:existing},{id:'one',file:png},{id:'two',file:png}];let count=0;
  const remember=(id,image)=>{items.find(x=>x.id===id).image=image;};
  await assert.rejects(images.prepareStatusImages(items,async()=>{if(++count===2)throw new Error('offline');return first;},remember,()=>{}),/offline/);
  const result=await images.prepareStatusImages(items,async()=>{count++;return second;},remember,()=>{});
  assert.equal(count,3);assert.deepEqual(Array.from(result),[existing,first,second]);
});
function editor(initial) {
  const state=[];let cursor=0;let counter=0;const calls=[];const revoked=[];
  const hooks={useState:(init)=>{const i=cursor++;if(!(i in state))state[i]=typeof init==='function'?init():init;return [state[i],value=>{state[i]=typeof value==='function'?value(state[i]):value;}];},useRef:(value)=>{const i=cursor++;return state[i]??(state[i]={current:value});},useEffect:()=>{}};
  const api={uploadStatusImage:async()=>{calls.push('upload');return {key:'new',url:'https://example/new'};},saveStatus:async(input,create)=>{calls.push({input,create});return {...input,created_at:'2026-10-03',updated_at:'2026-10-03'};}};
  const {StatusEditor}=load('status-editor.tsx',{'react':hooks,'react/jsx-runtime':jsx,'@/components/ui/button':{Button:'button'},'@/lib/navigation-guard':{guardUnsavedChanges:()=>{}},'@/lib/blog':api,'./status-images':images},{URL:{createObjectURL:()=>`blob:${++counter}`,revokeObjectURL:url=>revoked.push(url)}});
  const saved=[];const render=()=>{cursor=0;return StatusEditor({initial,onSaved:s=>saved.push(s),onCancel:()=>calls.push('cancel')});};
  return {render,calls,saved,revoked};
}
function find(tree, predicate) {if(!tree||typeof tree!=='object')return null;if(predicate(tree))return tree;for(const child of [tree.props?.children].flat(Infinity)){const match=find(child,predicate);if(match)return match;}return null;}
const tick=()=>new Promise(resolve=>setImmediate(resolve));
test('选择和移除图片不会上传，点发布才上传并保存',async()=>{
  const e=editor();let tree=e.render();
  find(tree,x=>x.type==='input').props.onChange({target:{files:[png],value:'file'}});await tick();
  assert.equal(e.calls.length,0);tree=e.render();
  assert.equal(find(tree,x=>x.type==='img').props.src,'blob:1');
  find(tree,x=>x.props?.['aria-label']==='移除图片 1').props.onClick();tree=e.render();assert.equal(e.calls.length,0);assert.deepEqual(e.revoked,['blob:1']);
  find(tree,x=>x.type==='input').props.onChange({target:{files:[png],value:'file'}});await tick();tree=e.render();
  await find(tree,x=>x.type==='button'&&x.props.children==='发布').props.onClick();
  assert.equal(e.calls[0],'upload');assert.equal(e.calls[1].create,true);assert.equal(e.saved.length,1);
});
test('编辑保留原图，取消不写入，保存调用更新并保留发布状态',async()=>{
  const e=editor({id:'existing',body:'before',images:[{key:'old',url:'old'}],published:true});let tree=e.render();
  find(tree,x=>x.type==='textarea').props.onChange({target:{value:'after'}});tree=e.render();
  find(tree,x=>x.type==='button'&&x.props.children==='取消').props.onClick();assert.deepEqual(e.calls,['cancel']);
  await find(tree,x=>x.type==='button'&&x.props.children==='保存').props.onClick();
  const call=e.calls[1];assert.equal(call.create,false);assert.equal(call.input.id,'existing');assert.equal(call.input.body,'after');assert.equal(call.input.images[0].key,'old');assert.equal(e.calls.includes('upload'),false);
});
test('管理页读取全部分页，删除取消不调用接口，失败保留条目，成功移除',async()=>{
  const state=[];let cursor=0;let effect;let confirm=false;let fail=false;let deletes=0;const offsets=[];
  const rows=Array.from({length:21},(_,i)=>({id:`status-${i}`,body:`内容 ${i}`,images:[],published:true,published_at:'2026-10-03T10:00:00Z',created_at:'2026-10-03T10:00:00Z',updated_at:'2026-10-03T10:00:00Z'}));
  const hooks={useState:(init)=>{const i=cursor++;if(!(i in state))state[i]=typeof init==='function'?init():init;return[state[i],v=>{state[i]=typeof v==='function'?v(state[i]):v;}];},useEffect:fn=>{effect=fn;}};
  const {BlogStatusPage}=load('../blog-status.tsx',{'react':hooks,'react/jsx-runtime':jsx,'@/components/ui/button':{Button:'button'},'./blog/status-editor':{StatusEditor:'editor'},'./blog/styles.css':{},'@/lib/blog':{listStatuses:async(_admin,offset)=>{offsets.push(offset);return rows.slice(offset,offset+20);},deleteStatus:async()=>{deletes++;if(fail)throw new Error('删除失败');}}},{window:{confirm:()=>confirm}});
  const render=()=>{cursor=0;return BlogStatusPage();};
  render();effect();await tick();let tree=render();assert.deepEqual(offsets,[0,20]);assert.equal(state[0].length,21);
  const del=()=>find(tree,x=>x.type==='button'&&x.props.children==='删除').props.onClick();
  del();await tick();assert.equal(deletes,0);
  confirm=true;fail=true;del();await tick();tree=render();assert.equal(state[0].length,21);assert.ok(find(tree,x=>x.props?.role==='alert'));
  fail=false;del();await tick();tree=render();assert.equal(state[0].length,20);assert.equal(state[0].some(x=>x.id==='status-0'),false);
});
