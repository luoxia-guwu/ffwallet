package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	init2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	msig2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/v6/actors/util/adt"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

type MessagePrototype struct {
	Message    types.Message
	ValidNonce bool
}

type PrintHelpErr struct {
	Err error
	Ctx *cli.Context
}

func (e *PrintHelpErr) Error() string {
	return e.Err.Error()
}

func (e *PrintHelpErr) Unwrap() error {
	return e.Err
}

func (e *PrintHelpErr) Is(o error) bool {
	_, ok := o.(*PrintHelpErr)
	return ok
}

func ShowHelp(cctx *cli.Context, err error) error {
	return &PrintHelpErr{Err: err, Ctx: cctx}
}

var msigCmd = &cli.Command{
	Name:  "msig",
	Usage: "多签工具",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		msigCreateCmd,
		msigInspectCmd,
		msigProposeCmd,
		msigProposeChangeOwnerCmd,
		msigProposeChangeWorkerCmd,
		msigProposeControlSetCmd,
		msigProposeWithdrawCmd,
		msigRemoveProposeCmd,
		msigApproveCmd,
		msigAddProposeCmd,
		msigAddApproveCmd,
		msigAddCancelCmd,
		msigVestedCmd,
		msigSwapApproveCmd,
		msigSwapProposeCmd,
	},
}

var msigCreateCmd = &cli.Command{
	Name:      "create",
	Usage:     "Create a new multisig wallet",
	ArgsUsage: "[address1 address2 ...]",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "required",
			Usage: "指定签名通过需要的票数",
		},
		&cli.StringFlag{
			Name:  "value",
			Usage: "initial funds to give to multisig",
			Value: "0",
		},
		&cli.StringFlag{
			Name:  "duration",
			Usage: "length of the period over which funds unlock",
			Value: "0",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the create message from",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 1 {
			return ShowHelp(cctx, fmt.Errorf("multisigs must have at least one signer"))
		}

		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		api, acloser, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			fmt.Println(err)
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		var addrs []address.Address
		for _, a := range cctx.Args().Slice() {
			addr, err := address.NewFromString(a)
			if err != nil {
				fmt.Println(err)
				return err
			}
			addrs = append(addrs, addr)
		}

		// get the address we're going to use to create the multisig (can be one of the above, as long as they have funds)
		var sendAddr address.Address
		if send := cctx.String("from"); send == "" {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				fmt.Println(err)
				return err
			}

			sendAddr = defaddr
		} else {
			addr, err := address.NewFromString(send)
			if err != nil {
				fmt.Println(err)
				return err
			}

			sendAddr = addr
		}

		val := cctx.String("value")
		filval, err := types.ParseFIL(val)
		if err != nil {
			fmt.Println(err)
			return err
		}

		intVal := types.BigInt(filval)

		required := cctx.Uint64("required")
		if required == 0 {
			required = uint64(len(addrs))
		}

		d := abi.ChainEpoch(cctx.Uint64("duration"))

		gp := types.NewInt(1)

		proto, err := api.MsigCreate(ctx, required, addrs, d, intVal, sendAddr, gp)
		if err != nil {
			fmt.Println(err)
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			fmt.Println(err)
			return err
		}

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			fmt.Println(err)
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Fprintln(cctx.App.Writer, "actor creation failed!")
			return err
		}

		// get address of newly created miner

		var execreturn init2.ExecReturn
		if err := execreturn.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
			return err
		}
		fmt.Fprintln(cctx.App.Writer, "Created new multisig: ", execreturn.IDAddress, execreturn.RobustAddress)

		// TODO: maybe register this somewhere
		return nil
	},
}

