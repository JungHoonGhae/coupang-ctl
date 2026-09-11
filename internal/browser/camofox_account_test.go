package browser

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestCamofoxAccountRunsBundledReaderUnderOneOperation(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for bundled reader")
	}
	calls := 0
	c := &Camofox{run: func(ctx context.Context, script, mode string, _ core.QRLinkPresenter) ([]byte, error) {
		calls++
		if mode != "account" {
			t.Fatal("account read used another operation")
		}
		const harness = `const vm=require('node:vm');
const member={loyaltyMemberInfo:{membershipStatus:'ACTIVE',notMember:false,trialMember:false,membershipOnHold:false,currentFee:0},wowBenefitUsage:{totalAmount:0},paymentMethod:{paymentMethodDTO:{payMethodName:'Synthetic card',payMethodAccount:'synthetic-private-account'}}};
const reward={cashableAmount:{currencyCode:'KRW',amount:100},nonCashableAmount:{currencyCode:'KRW',amount:0},displayMessage:'WOW card',description:'synthetic-private-description',createdAt:'2026-09-11T00:00:00Z'};
let opened=0,closed=0,active=false,requests=0,result;
const openTab=async url=>{
 if(active)throw Error('overlapping tabs');active=true;opened++;
 const context=vm.createContext({URL,TextDecoder,Uint8Array,AbortController,setTimeout,clearTimeout,location:{href:url},performance:{getEntriesByType:()=>[]},document:{title:'Synthetic 최근 3개월',readyState:'complete',body:{innerText:''},querySelector:()=>null,getElementById:()=>({textContent:JSON.stringify({props:{pageProps:{data:member}}})})},fetch:async(url,opts)=>{requests++;if(opts.method!=='GET')throw Error('write');let data=url.includes('transactions')?{content:{currentPageNumber:1,nextPageExist:true,list:[reward]}}:{content:{expectedWowCardAccumulationAmount:{currency:'KRW',amount:0}}};return new Response(JSON.stringify(data),{headers:{'content-type':'application/json'}});}});
 return {evaluate:async expression=>vm.runInContext(expression,context),close:async()=>{active=false;closed++;}};
};
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
new AsyncFunction('openTab','console',process.argv[1])(openTab,{log:line=>result=line}).then(()=>{
 if(opened!==2||closed!==2||requests!==2||result.includes('synthetic-private'))throw Error('privacy or lifecycle failure');
 process.stdout.write(result.slice('COUPANGCTL_RESULT '.length));
}).catch(()=>process.exitCode=1);`
		return exec.CommandContext(ctx, node, "-e", harness, script).Output()
	}}
	got, err := c.Snapshot(context.Background(), core.AccountBenefitsRequest{MaxCashTransactionPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !got.Coverage.CurrentMembershipFeeObserved || !got.Coverage.BenefitUsageObserved || got.Coverage.CashTransactionStatus != "partial" || got.CardRewards.ObservedTransactionCount != 1 || got.CardRewards.ObservedAccumulationKRW != 100 {
		t.Fatal("account evidence or request budget lost")
	}
	if got.Membership.IsPaidMember != nil || got.Membership.IsMember == nil || !*got.Membership.IsMember {
		t.Fatal("bundled page reader lost the independent availability of membership flags")
	}
}

func TestCamofoxAccountValidatesBeforeAcquisitionAndPreservesFailures(t *testing.T) {
	c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
		t.Fatal("invalid request acquired browser")
		return nil, nil
	}}
	for _, limit := range []int{-1, 101} {
		if _, err := c.Snapshot(context.Background(), core.AccountBenefitsRequest{MaxCashTransactionPages: limit}); err == nil {
			t.Fatal("invalid page budget accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Snapshot(ctx, core.AccountBenefitsRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled account read acquired browser")
	}
	for _, tc := range []struct {
		wire string
		want error
	}{
		{`{"status":"authentication_required"}`, core.ErrAuthenticationRequired},
		{`{"status":"access_denied"}`, core.ErrBrowserAccessDenied},
		{`{"status":"account_data_missing"}`, ErrStructuredAccountBenefitsDataMissing},
		{`{"status":"ok","document":{}}`, ErrStructuredAccountBenefitsDataMissing},
		{`{"status":"ok","inspection":{}}`, ErrDocumentProtocol},
	} {
		c.run = func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
			return []byte(tc.wire), nil
		}
		if _, err := c.Snapshot(context.Background(), core.AccountBenefitsRequest{MaxCashTransactionPages: 1}); !errors.Is(err, tc.want) {
			t.Fatal("account failure became evidence")
		}
	}
}
