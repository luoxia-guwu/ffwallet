package main

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/hako/durafmt"
	"github.com/ipfs/go-cid"
	"io"
	"os"
	"strings"
	"time"

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


func EpochTime(curr, e abi.ChainEpoch) string {
	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e))).LimitFirstN(2))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr))).LimitFirstN(2))
	}

	panic("math broke")
}

func EpochTimeHuman(curr, e abi.ChainEpoch) string {
	switch {
	case curr > e:
		return time.Now().Add(durafmt.Parse(time.Second * time.Duration(int64(build.BlockDelaySecs)*int64(e-curr))).Duration()).Format("2006-01-02 15:04")
		//return fmt.Sprintf("%d (%s ago)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e))).LimitFirstN(2))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return time.Now().Add(durafmt.Parse(time.Second * time.Duration(int64(build.BlockDelaySecs)*int64(e-curr))).Duration()).Format("2006-01-02 15:04")
		//return fmt.Sprintf("%d (in %s)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr))).LimitFirstN(2))
	}

	panic("math broke")
}

// 将sector数量转换成算力单位是GTP

func sectorsCountToGTP(sectorsCount uint64,pt abi.RegisteredSealProof) string {

	sectorSizeUint:=int64(32)
	if pt==abi.RegisteredSealProof_StackedDrg64GiBV1||pt==abi.RegisteredSealProof_StackedDrg64GiBV1_1{
		sectorSizeUint=64
	}

	power:= float64(sectorSizeUint * int64(sectorsCount))
	if power/1024.0<1{
		return fmt.Sprintf("%.2f G",power)
	}

	if power/1024.0/1024.0<1{
		return fmt.Sprintf("%.2f T",power/1024.0)
	}

	if power/1024.0/1024.0/1024.0<1{
		return fmt.Sprintf("%.2f P",power/1024.0/1024.0)
	}

	return ""
}