var msigProposeCmd = &cli.Command{
	Name:      "propose",
	Usage:     "Propose a multisig transaction",
	ArgsUsage: "[multisigAddress destinationAddress value <methodId methodParams> (optional)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 3 {
			return ShowHelp(cctx, fmt.Errorf("must pass at least multisig address, destination, and value"))
		}

		if cctx.Args().Len() > 3 && cctx.Args().Len() != 5 {
			return ShowHelp(cctx, fmt.Errorf("must either pass three or five arguments"))
		}
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		dest, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		value, err := types.ParseFIL(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		var method uint64
		var params []byte
		if cctx.Args().Len() == 5 {
			m, err := strconv.ParseUint(cctx.Args().Get(3), 10, 64)
			if err != nil {
				return err
			}
			method = m

			p, err := hex.DecodeString(cctx.Args().Get(4))
			if err != nil {
				return err
			}
			params = p
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		act, err := api.StateGetActor(ctx, msig, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("failed to look up multisig %s: %w", msig, err)
		}

		if !builtin.IsMultisigActor(act.Code) {
			return fmt.Errorf("actor %s is not a multisig actor", msig)
		}

		proto, err := api.MsigPropose(ctx, msig, dest, types.BigInt(value), from, method, params)
		if err != nil {
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			return err
		}

		fmt.Println("send proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("proposal returned exit %d", wait.Receipt.ExitCode)
		}

		var retval msig2.ProposeReturn
		if err := retval.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
			return fmt.Errorf("failed to unmarshal propose return value: %w", err)
		}

		fmt.Printf("Transaction ID: %d\n", retval.TxnID)
		if retval.Applied {
			fmt.Printf("Transaction was executed during propose\n")
			fmt.Printf("Exit Code: %d\n", retval.Code)
			fmt.Printf("Return Value: %x\n", retval.Ret)
		}

		return nil
	},
}

