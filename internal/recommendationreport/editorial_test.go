package recommendationreport

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func editorialReport(count int) core.ProductRecommendationReport {
	r := core.ProductRecommendationReport{Title: "합성 비교 리포트", Recommendation: core.ProductRecommendationResult{
		SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete,
	}}
	for i := 0; i < count; i++ {
		id := []string{"101", "102", "103"}[i]
		r.Recommendation.Candidates = append(r.Recommendation.Candidates, core.ProductRecommendationCandidate{
			Product:         core.ProductCard{Reference: core.ProductReference{ProductID: id}, Name: "Synthetic " + id},
			MissingEvidence: []string{"전원 규격 미확인"},
		})
	}
	return r
}

func TestEditorialReportRetainsEvidenceAndAccessibleNavigation(t *testing.T) {
	data, err := Render(editorialReport(2))
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{`content="light"`, `content="noindex,nofollow,noarchive"`, `href="#shortlist"`, `for="compare-a"`, `for="compare-b"`, "<noscript>", "검증 미완료", "전원 규격 미확인", "가격 미확인", "출처·범위·확인 시각 미확인", "표시 순서는 추천 순위가 아닙니다.", "prefers-reduced-motion", "@media print"} {
		if !strings.Contains(html, want) {
			t.Fatalf("editorial report lost %q", want)
		}
	}
	if strings.Index(html, `<section id="shortlist">`) > strings.Index(html, `<section id="research">`) {
		t.Fatal("research log precedes readable product comparison")
	}
	if strings.Contains(html, "Comic Sans") || strings.Contains(html, "box-shadow:") {
		t.Fatal("legacy visual treatment retained")
	}
}

func TestEditorialReportEmptyAndSingleCandidateStates(t *testing.T) {
	for _, count := range []int{0, 1} {
		data, err := Render(editorialReport(count))
		if err != nil {
			t.Fatal(err)
		}
		html := string(data)
		if !strings.Contains(html, "나란히 비교하려면 후보가 2개 이상 필요합니다.") || strings.Contains(html, `<select id="compare-a"`) {
			t.Fatal("empty comparison controls shown")
		}
		if strings.Contains(html, "표시할 비교 후보가 없습니다.") != (count == 0) {
			t.Fatal("empty shortlist state is incorrect")
		}
	}
}

func TestMinimalReportShowsPhotosAndEditorialNotesWithoutChangingEvidence(t *testing.T) {
	r := editorialReport(2)
	r.Summary = "<script>bad()</script> 조건부 판단"
	r.CandidateNotes = []core.ProductReportCandidateNote{{Reference: r.Recommendation.Candidates[0].Product.Reference, Title: "짧은 후보명", Rationale: "합성 선택 이유", Tradeoffs: []string{"합성 비용 차이"}}}
	r.Recommendation.Candidates[0].Product.ImageURL = "https://thumbnail.coupangcdn.com/synthetic.jpg"
	data, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{`<h3>짧은 후보명</h3>`, `src="https://thumbnail.coupangcdn.com/synthetic.jpg"`, "Synthetic 101", "합성 선택 이유", "합성 비용 차이", "선택 이유 · 추론", `class="appendix"`, "&lt;script&gt;bad()&lt;/script&gt;"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(html, "<script>bad()") || strings.Count(html, "</script>") != 1 {
		t.Fatal("editorial text became executable markup")
	}
}

func TestEditorialComparisonScriptChangesSelectionWithoutInterpretingMarkup(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for rendered interaction test")
	}
	r := editorialReport(3)
	r.Recommendation.Candidates[2].Product.Name = `<img src=x onerror=alert(1)>`
	data, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "-e", `
const fs=require('node:fs'), vm=require('node:vm'), assert=require('node:assert/strict');
class Node {
  constructor(tag){this.tag=tag;this.children=[];this.value='0';this.textContent='';this.hidden=true}
  appendChild(n){this.children.push(n)}
  replaceChildren(...nodes){this.children=nodes}
  prepend(n){this.children.unshift(n)}
}
const nodes=Object.fromEntries(['compare-a','compare-b','pairwise','compare-controls'].map(id=>['#'+id,new Node(id)]));
const document={querySelector(s){return nodes[s]||null},createElement(tag){return new Node(tag)}};
const source=fs.readFileSync(0,'utf8').match(/<script>([\s\S]*?)<\/script>/)[1];
vm.runInNewContext(source,{document},{timeout:1000});
const a=nodes['#compare-a'],b=nodes['#compare-b'],box=nodes['#pairwise'];
assert.equal(nodes['#compare-controls'].hidden,false);
assert.equal(a.children.length,3);assert.equal(b.value,'1');
const table=()=>box.children.find(n=>n.tag==='table');
const header=()=>table().children.find(n=>n.tag==='thead').children[0];
assert.equal(header().children[1].scope,'col');
assert.equal(header().children[2].textContent,'Synthetic 102');
b.value='2';b.onchange();
assert.equal(header().children[2].textContent,'<img src=x onerror=alert(1)>');
const body=table().children.find(n=>n.tag==='tbody');
assert.equal(body.children[0].children[0].scope,'row');
assert.equal(body.children[0].children[1].textContent,'가격 미확인');
a.value='2';a.onchange();
assert.equal(box.children[0].className,'same-choice');
b.value='1';b.onchange();assert.equal(box.children.length,1);
`)
	cmd.Stdin = strings.NewReader(string(data))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("comparison interaction failed: %v %s", err, output)
	}
}
