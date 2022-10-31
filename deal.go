package main

import (
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil/cidenc"
	"github.com/multiformats/go-multibase"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"strconv"
)

// GetCidEncoder returns an encoder using the `cid-base` flag if provided, or
// the default (Base32) encoder if not.
func GetCidEncoder(cctx *cli.Context) (cidenc.Encoder, error) {
	val := cctx.String("cid-base")

	e := cidenc.Encoder{Base: multibase.MustNewEncoder(multibase.Base32)}

	if val != "" {
		var err error
		e.Base, err = multibase.EncoderByName(val)
		if err != nil {
			return e, err
		}
	}

	return e, nil
}

var dealCmd = &cli.Command{
	Name:  "deal",
	Usage: " 订单工具",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		clientDealCmd,
	},
}

var clientDealCmd = &cli.Command{
	Name:  "send",
	Usage: "Initialize storage deal with a miner",
	Description: `Make a deal with a miner.
dataCid comes from running 'lotus client import'.
miner is the address of the miner you wish to make a deal with.
price is measured in FIL/Epoch. Miners usually don't accept a bid
lower than their advertised ask (which is in FIL/GiB/Epoch). You can check a miners listed price
with 'lotus client query-ask <miner address>'.
duration is how long the miner should store the data for, in blocks.
The minimum value is 518400 (6 months).`,
	ArgsUsage: "[dataCid miner price duration]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "manual-piece-cid",
			Usage: "manually specify piece commitment for data (dataCid must be to a car file)",
		},
		&cli.Int64Flag{
			Name:  "manual-piece-size",
			Usage: "if manually specifying piece cid, used to specify size (dataCid must be to a car file)",
		},
		&cli.BoolFlag{
			Name:  "manual-stateless-deal",
			Usage: "instructs the node to send an offline deal without registering it with the deallist/fsm",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "specify address to fund the deal with",
		},
		&cli.Int64Flag{
			Name:  "start-epoch",
			Usage: "specify the epoch that the deal should start at",
			Value: -1,
		},
		&cli.BoolFlag{
			Name:  "fast-retrieval",
			Usage: "indicates that data should be available for fast retrieval",
			Value: true,
		},
		&cli.BoolFlag{
			Name:        "verified-deal",
			Usage:       "indicate that the deal counts towards verified client total",
			DefaultText: "true if client is verified, false otherwise",
		},
		&cli.StringFlag{
			Name:  "provider-collateral",
			Usage: "specify the requested provider collateral the miner should put up",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {

		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		expectedArgsMsg := "expected 4 args: dataCid, miner, price, duration"

		if !cctx.Args().Present() {
			if cctx.Bool("manual-stateless-deal") {
				return xerrors.New("--manual-stateless-deal can not be combined with interactive deal mode: you must specify the " + expectedArgsMsg)
			}
			return nil
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		afmt := NewAppFmt(cctx.App)

		if cctx.NArg() != 4 {
			return xerrors.New(expectedArgsMsg)
		}

		// [data, miner, price, dur]

		data, err := cid.Parse(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		miner, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		price, err := types.ParseFIL(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		dur, err := strconv.ParseInt(cctx.Args().Get(3), 10, 32)
		if err != nil {
			return err
		}

		var provCol big.Int
		if pcs := cctx.String("provider-collateral"); pcs != "" {
			pc, err := big.FromString(pcs)
			if err != nil {
				return fmt.Errorf("failed to parse provider-collateral: %w", err)
			}
			provCol = pc
		}

		if abi.ChainEpoch(dur) < build.MinDealDuration {
			return xerrors.Errorf("minimum deal duration is %d blocks", build.MinDealDuration)
		}
		if abi.ChainEpoch(dur) > build.MaxDealDuration {
			return xerrors.Errorf("maximum deal duration is %d blocks", build.MaxDealDuration)
		}

		var a address.Address
		if from := cctx.String("from"); from != "" {
			faddr, err := address.NewFromString(from)
			if err != nil {
				return xerrors.Errorf("failed to parse 'from' address: %w", err)
			}
			a = faddr
		} else {
			return xerrors.Errorf("must set 'from' !")
		}

		ref := &storagemarket.DataRef{
			TransferType: storagemarket.TTGraphsync,
			Root:         data,
		}

		if mpc := cctx.String("manual-piece-cid"); mpc != "" {
			c, err := cid.Parse(mpc)
			if err != nil {
				return xerrors.Errorf("failed to parse provided manual piece cid: %w", err)
			}

			ref.PieceCid = &c

			psize := cctx.Int64("manual-piece-size")
			if psize == 0 {
				return xerrors.Errorf("must specify piece size when manually setting cid")
			}

			ref.PieceSize = abi.UnpaddedPieceSize(psize)

			ref.TransferType = storagemarket.TTManual
		}

		// Check if the address is a verified client
		dcap, err := api.StateVerifiedClientStatus(ctx, a, types.EmptyTSK)
		if err != nil {
			return err
		}

		isVerified := dcap != nil

		// If the user has explicitly set the --verified-deal flag
		if cctx.IsSet("verified-deal") {
			// If --verified-deal is true, but the address is not a verified
			// client, return an error
			verifiedDealParam := cctx.Bool("verified-deal")
			if verifiedDealParam && !isVerified {
				return xerrors.Errorf("address %s does not have verified client status", a)
			}

			// Override the default
			isVerified = verifiedDealParam
		}

		sdParams := &lapi.StartDealParams{
			Data:               ref,
			Wallet:             a,
			Miner:              miner,
			EpochPrice:         types.BigInt(price),
			MinBlocksDuration:  uint64(dur),
			DealStartEpoch:     abi.ChainEpoch(cctx.Int64("start-epoch")),
			FastRetrieval:      cctx.Bool("fast-retrieval"),
			VerifiedDeal:       isVerified,
			ProviderCollateral: provCol,
		}

		var proposal *cid.Cid
		if cctx.Bool("manual-stateless-deal") {
			if ref.TransferType != storagemarket.TTManual || price.Int64() != 0 {
				return xerrors.New("when manual-stateless-deal is enabled, you must also provide a 'price' of 0 and specify 'manual-piece-cid' and 'manual-piece-size'")
			}
			proposal, err = api.ClientStatelessDeal(ctx, sdParams)
		} else {
			proposal, err = api.ClientStartDeal(ctx, sdParams)
		}

		if err != nil {
			return err
		}

		encoder, err := GetCidEncoder(cctx)
		if err != nil {
			return err
		}

		afmt.Println(encoder.Encode(*proposal))

		return nil
	},
}