var msigInspectCmd = &cli.Command{
	Name:      "inspect",
	Usage:     "Inspect a multisig wallet",
	ArgsUsage: "[address]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "vesting",
			Usage: "Include vesting details",
		},
		&cli.BoolFlag{
			Name:  "decode-params",
			Usage: "Decode parameters of transaction proposals",
		},
	},
	//Before: func(context *cli.Context) error {
	//	if err := _init(); err != nil {
	//		passwdValid = false
	//	}
	//	return nil
	//},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return ShowHelp(cctx, fmt.Errorf("must specify address of multisig to inspect"))
		}

		//if !passwdValid {
		//	fmt.Println("密码错误.")
		//	return fmt.Errorf("密码错误")
		//}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(api)))

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		act, err := api.StateGetActor(ctx, maddr, head.Key())
		if err != nil {
			return err
		}

		ownId, err := api.StateLookupID(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		mstate, err := multisig.Load(store, act)
		if err != nil {
			return err
		}
		locked, err := mstate.LockedBalance(head.Height())
		if err != nil {
			return err
		}

		fmt.Fprintf(cctx.App.Writer, "Balance: %s\n", types.FIL(act.Balance))
		fmt.Fprintf(cctx.App.Writer, "Spendable: %s\n", types.FIL(types.BigSub(act.Balance, locked)))

		if cctx.Bool("vesting") {
			ib, err := mstate.InitialBalance()
			if err != nil {
				return err
			}
			fmt.Fprintf(cctx.App.Writer, "InitialBalance: %s\n", types.FIL(ib))
			se, err := mstate.StartEpoch()
			if err != nil {
				return err
			}
			fmt.Fprintf(cctx.App.Writer, "StartEpoch: %d\n", se)
			ud, err := mstate.UnlockDuration()
			if err != nil {
				return err
			}
			fmt.Fprintf(cctx.App.Writer, "UnlockDuration: %d\n", ud)
		}

		signers, err := mstate.Signers()
		if err != nil {
			return err
		}
		threshold, err := mstate.Threshold()
		if err != nil {
			return err
		}
		fmt.Fprintf(cctx.App.Writer, "Threshold: %d / %d\n", threshold, len(signers))
		fmt.Fprintln(cctx.App.Writer, "Signers:")

		signerTable := tabwriter.NewWriter(cctx.App.Writer, 8, 4, 2, ' ', 0)
		fmt.Fprintf(signerTable, "ID\tAddress\n")
		for _, s := range signers {
			signerActor, err := api.StateAccountKey(ctx, s, types.EmptyTSK)
			if err != nil {
				fmt.Fprintf(signerTable, "%s\t%s\n", s, "N/A")
			} else {
				fmt.Fprintf(signerTable, "%s\t%s\n", s, signerActor)
			}
		}
		if err := signerTable.Flush(); err != nil {
			return xerrors.Errorf("flushing output: %+v", err)
		}

		pending := make(map[int64]multisig.Transaction)
		if err := mstate.ForEachPendingTxn(func(id int64, txn multisig.Transaction) error {
			pending[id] = txn
			return nil
		}); err != nil {
			return xerrors.Errorf("reading pending transactions: %w", err)
		}

		decParams := cctx.Bool("decode-params")
		fmt.Fprintln(cctx.App.Writer, "Transactions: ", len(pending))
		if len(pending) > 0 {
			var txids []int64
			for txid := range pending {
				txids = append(txids, txid)
			}
			sort.Slice(txids, func(i, j int) bool {
				return txids[i] < txids[j]
			})

			w := tabwriter.NewWriter(cctx.App.Writer, 8, 4, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tState\tApprovals\tTo\tValue\tMethod\tParams\n")
			for _, txid := range txids {
				tx := pending[txid]
				target := tx.To.String()
				if tx.To == ownId {
					target += " (self)"
				}
				targAct, err := api.StateGetActor(ctx, tx.To, types.EmptyTSK)
				paramStr := fmt.Sprintf("%x", tx.Params)

				if err != nil {
					if tx.Method == 0 {
						fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s(%d)\t%s\n", txid, "pending", len(tx.Approved), target, types.FIL(tx.Value), "Send", tx.Method, paramStr)
					} else {
						fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s(%d)\t%s\n", txid, "pending", len(tx.Approved), target, types.FIL(tx.Value), "new account, unknown method", tx.Method, paramStr)
					}
				} else {
					method := filcns.NewActorRegistry().Methods[targAct.Code][tx.Method] // TODO: use remote map

					if decParams && tx.Method != 0 {
						ptyp := reflect.New(method.Params.Elem()).Interface().(cbg.CBORUnmarshaler)
						if err := ptyp.UnmarshalCBOR(bytes.NewReader(tx.Params)); err != nil {
							return xerrors.Errorf("failed to decode parameters of transaction %d: %w", txid, err)
						}

						b, err := json.Marshal(ptyp)
						if err != nil {
							return xerrors.Errorf("could not json marshal parameter type: %w", err)
						}

						paramStr = string(b)
						if tx.Method == 16 {
							attoFil := strings.Split(paramStr, ":")[1]
							//fmt.Println("atto fil: ", attoFil)
							fil := transFilToFIL(strings.Trim(attoFil, "\"}"))
							//fmt.Println("fil: ", fil)
							paramStr = strings.Split(paramStr, ":")[0] + ":" + "\"" + fil + "\"" + "}"
						}
					}

					fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s(%d)\t%s\n", txid, "pending", len(tx.Approved), target, types.FIL(tx.Value), method.Name, tx.Method, paramStr)
				}
			}
			if err := w.Flush(); err != nil {
				return xerrors.Errorf("flushing output: %+v", err)
			}

		}

		return nil
	},
}

