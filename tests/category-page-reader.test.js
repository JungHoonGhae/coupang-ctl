import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const source = readFileSync(new URL('../internal/browser/category_page_reader.js', import.meta.url), 'utf8').replace('export function', 'function');
const target = 'https://www.coupang.com/vp/products/101?vendorItemId=201';
const product = {'@type': 'Product', url: target, name: 'Synthetic private product', secret: 'must-not-leave-page'};
const breadcrumb = {'@type': 'BreadcrumbList', secret: 'must-not-leave-page', itemListElement: [
  {'@type': 'ListItem', position: 1, name: 'Home', item: 'https://www.coupang.com/'},
  {'@type': 'ListItem', position: 2, name: 'Synthetic category', item: 'https://www.coupang.com/np/categories/100', secret: 'must-not-leave-page'},
]};
function read({href = target, expected = target, scripts = [product, breadcrumb], ready = 'complete', status = 200, title = 'Synthetic', canonical, password = false, URLType = URL} = {}) {
  const context = vm.createContext({URL: URLType, location: {href}, performance: {getEntriesByType: () => [{name: href, responseStatus: status}]}, document: {
    readyState: ready, title, body: {innerText: ''},
    querySelector: selector => selector.includes('password') ? (password ? {} : null) : (canonical ? {href: canonical} : null),
    querySelectorAll: () => scripts.map(value => ({textContent: JSON.stringify(value)})),
  }});
  vm.runInContext(source, context);
  return JSON.parse(JSON.stringify(context.readProductCategoryPage(expected)));
}
test('category result retains only source-native breadcrumb fields', () => {
  const result = read();
  assert.equal(result.status, 'ok');
  assert.deepEqual(result.category.json_ld, [{'@type': 'BreadcrumbList', itemListElement: [
    {'@type': 'ListItem', position: 1, name: 'Home', item: 'https://www.coupang.com/'},
    {'@type': 'ListItem', position: 2, name: 'Synthetic category', item: 'https://www.coupang.com/np/categories/100'},
  ]}]);
  assert.equal(JSON.stringify(result).includes('must-not-leave-page'), false);
  assert.equal(JSON.stringify(result).includes(product.name), false);
});
test('page URL polyfills without iterable keys preserve the same target validation', () => {
  class PageURL extends URL {
    constructor(...args) {
      super(...args);
      this.searchParams.keys = () => ({next: () => ({done: true})});
    }
  }
  assert.equal(read({URLType: PageURL}).status, 'ok');
  for (const href of [target + '&vendorItemId=999', target + '&checkout=true', target + '#other']) {
    assert.equal(read({URLType: PageURL, href}).status, 'category_data_missing');
  }
});
test('category absence needs a loaded matching product, not an empty or failed page', () => {
  assert.deepEqual(read({scripts: [product]}), {status: 'ok', category: {json_ld: []}});
  assert.equal(read({scripts: []}).status, 'loading');
  assert.equal(read({scripts: [{...product, url: target.replace('/101', '/999')}]}).status, 'loading');
  assert.equal(read({ready: 'interactive'}).status, 'loading');
  assert.equal(read({status: 500}).status, 'category_data_missing');
});
test('category denial, login, and confirmed not-found remain distinct', () => {
  for (const status of [403, 429]) assert.equal(read({status}).status, 'access_denied');
  for (const status of [404, 410]) assert.equal(read({status}).status, 'category_unavailable');
  assert.equal(read({title: 'CAPTCHA'}).status, 'access_denied');
  assert.equal(read({href: 'https://login.coupang.com/login/login.pang'}).status, 'authentication_required');
  assert.equal(read({password: true}).status, 'authentication_required');
});
test('unrelated or ambiguous identities cannot return category evidence', () => {
  for (const href of [target.replace('/101', '/999'), target.replace('www.coupang.com', 'evil.invalid'), target + '&vendorItemId=999', target + '#other']) {
    assert.equal(read({href}).status, 'category_data_missing');
    assert.equal(read({expected: href}).status, 'category_data_missing');
  }
  assert.equal(read({canonical: 'https://www.coupang.com/vp/products/999'}).status, 'category_data_missing');
});
test('malformed or conflicting breadcrumbs are not cached as missing', () => {
  for (const row of [
    {...breadcrumb.itemListElement[1], item: 'https://user:pass@www.coupang.com/np/categories/100'},
    {...breadcrumb.itemListElement[1], item: 'https://www.coupang.com/np/categories/100?token=synthetic'},
    {...breadcrumb.itemListElement[1], position: 1},
    {...breadcrumb.itemListElement[1], name: ''},
  ]) assert.equal(read({scripts: [{...breadcrumb, itemListElement: [breadcrumb.itemListElement[0], row]}]}).status, 'category_data_missing');
  const different = {...breadcrumb, itemListElement: [breadcrumb.itemListElement[0], {...breadcrumb.itemListElement[1], name: 'Other'}]};
  assert.equal(read({scripts: [breadcrumb, different]}).status, 'category_data_missing');
  assert.equal(read({scripts: [{'@graph': [breadcrumb, breadcrumb]}]}).category.json_ld.length, 1);
});
