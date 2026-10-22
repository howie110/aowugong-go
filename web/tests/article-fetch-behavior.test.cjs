const test = require('node:test');
const assert = require('node:assert/strict');
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
const { loadTypeScript } = require('./helpers/load-typescript.cjs');
const settle = () => new Promise(resolve => setImmediate(resolve));

// 只替换页面的 hooks 和 API 边界，执行真实加载与按钮回调；子组件正常渲染。
async function page(state = 'connected', actionOverrides = {}) {
  const values = []; const effects = []; const calls = []; const notices = [];
  let cursor = 0; let mounted = false;
  const hooks = { ...React,
    useState(initial) {
      const slot = cursor++;
      if (!(slot in values)) values[slot] = typeof initial === 'function' ? initial() : initial;
      return [values[slot], value => { values[slot] = typeof value === 'function' ? value(values[slot]) : value; }];
    },
    useRef: initial => ({ current: initial }),
    useMemo: compute => compute(),
    useEffect(effect) { if (!mounted) effects.push(effect); },
  };
  const api = {
    fetchArticleSources: async () => [],
    fetchWeReadArticleBinding: async () => ({ state, accounts: [{ account_id: 'a', title: '测试公众号', article_count: 3, today_inserted_count: 1, unparsed_count: 0, pending_count: 2 }] }),
    fetchArticleAnalysisModelSettings: async () => ({ models: [], analysis_prompt: 'prompt', prompt_version: 'v1' }),
  };
  for (const name of ['syncArticles', 'parsePendingArticles', 'analyzePendingArticles']) {
    api[name] = async (...args) => {
      calls.push([name, ...args]);
      if (actionOverrides[name]) return actionOverrides[name](...args);
      return { analyzed_count: 1, classified_alias_count: 0, skipped_count: 0, error_count: 0 };
    };
  }
  const { ArticleFetchPage } = loadTypeScript('pages/finance/article-fetch.tsx', {
    moduleMocks: { 'pages/finance/article-fetch.tsx': { react: hooks } },
    mocks: { '@/lib/article-analysis': api, '@/lib/notify': { notify: Object.fromEntries(['success','warning','errorFrom'].map(name => [name, (...args) => notices.push([name, ...args])])) } },
  });
  const render = () => { cursor = 0; return ArticleFetchPage(); };
  render(); mounted = true; effects.forEach(effect => effect()); await settle();
  return { render, calls, notices };
}
function elements(tree) {
  if (!React.isValidElement(tree)) return [];
  return [tree, ...React.Children.toArray(tree.props.children).flatMap(elements)];
}
function text(tree) {
  if (typeof tree === 'string') return tree;
  if (!React.isValidElement(tree)) return '';
  return React.Children.toArray(tree.props.children).map(text).join('').trim();
}
function button(tree, label) {
  const found = elements(tree).find(e => typeof e.props.onClick === 'function' && text(e) === label);
  assert.ok(found, `missing button ${label}`); return found;
}
for (const [state, label] of [['connected','可用'], ['failed','已失效'], ['disconnected','未绑定']]) {
  test(`binding ${state} shows ${label} and keeps discovered accounts visible`, async () => {
    const p = await page(state); const html = renderToStaticMarkup(p.render());
    for (const expected of ['信息源','微信读书绑定','获取公众号','测试公众号',label]) assert.ok(html.includes(expected));
    assert.ok(!elements(p.render()).some(e => typeof e.props.onClick === 'function' && text(e) === '刷新'));
  });
}
test('article commands keep equal width, call the intended API, and disable while pending', async () => {
  for (const [label, name, args] of [['爬取','syncArticles',[30,false,1]], ['解析','parsePendingArticles',[30]], ['分析','analyzePendingArticles',[50,true]]]) {
    let finish;
    const p = await page('connected', { [name]: () => new Promise(resolve => { finish = resolve; }) });
    const tree = p.render();
    const widths = ['爬取','解析','分析'].map(label => button(tree,label).props.className.match(/\bw-\S+/)?.[0]);
    assert.ok(widths[0]); assert.ok(widths.every(w => w === widths[0]));
    button(tree,label).props.onClick();
    assert.deepEqual(p.calls, [[name,...args]]);
    for (const action of ['爬取','解析','分析']) assert.equal(button(p.render(),action).props.disabled,true);
    finish({ analyzed_count: 1, classified_alias_count: 0, skipped_count: 0, error_count: 0 }); await settle();
    for (const action of ['爬取','解析','分析']) assert.equal(button(p.render(),action).props.disabled,false);
  }
});
test('failed article operation reports the error and restores controls', async () => {
  const failure = new Error('test failure');
  const p = await page('connected', { analyzePendingArticles: async () => { throw failure; } });
  button(p.render(),'分析').props.onClick(); await settle();
  assert.ok(p.notices.some(([kind,error]) => kind === 'errorFrom' && error === failure));
  assert.equal(button(p.render(),'分析').props.disabled,false);
});