var msigProposeChangeOwnerCmd = &cli.Command{
	Name:      "propose-change-owner",
	Usage:     "Propose change a miner owner address",
	ArgsUsage: "[multisigAddress  miner ]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		//if cctx.Args().Len() < 3 {
		//	return ShowHelp(cctx, fmt.Errorf("must pass at least multisig address, destination, and value"))
		//}
		//
		//if cctx.Args().Len() > 3 && cctx.Args().Len() != 5 {
		//	return ShowHelp(cctx, fmt.Errorf("must either pass three or five arguments"))
		//}
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		maddr, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		newAddrId, err := api.StateLookupID(ctx, msig, types.EmptyTSK)
		if err != nil {
			return err
		}

		sp, err := actors.SerializeParams(&newAddrId)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		act, err := api.StateGetActor(ctx, msig, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("failed to look up multisig %s: %w", msig, err)
		}

		if !builtin.IsMultisigActor(act.Code) {
			return fmt.Errorf("actor %s is not a multisig actor", msig)
		}

		proto, err := api.MsigPropose(ctx, msig, maddr, big.Zero(), from, uint64(miner.Methods.ChangeOwnerAddress), sp)
		if err != nil {
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			return err
		}

		fmt.Println("send proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("proposal returned exit %d", wait.Receipt.ExitCode)
		}

		//var retval msig2.ProposeReturn
		//if err := retval.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
		//	return fmt.Errorf("failed to unmarshal propose return value: %w", err)
		//}
		//
		//fmt.Printf("Transaction ID: %d\n", retval.TxnID)
		//if retval.Applied {
		//	fmt.Printf("Transaction was executed during propose\n")
		//	fmt.Printf("Exit Code: %d\n", retval.Code)
		//	fmt.Printf("Return Value: %x\n", retval.Ret)
		//}

		return nil
	},
}

var msigProposeChangeWorkerCmd = &cli.Command{
	Name:      "propose-change-worker",
	Usage:     "Propose change a miner worker address",
	ArgsUsage: "[miner worker]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
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

		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			fmt.Println(err)
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		//msig, err := address.NewFromString(cctx.Args().Get(0))
		//if err != nil {
		//	return err
		//}

		maddr, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			fmt.Println(err)
			return err
		}

		worker, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			fmt.Println(err)
			return err
		}

		newAddr, err := api.StateLookupID(ctx, worker, types.EmptyTSK)
		if err != nil {
			fmt.Println(err)
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			fmt.Println(err)
			return err
		}

		if mi.NewWorker.Empty() {
			if mi.Worker == newAddr{
				fmt.Println("worker address already set to ", worker)
				return fmt.Errorf("worker address already set to %s", worker)
			}
		} else {
			if mi.NewWorker == newAddr {
				fmt.Printf("change to worker address %s already pending\n", worker)
				return fmt.Errorf("change to worker address %s already pending", worker)
			}
		}

		cwp := &miner2.ChangeWorkerAddressParams{
			NewWorker:       newAddr,
			NewControlAddrs: mi.ControlAddresses,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			fmt.Println(xerrors.Errorf("serializing params: %w", err))
			return xerrors.Errorf("serializing params: %w", err)
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				fmt.Println(err)
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				fmt.Println(err)
				return err
			}
			from = defaddr
		}

		act, err := api.StateGetActor(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			fmt.Printf("failed to look up multisig %s: %w", mi.Owner, err)
			return fmt.Errorf("failed to look up multisig %s: %w", mi.Owner, err)
		}

		if !builtin.IsMultisigActor(act.Code) {
			fmt.Printf("actor %s is not a multisig actor", mi.Owner)
			return fmt.Errorf("actor %s is not a multisig actor", mi.Owner)
		}

		proto, err := api.MsigPropose(ctx, mi.Owner, maddr, big.Zero(), from, uint64(miner.Methods.ChangeWorkerAddress), sp)
		if err != nil {
			fmt.Println(err)
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			fmt.Println(err)
			return err
		}

		fmt.Println("send proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			fmt.Println(err)
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			fmt.Printf("proposal returned exit %d", wait.Receipt.ExitCode)
			return fmt.Errorf("proposal returned exit %d", wait.Receipt.ExitCode)
		}
		return nil
	},
}

