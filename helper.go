package main

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"io"
	"os"
	"strings"

	ufcli "github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

func RunApp(app *ufcli.App) {
	if err := app.Run(os.Args); err != nil {
		if os.Getenv("LOTUS_DEV") != "" {
			fmt.Printf("%+v", err)
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n\n", err) // nolint:errcheck
		}
		var phe *PrintHelpErr
		if xerrors.As(err, &phe) {
			_ = ufcli.ShowCommandHelp(phe.Ctx, phe.Ctx.Command.Name)
		}
		os.Exit(1)
	}
}

type AppFmt struct {
	app   *ufcli.App
	Stdin io.Reader
}

func NewAppFmt(a *ufcli.App) *AppFmt {
	var stdin io.Reader
	istdin, ok := a.Metadata["stdin"]
	if ok {
		stdin = istdin.(io.Reader)
	} else {
		stdin = os.Stdin
	}
	return &AppFmt{app: a, Stdin: stdin}
}

func (a *AppFmt) Print(args ...interface{}) {
	fmt.Fprint(a.app.Writer, args...)
}

func (a *AppFmt) Println(args ...interface{}) {
	fmt.Fprintln(a.app.Writer, args...)
}

func (a *AppFmt) Printf(fmtstr string, args ...interface{}) {
	fmt.Fprintf(a.app.Writer, fmtstr, args...)
}

func (a *AppFmt) Scan(args ...interface{}) (int, error) {
	return fmt.Fscan(a.Stdin, args...)
}

func transFilToFIL(amount string) (mount string) {
	//log.Logger.Debug("******** WalletSettlementData transFilToFIL amount :%+v", amount)
	//fmt.Println("amount: ", amount)
	if len(amount) <= 18 {
		if amount == "0" {
			mount = "0.0"
		} else {
			mount = intTo18BitDecimal(amount)
			mount = fmt.Sprintf("0.%s", mount)
			//log.Logger.Debug("******** WalletSettlementData transFilToFIL len<18 mount :%+v", mount)
		}
	} else {
		Fil := fmt.Sprintf("%s", amount[len(amount)-18:])
		//Fil = rTrimZero(Fil)
		mount = fmt.Sprintf("%s.%s", amount[0:len(amount)-18], Fil)
		//log.Logger.Debug("******** WalletSettlementData transFilToFIL len>18 mount :%+v", mount)
	}

	return
}

func intTo18BitDecimal(x string) string {
	lenght := len(x)
	head := ""
	for i := 0; i < 18-lenght; i++ {
		head = fmt.Sprintf("0%s", head)
	}
	y := fmt.Sprintf("%s%s", head, x)
	//res := rTrimZero(y)
	return y
}

func ParseTipSetString(ts string) ([]cid.Cid, error) {
	strs := strings.Split(ts, ",")

	var cids []cid.Cid
	for _, s := range strs {
		c, err := cid.Parse(strings.TrimSpace(s))
		if err != nil {
			return nil, err
		}
		cids = append(cids, c)
	}

	return cids, nil
}

// LoadTipSet gets the tipset from the context, or the head from the API.
//
// It always gets the head from the API so commands use a consistent tipset even if time pases.
func LoadTipSet(ctx context.Context, cctx *ufcli.Context, api v0api.FullNode) (*types.TipSet, error) {
	tss := cctx.String("tipset")
	if tss == "" {
		return api.ChainHead(ctx)
	}

	return ParseTipSetRef(ctx, api, tss)
}

func ParseTipSetRef(ctx context.Context, api v0api.FullNode, tss string) (*types.TipSet, error) {
	if tss[0] == '@' {
		if tss == "@head" {
			return api.ChainHead(ctx)
		}

		var h uint64
		if _, err := fmt.Sscanf(tss, "@%d", &h); err != nil {
			return nil, xerrors.Errorf("parsing height tipset ref: %w", err)
		}

		return api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(h), types.EmptyTSK)
	}

	cids, err := ParseTipSetString(tss)
	if err != nil {
		return nil, err
	}

	if len(cids) == 0 {
		return nil, nil
	}

	k := types.NewTipSetKey(cids...)
	ts, err := api.ChainGetTipSet(ctx, k)
	if err != nil {
		return nil, err
	}

	return ts, nil
}
