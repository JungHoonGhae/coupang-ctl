import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const script = readFileSync(new URL('../internal/recommendationreport/radar.js', import.meta.url), 'utf8');
const axes = ['a', 'b', 'c'].map(id => ({id, label: id, weight: 1 / 3}));
function render(values, extra = []) {
  const svg = {
    children: [], replaceChildren() { this.children = []; }, appendChild(node) { this.children.push(node); },
    ownerDocument: {createElementNS(_ns, tag) { return {tag, attrs: {}, setAttribute(key, value) { this.attrs[key] = value; }}; }},
  };
  const context = vm.createContext({svg, data: {axes, scores: [{product_id: '101', values}, ...extra]}});
  vm.runInContext(script + '\nrenderRecommendationRadar(svg, data)', context);
  return svg.children.filter(node => node.attrs['data-role'] === 'score');
}
const values = numbers => numbers.map((score, i) => ({axis_id: axes[i].id, score}));

test('null and absent axes never become zero-valued polygons', () => {
  assert.equal(render(values([50, null, 50])).length, 0);
  assert.equal(render(values([50, 50])).length, 0);
});
test('an actual zero is retained as the center point of a complete polygon', () => {
  const [polygon] = render(values([0, 50, 100]));
  assert.ok(polygon);
  assert.equal(polygon.attrs.points.split(' ')[0], '380,290');
});
test('missing candidate cannot suppress the other complete candidate', () => {
  const polygons = render(values([null, 50, 50]), [{product_id: '102', values: values([50, 50, 50])}]);
  assert.equal(polygons.length, 1);
  assert.equal(polygons[0].attrs['data-product-id'], '102');
});
test('invalid numeric values are not drawn or coerced', () => {
  for (const invalid of [NaN, Infinity, -1, 101, '50', false, undefined]) {
    assert.equal(render(values([invalid, 50, 50])).length, 0);
  }
});