var msigProposeControlSetCmd = &cli.Command{
	Name:      "propose-control-set",
	Usage:     "Propose a miner set control address",
	ArgsUsage: "[miner ...worker]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
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
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		maddr, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		del := map[address.Address]struct{}{}
		existing := map[address.Address]struct{}{}
		for _, controlAddress := range mi.ControlAddresses {
			ka, err := api.StateAccountKey(ctx, controlAddress, types.EmptyTSK)
			if err != nil {
				return err
			}

			del[ka] = struct{}{}
			existing[ka] = struct{}{}
		}

		var toSet []address.Address

		for i, as := range cctx.Args().Slice() {
			if i == 0 {
				continue
			}
			a, err := address.NewFromString(as)
			if err != nil {
				return xerrors.Errorf("parsing address %d: %w", i, err)
			}

			ka, err := api.StateAccountKey(ctx, a, types.EmptyTSK)
			if err != nil {
				return err
			}

			// make sure the address exists on chain
			_, err = api.StateLookupID(ctx, ka, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("looking up %s: %w", ka, err)
			}

			delete(del, ka)
			toSet = append(toSet, ka)
		}
		for a := range del {
			fmt.Println("Remove", a)
		}
		for _, a := range toSet {
			if _, exists := existing[a]; !exists {
				fmt.Println("Add", a)
			}
		}

		cwp := &miner2.ChangeWorkerAddressParams{
			NewWorker:       mi.Worker,
			NewControlAddrs: toSet,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		act, err := api.StateGetActor(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("failed to look up multisig %s: %w", mi.Owner, err)
		}

		if !builtin.IsMultisigActor(act.Code) {
			return fmt.Errorf("actor %s is not a multisig actor", mi.Owner)
		}

		proto, err := api.MsigPropose(ctx, mi.Owner, maddr, big.Zero(), from, uint64(miner.Methods.ChangeWorkerAddress), sp)
		if err != nil {
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			return err
		}

		fmt.Println("Message CID:", msgCid)

		return nil
	},
}
var msigProposeWithdrawCmd = &cli.Command{
	Name:      "propose-withdraw",
	Usage:     "Propose a miner withdraw available balance",
	ArgsUsage: "[ miner amount]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
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
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		maddr, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		available, err := api.StateMinerAvailableBalance(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		amount := available

		f, err := types.ParseFIL(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}

		amount = abi.TokenAmount(f)

		if amount.GreaterThan(available) {
			return xerrors.Errorf("can't withdraw more funds than available; requested: %s; available: %s", amount, available)
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}
		params, err := actors.SerializeParams(&miner2.WithdrawBalanceParams{
			AmountRequested: amount, // Default to attempting to withdraw all the extra funds in the miner actor
		})
		if err != nil {
			return err
		}
		act, err := api.StateGetActor(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("failed to look up multisig %s: %w", mi.Owner, err)
		}

		if !builtin.IsMultisigActor(act.Code) {
			return fmt.Errorf("actor %s is not a multisig actor", mi.Owner)
		}

		proto, err := api.MsigPropose(ctx, mi.Owner, maddr, big.Zero(), from, uint64(miner.Methods.WithdrawBalance), params)
		if err != nil {
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			return err
		}

		fmt.Println("send proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("proposal returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigRemoveProposeCmd = &cli.Command{
	Name:      "propose-remove",
	Usage:     "Propose to remove a signer",
	ArgsUsage: "[multisigAddress signer]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "decrease-threshold",
			Usage: "whether the number of required signers should be decreased",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address and signer address"))
		}

		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		addr, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		proto, err := api.MsigRemoveSigner(ctx, msig, from, addr, cctx.Bool("decrease-threshold"))
		if err != nil {
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			return err
		}

		fmt.Println("sent remove proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("add proposal returned exit %d", wait.Receipt.ExitCode)
		}

		var ret multisig.ProposeReturn
		err = ret.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return))
		if err != nil {
			return xerrors.Errorf("decoding proposal return: %w", err)
		}
		fmt.Printf("TxnID: %d", ret.TxnID)

		return nil
	},
}
var msigApproveCmd = &cli.Command{
	Name:      "approve",
	Usage:     "Approve a multisig message",
	ArgsUsage: "<multisigAddress messageId> [proposerAddress destination value [methodId methodParams]]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 2 {
			return ShowHelp(cctx, fmt.Errorf("must pass at least multisig address and message ID"))
		}

		if cctx.Args().Len() > 2 && cctx.Args().Len() < 5 {
			return ShowHelp(cctx, fmt.Errorf("usage: msig approve <msig addr> <message ID> <proposer address> <desination> <value>"))
		}

		if cctx.Args().Len() > 5 && cctx.Args().Len() != 7 {
			return ShowHelp(cctx, fmt.Errorf("usage: msig approve <msig addr> <message ID> <proposer address> <desination> <value> [ <method> <params> ]"))
		}

		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		var msgCid cid.Cid
		if cctx.Args().Len() == 2 {
			proto, err := api.MsigApprove(ctx, msig, txid, from)
			if err != nil {
				return err
			}

			mcid, err := InteractiveSend(ctx, cctx, api, proto)
			if err != nil {
				return err
			}

			msgCid = mcid
		} else {
			proposer, err := address.NewFromString(cctx.Args().Get(2))
			if err != nil {
				return err
			}

			if proposer.Protocol() != address.ID {
				proposer, err = api.StateLookupID(ctx, proposer, types.EmptyTSK)
				if err != nil {
					return err
				}
			}

			dest, err := address.NewFromString(cctx.Args().Get(3))
			if err != nil {
				return err
			}

			value, err := types.ParseFIL(cctx.Args().Get(4))
			if err != nil {
				return err
			}

			var method uint64
			var params []byte
			if cctx.Args().Len() == 7 {
				m, err := strconv.ParseUint(cctx.Args().Get(5), 10, 64)
				if err != nil {
					return err
				}
				method = m

				p, err := hex.DecodeString(cctx.Args().Get(6))
				if err != nil {
					return err
				}
				params = p
			}

			proto, err := api.MsigApproveTxnHash(ctx, msig, txid, proposer, dest, types.BigInt(value), from, method, params)
			if err != nil {
				return err
			}

			smcid, err := InteractiveSend(ctx, cctx, api, proto)
			if err != nil {
				return err
			}

			msgCid = smcid
		}

		fmt.Println("sent approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("approve returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}
