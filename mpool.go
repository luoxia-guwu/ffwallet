package main

import (
	"fmt"
	"strconv"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var mpoolCmd = &cli.Command{
	Name:  "mpool",
	Usage: "manage msg pool",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		msgReplaceCmd,
	},
}

var msgReplaceCmd = &cli.Command{
	Name:  "replace",
	Usage: "replace a message in the mempool",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "gas-feecap",
			Usage: "gas feecap for new message (burn and pay to miner, attoFIL/GasUnit)",
		},
		&cli.StringFlag{
			Name:  "gas-premium",
			Usage: "gas price for new message (pay to miner, attoFIL/GasUnit)",
		},
		&cli.Int64Flag{
			Name:  "gas-limit",
			Usage: "gas limit for new message (GasUnit)",
		},
		&cli.BoolFlag{
			Name:  "auto",
			Usage: "automatically reprice the specified message",
		},
		&cli.StringFlag{
			Name:  "fee-limit",
			Usage: "Spend up to X FIL for this message in units of FIL. Previously when flag was `max-fee` units were in attoFIL. Applicable for auto mode",
		},
	},
	ArgsUsage: "<from> <nonce> | <message-cid>",
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		var from address.Address
		var nonce uint64
		switch cctx.NArg() {
		case 1:
			mcid, err := cid.Decode(cctx.Args().First())
			if err != nil {
				return err
			}

			msg, err := api.ChainGetMessage(ctx, mcid)
			if err != nil {
				return xerrors.Errorf("could not find referenced message: %w", err)
			}

			from = msg.From
			nonce = msg.Nonce
		case 2:
			arg0 := cctx.Args().Get(0)
			f, err := address.NewFromString(arg0)
			if err != nil {
				return err
			}

			n, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
			if err != nil {
				return err
			}

			from = f
			nonce = n
		default:
			return cli.ShowCommandHelp(cctx, cctx.Command.Name)
		}

		ts, err := api.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}

		pending, err := api.MpoolPending(ctx, ts.Key())
		if err != nil {
			return err
		}

		var found *types.SignedMessage
		for _, p := range pending {
			if p.Message.From == from && p.Message.Nonce == nonce {
				found = p
				break
			}
		}

		if found == nil {
			return xerrors.Errorf("no pending message found from %s with nonce %d", from, nonce)
		}

		msg := found.Message

		if cctx.Bool("auto") {
			cfg, err := api.MpoolGetConfig(ctx)
			if err != nil {
				return xerrors.Errorf("failed to lookup the message pool config: %w", err)
			}

			defaultRBF := messagepool.ComputeRBF(msg.GasPremium, cfg.ReplaceByFeeRatio)

			var mss *lapi.MessageSendSpec
			if cctx.IsSet("fee-limit") {
				maxFee, err := types.ParseFIL(cctx.String("fee-limit"))
				if err != nil {
					return xerrors.Errorf("parsing max-spend: %w", err)
				}
				mss = &lapi.MessageSendSpec{
					MaxFee: abi.TokenAmount(maxFee),
				}
			}

			// msg.GasLimit = 0 // TODO: need to fix the way we estimate gas limits to account for the messages already being in the mempool
			msg.GasFeeCap = abi.NewTokenAmount(0)
			msg.GasPremium = abi.NewTokenAmount(0)
			retm, err := api.GasEstimateMessageGas(ctx, &msg, mss, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("failed to estimate gas values: %w", err)
			}

			msg.GasPremium = big.Max(retm.GasPremium, defaultRBF)
			msg.GasFeeCap = big.Max(retm.GasFeeCap, msg.GasPremium)

			mff := func() (abi.TokenAmount, error) {
				return abi.TokenAmount(config.DefaultDefaultMaxFee()), nil
			}

			messagepool.CapGasFee(mff, &msg, mss)
		} else {
			if cctx.IsSet("gas-limit") {
				msg.GasLimit = cctx.Int64("gas-limit")
			}
			msg.GasPremium, err = types.BigFromString(cctx.String("gas-premium"))
			if err != nil {
				return xerrors.Errorf("parsing gas-premium: %w", err)
			}
			// TODO: estimate fee cap here
			msg.GasFeeCap, err = types.BigFromString(cctx.String("gas-feecap"))
			if err != nil {
				return xerrors.Errorf("parsing gas-feecap: %w", err)
			}
		}

		mb, err := msg.ToStorageBlock()
		if err != nil {
			fmt.Printf("序列化消息失败， err:%v", err)
			return xerrors.Errorf("serializing message: %w", err)
		}

		// 签名
		sb, err := signMessage(mb.Cid().Bytes(), msg.From)
		if err != nil {
			fmt.Printf("签名失败， err:%v\n", err)
			return xerrors.Errorf("签名失败: %w", err)
		}

		// 推送消息
		cid, err := api.MpoolPush(ctx, &types.SignedMessage{Message: msg, Signature: *sb})
		if err != nil {
			fmt.Printf("推送消息上链失败，err:%v\n", err)
			return err
		}

		fmt.Println("new message cid: ", cid)
		return nil
	},
}
