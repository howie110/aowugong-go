const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const ts = require('../node_modules/typescript');
const jsx = require('../node_modules/react/jsx-runtime');

// 只替换 UI 原语和网络，实际执行页面、表单及排序逻辑。
function harness(rows = []) {
  const states = []; let cursor = 0; let effect; let fail = false; let saveWait;
  const calls = [];
  const hooks = {
    useState(init) { const i = cursor++; if (!(i in states)) states[i] = typeof init === 'function' ? init() : init; return [states[i], v => { states[i] = typeof v === 'function' ? v(states[i]) : v; }]; },
    useMemo: fn => fn(), useEffect: fn => { effect = fn; },
    useRef(init) { const i=cursor++; return states[i] ?? (states[i]={current:init}); },
  };
  const api = {
    fetchSubscriptions: async () => rows,
    createSubscription: async data => { calls.push(['create', data]); await saveWait; if (fail) throw new Error('failed'); },
    updateSubscription: async (id,data) => { calls.push(['update', id, data]); await saveWait; if (fail) throw new Error('failed'); },
    deleteSubscription: async id => { calls.push(['delete',id]); },
  };
  const code = ts.transpileModule(fs.readFileSync(require.resolve('../src/pages/subscriptions.tsx'), 'utf8'), {compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020,jsx:ts.JsxEmit.ReactJSX}}).outputText;
  const module={exports:{}};
  vm.runInNewContext(code,{module,exports:module.exports,require(name){
    if(name==='react')return hooks;
    if(name==='react/jsx-runtime')return jsx;
    if(name==='@/lib/subscriptions')return api;
    if(name==='@/lib/notify')return {notify:{warning(){},success(){},errorFrom(){}}};
    if(name.startsWith('@/components/')||name==='lucide-react')return new Proxy({}, {get:(_,key)=>String(key)});
    throw new Error(name);
  }});
  function expand(node) {
    if(!node||typeof node!=='object')return node;
    if(Array.isArray(node))return node.map(expand);
    if(typeof node.type==='function')return expand(node.type(node.props));
    return {...node,props:{...node.props,children:expand(node.props?.children)}};
  }
  return {calls,setFail(v){fail=v;},holdSave(promise){saveWait=promise;},render(){cursor=0;return expand(module.exports.SubscriptionsPage());},async mount(){this.render();effect();await new Promise(r=>setImmediate(r));return this.render();}};
}
function all(node,predicate) {
  if(!node||typeof node!=='object')return [];
  if(Array.isArray(node))return node.flatMap(n=>all(n,predicate));
  return [...(predicate(node)?[node]:[]),...all(node.props?.children,predicate)];
}
const byType=(tree,type)=>all(tree,n=>n.type===type);
const button=(tree,text)=>all(tree,n=>n.type==='Button'&&[n.props.children].flat(Infinity).includes(text))[0];
const row=(id,expired)=>({id,service_name:'service-'+id,category:'IT',annual_fee:'12',monthly_fee:'1',expires_on:'2026-10-20',current_status:expired?'已结束':'订阅中',days_until_expiry:expired?-1:4});

test('已结束置底且两组内保留接口排序，不改变源数组',async()=>{
  const rows=[row(1,true),row(2,true),row(3,false),row(4,false)];const h=harness(rows);const tree=await h.mount();
  const names=all(tree,n=>n.type==='div'&&n.props.className==='font-medium').map(n=>n.props.children);
  assert.deepEqual(names,['service-3','service-4','service-1','service-2']);assert.deepEqual(rows.map(r=>r.id),[1,2,3,4]);
});
test('新增打开空白弹框，取消不保存，再打开不残留；成功关闭',async()=>{
  const h=harness();let tree=await h.mount();assert.equal(byType(tree,'Dialog')[0]?.props.open,false);
  button(tree,'新增订阅').props.onClick();tree=h.render();assert.equal(byType(tree,'Dialog')[0].props.open,true);
  byType(tree,'Input')[0].props.onChange({target:{value:'temp'}});tree=h.render();button(tree,'取消').props.onClick();assert.equal(h.calls.length,0);
  tree=h.render();button(tree,'新增订阅').props.onClick();tree=h.render();assert.equal(byType(tree,'Input')[0].props.value,'');
  byType(tree,'Input')[0].props.onChange({target:{value:'new'}});tree=h.render();byType(tree,'DatePicker')[1].props.onChange('2026-12-01');tree=h.render();
  await button(tree,'保存').props.onClick();tree=h.render();assert.equal(h.calls[0][0],'create');assert.equal(byType(tree,'Dialog')[0].props.open,false);
});
test('编辑回填；失败保留输入和弹框；重试成功更新原记录',async()=>{
  const h=harness([row(7,false)]);let tree=await h.mount();all(tree,n=>n.props?.['aria-label']==='编辑订阅')[0].props.onClick();tree=h.render();
  assert.equal(byType(tree,'Dialog')[0]?.props.open,true);assert.equal(byType(tree,'Input')[0].props.value,'service-7');
  byType(tree,'Input')[0].props.onChange({target:{value:'edited'}});tree=h.render();h.setFail(true);await button(tree,'保存').props.onClick();tree=h.render();
  assert.equal(byType(tree,'Dialog')[0].props.open,true);assert.equal(byType(tree,'Input')[0].props.value,'edited');
  h.setFail(false);await button(tree,'保存').props.onClick();assert.equal(h.calls[1][0],'update');assert.equal(h.calls[1][1],7);assert.equal(byType(h.render(),'Dialog')[0].props.open,false);
});

test('保存期间锁定表单、阻止关闭和重复提交',async()=>{
  const h=harness([row(1,false)]);let tree=await h.mount();all(tree,n=>n.props?.['aria-label']==='编辑订阅')[0].props.onClick();tree=h.render();
  let release;h.holdSave(new Promise(resolve=>{release=resolve;}));
  const save=button(tree,'保存').props.onClick;const pending=save();await save();assert.equal(h.calls.length,1);
  tree=h.render();assert.equal(byType(tree,'fieldset')[0].props.disabled,true);
  byType(tree,'Dialog')[0].props.onOpenChange(false);assert.equal(byType(h.render(),'Dialog')[0].props.open,true);
  release();await pending;assert.equal(byType(h.render(),'Dialog')[0].props.open,false);
});