var msigAddProposeCmd = &cli.Command{
	Name:      "add-propose",
	Usage:     "Propose to add a signer",
	ArgsUsage: "[multisigAddress signer]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "increase-threshold",
			Usage: "whether the number of required signers should be increased",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address and signer address"))
		}
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		addr, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		proto, err := api.MsigAddPropose(ctx, msig, from, addr, cctx.Bool("increase-threshold"))
		if err != nil {
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			return err
		}

		fmt.Fprintln(cctx.App.Writer, "sent add proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("add proposal returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}
var msigAddApproveCmd = &cli.Command{
	Name:      "add-approve",
	Usage:     "Approve a message to add a signer",
	ArgsUsage: "[multisigAddress proposerAddress txId newAddress increaseThreshold]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 5 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, proposer address, transaction id, new signer address, whether to increase threshold"))
		}
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		prop, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return err
		}

		newAdd, err := address.NewFromString(cctx.Args().Get(3))
		if err != nil {
			return err
		}

		inc, err := strconv.ParseBool(cctx.Args().Get(4))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		proto, err := api.MsigAddApprove(ctx, msig, from, txid, prop, newAdd, inc)
		if err != nil {
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			return err
		}

		fmt.Println("sent add approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("add approval returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigAddCancelCmd = &cli.Command{
	Name:      "add-cancel",
	Usage:     "Cancel a message to add a signer",
	ArgsUsage: "[multisigAddress txId newAddress increaseThreshold]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 4 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, transaction id, new signer address, whether to increase threshold"))
		}
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		newAdd, err := address.NewFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		inc, err := strconv.ParseBool(cctx.Args().Get(3))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		proto, err := api.MsigAddCancel(ctx, msig, from, txid, newAdd, inc)
		if err != nil {
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			return err
		}

		fmt.Println("sent add cancellation in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("add cancellation returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}
