// Missing axes are not zero scores. The table remains the full accessible
// representation; only complete, finite sets can form a comparison polygon.
function renderRecommendationRadar(svg, data) {
  if (!svg || !Array.isArray(data?.axes) || data.axes.length < 3) return;
  const doc = svg.ownerDocument, ns = 'http://www.w3.org/2000/svg';
  const axes = data.axes, n = axes.length, cx = 380, cy = 290, radius = 210;
  const point = (i, value) => {
    const angle = -Math.PI / 2 + i * 2 * Math.PI / n;
    return [cx + Math.cos(angle) * radius * value, cy + Math.sin(angle) * radius * value];
  };
  const element = (tag, attrs, text) => {
    const node = doc.createElementNS(ns, tag);
    for (const [key, value] of Object.entries(attrs)) node.setAttribute(key, String(value));
    if (text !== undefined) node.textContent = text;
    svg.appendChild(node);
    return node;
  };
  svg.replaceChildren();
  element('title', {id: 'comparison-radar-title'}, '입력된 비교 축의 점수');
  element('desc', {id: 'comparison-radar-desc'}, '모든 축에 값이 있는 후보만 0부터 100까지 표시합니다. 미확인 값과 자세한 근거는 위 표에서 확인하세요.');
  if (axes.length > 5 || (data.scores ?? []).length > 5) {
    element('text', {x: 24, y: 40, fill: '#4f5d75'}, '비교 항목이 많아 위 표로 표시합니다.');
    return;
  }
  for (let ring = 1; ring <= 5; ring++) {
    element('polygon', {points: axes.map((_, i) => point(i, ring / 5).join(',')).join(' '), fill: 'none', stroke: '#bfc0c0'});
    element('text', {x: cx - 8, y: cy - radius * ring / 5, 'text-anchor': 'end', fill: '#4f5d75', 'font-size': 12}, String(ring * 20));
  }
  axes.forEach((axis, i) => {
    const [endX, endY] = point(i, 1);
    element('line', {x1: cx, y1: cy, x2: endX, y2: endY, stroke: '#bfc0c0'});
    const [x, y] = point(i, 1.15);
    element('text', {x, y, 'text-anchor': 'middle', fill: '#2d3142', 'font-size': 16}, axis.label + ' ' + Math.round(axis.weight * 100) + '%');
  });
  // No candidate is visually endorsed: these are the non-focal series tokens.
  const colors = ['#7c8f6f', '#5e7a9b', '#b8915a', '#9c6b50', '#6e6479'];
  const legend = [];
  for (const [index, score] of (data.scores ?? []).entries()) {
    const byAxis = new Map((score.values ?? []).map(value => [value.axis_id, value.score]));
    const complete = axes.every(axis => typeof byAxis.get(axis.id) === 'number' && Number.isFinite(byAxis.get(axis.id)) && byAxis.get(axis.id) >= 0 && byAxis.get(axis.id) <= 100);
    if (!complete) continue;
    const color = colors[index % colors.length];
    legend.push({color, label: '상품 ' + score.product_id});
    element('polygon', {
      'data-product-id': score.product_id, 'data-role': 'score',
      points: axes.map((axis, i) => point(i, byAxis.get(axis.id) / 100).join(',')).join(' '),
      fill: color, 'fill-opacity': .18, stroke: color, 'stroke-width': 1.5,
    });
  }
  element('line', {x1: 24, y1: 540, x2: 736, y2: 540, stroke: '#bfc0c0'});
  legend.forEach((item, index) => {
    const x = 24 + index * 144;
    element('rect', {x, y: 564, width: 16, height: 8, fill: item.color});
    element('text', {x: x + 24, y: 576, fill: '#4f5d75', 'font-size': 12}, item.label);
  });
}
