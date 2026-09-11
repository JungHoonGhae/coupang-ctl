package browser

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangaccount "github.com/JungHoonGhae/coupang-ctl/internal/coupang/account"
)

//go:embed account_page_reader.js
var accountPageReader string

// Snapshot satisfies the account module's existing typed Source interface.
// Both documents are read under one dedicated-profile lock and runtime.
func (c *Camofox) Snapshot(ctx context.Context, request core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return core.AccountBenefitsSnapshot{}, err
	}
	if err := request.Validate(); err != nil {
		return core.AccountBenefitsSnapshot{}, err
	}
	if request.MaxCashTransactionPages == 0 {
		request.MaxCashTransactionPages = 50
	}
	reader := strings.Replace(accountPageReader, "export async function", "async function", 1)
	membership, _ := json.Marshal("(" + reader + ")('membership'," + strconv.Itoa(request.MaxCashTransactionPages) + ")")
	cash, _ := json.Marshal("(" + reader + ")('cash'," + strconv.Itoa(request.MaxCashTransactionPages) + ")")
	script := `const read=async(url,expression)=>{const p=await openTab(url);try{let result;for(let i=0;i<30;i++){result=await p.evaluate(expression);if(result.status!=='loading')return result;await new Promise(r=>setTimeout(r,500));}return {status:'account_data_missing'};}finally{await p.close();}};
const membership=await read('https://loyalty.coupang.com/loyalty/management/home',` + string(membership) + `);
if(membership.status!=='ok'){console.log('COUPANGCTL_RESULT '+JSON.stringify({status:membership.status}));}
else {const cash=await read('https://cash.coupang.com/coupang-cash/home',` + string(cash) + `);
if(cash.status!=='ok'){console.log('COUPANGCTL_RESULT '+JSON.stringify({status:cash.status}));}
else console.log('COUPANGCTL_RESULT '+JSON.stringify({status:'ok',document:{membership:membership.data,cash_summary:cash.summary,cash_transaction_pages:cash.pages}}));}`
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	data, err := c.run(ctx, script, "account", nil)
	if err != nil {
		return core.AccountBenefitsSnapshot{}, err
	}
	var result struct {
		Status   string          `json:"status"`
		Document json.RawMessage `json:"document"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if len(data) > 768<<10 || decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return core.AccountBenefitsSnapshot{}, ErrDocumentProtocol
	}
	switch result.Status {
	case "authentication_required":
		return core.AccountBenefitsSnapshot{}, core.ErrAuthenticationRequired
	case "access_denied":
		return core.AccountBenefitsSnapshot{}, core.ErrBrowserAccessDenied
	case "ok":
		parsed, err := coupangaccount.ParseSnapshotDocument(result.Document)
		if err != nil || parsed.Coverage.CashTransactionPagesRead < 1 || parsed.Coverage.CashTransactionPagesRead > request.MaxCashTransactionPages {
			return core.AccountBenefitsSnapshot{}, ErrStructuredAccountBenefitsDataMissing
		}
		return parsed, nil
	default:
		return core.AccountBenefitsSnapshot{}, ErrStructuredAccountBenefitsDataMissing
	}
}