var msigVestedCmd = &cli.Command{
	Name:      "vested",
	Usage:     "Gets the amount vested in an msig between two epochs",
	ArgsUsage: "[multisigAddress]",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "start-epoch",
			Usage: "start epoch to measure vesting from",
			Value: 0,
		},
		&cli.Int64Flag{
			Name:  "end-epoch",
			Usage: "end epoch to stop measure vesting at",
			Value: -1,
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address"))
		}
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		start, err := api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(cctx.Int64("start-epoch")), types.EmptyTSK)
		if err != nil {
			return err
		}

		var end *types.TipSet
		if cctx.Int64("end-epoch") < 0 {
			end, err = LoadTipSet(ctx, cctx, api)
			if err != nil {
				return err
			}
		} else {
			end, err = api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(cctx.Int64("end-epoch")), types.EmptyTSK)
			if err != nil {
				return err
			}
		}

		ret, err := api.MsigGetVested(ctx, msig, start.Key(), end.Key())
		if err != nil {
			return err
		}

		fmt.Printf("Vested: %s between %d and %d\n", types.FIL(ret), start.Height(), end.Height())

		return nil
	},
}

var msigSwapApproveCmd = &cli.Command{
	Name:      "swap-approve",
	Usage:     "Approve a message to swap signers",
	ArgsUsage: "[multisigAddress proposerAddress txId oldAddress newAddress]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 5 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, proposer address, transaction id, old signer address, new signer address"))
		}
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		prop, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return err
		}

		oldAdd, err := address.NewFromString(cctx.Args().Get(3))
		if err != nil {
			return err
		}

		newAdd, err := address.NewFromString(cctx.Args().Get(4))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		proto, err := api.MsigSwapApprove(ctx, msig, from, txid, prop, oldAdd, newAdd)
		if err != nil {
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			return err
		}

		fmt.Println("sent swap approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("swap approval returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigSwapProposeCmd = &cli.Command{
	Name:      "swap-propose",
	Usage:     "Propose to swap signers",
	ArgsUsage: "[multisigAddress oldAddress newAddress]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, old signer address, new signer address"))
		}
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		oldAdd, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		newAdd, err := address.NewFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		proto, err := api.MsigSwapPropose(ctx, msig, from, oldAdd, newAdd)
		if err != nil {
			return err
		}

		msgCid, err := InteractiveSend(ctx, cctx, api, proto)
		if err != nil {
			return err
		}

		fmt.Println("sent swap proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), build.Finality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("swap proposal returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

func InteractiveSend(ctx context.Context, cctx *cli.Context, api v1api.FullNode, proto *api.MessagePrototype) (cid.Cid, error) {
	// 获取nonce
	if !proto.ValidNonce {
		a, err := api.StateGetActor(ctx, proto.Message.From, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取获取owner地址的nonce失败，err:%v\n", err)
			return cid.Cid{}, err
		}
		proto.Message.Nonce = a.Nonce
	}

	msg, err := api.GasEstimateMessageGas(ctx, &proto.Message, nil, types.EmptyTSK)
	if err != nil {
		fmt.Printf("评估消息的gas费用失败， err:%v\n", err)
		return cid.Cid{}, xerrors.Errorf("GasEstimateMessageGas error: %w", err)
	}

	fmt.Printf("\n%+v\n", msg)

	mb, err := msg.ToStorageBlock()
	if err != nil {
		fmt.Printf("序列化消息失败， err:%v", err)
		return cid.Cid{}, xerrors.Errorf("serializing message: %w", err)
	}

	// 签名
	sb, err := signMessage(mb.Cid().Bytes(), msg.From)
	if err != nil {
		fmt.Printf("签名失败， err:%v\n", err)
		return cid.Cid{}, xerrors.Errorf("签名失败: %w", err)
	}

	// 推送消息
	msgCid, err := api.MpoolPush(ctx, &types.SignedMessage{Message: *msg, Signature: *sb})
	if err != nil {
		fmt.Printf("推送消息上链失败，err:%v\n", err)
		return cid.Cid{}, err
	}

	fmt.Println("Message CID:", msgCid.String())

	return msgCid, nil
}
