package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v8/verifreg"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)
import lcli "github.com/filecoin-project/lotus/cli"

var filplusCmd = &cli.Command{
	Name:  "filplus",
	Usage: "Interact with the verified registry actor used by Filplus",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		filplusVerifyClientCmd,
		filplusListNotariesCmd,
		filplusListClientsCmd,
		filplusCheckClientCmd,
		filplusCheckNotaryCmd,
		//filplusSignRemoveDataCapProposal,
	},
}


var filplusVerifyClientCmd = &cli.Command{
	Name:  "grant-datacap",
	Usage: "give allowance to the specified verified client address",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "from",
			Usage:    "specify your notary address to send the message from",
			Required: true,
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		/**
		1 获取nonce，
		2 签名，使用本地签名
		3 使用fullnodeapi推送消息
		*/

		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		froms := cctx.String("from")
		if froms == "" {
			return fmt.Errorf("must specify from address with --from")
		}

		fromk, err := address.NewFromString(froms)
		if err != nil {
			return err
		}

		if cctx.Args().Len() != 2 {
			return fmt.Errorf("must specify two arguments: address and allowance")
		}

		target, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		allowance, err := types.BigFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		found, dcap, err := checkNotary(ctx, api, fromk)
		if err != nil {
			return err
		}

		if !found {
			return xerrors.New("sender address must be a notary")
		}

		if dcap.Cmp(allowance.Int) < 0 {
			return xerrors.Errorf("cannot allot more allowance than notary data cap: %s < %s", dcap, allowance)
		}

		// TODO: This should be abstracted over actor versions
		params, err := actors.SerializeParams(&verifregtypes.AddVerifiedClientParams{Address: target, Allowance: allowance})
		if err != nil {
			return err
		}

		// 获取nonce
		a, err := api.StateGetActor(ctx, fromk, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取获取owner地址的nonce失败，err:%v\n", err)
			return err
		}

		msg := &types.Message{
			To:     verifreg.Address,
			From:   fromk,
			Method: verifreg.Methods.AddVerifiedClient,
			Params: params,
			Nonce: a.Nonce,
		}

		fmt.Printf("\n%+v\n", msg)

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
		cid, err := api.MpoolPush(ctx, &types.SignedMessage{Message: *msg, Signature: *sb})
		if err != nil {
			fmt.Printf("推送消息上链失败，err:%v\n", err)
			return err
		}

		//smsg, err := api.MpoolPushMessage(ctx, msg, nil)
		//if err != nil {
		//	return err
		//}

		//fmt.Printf("message sent, now waiting on cid: %s\n", smsg.Cid())

		mwait, err := api.StateWaitMsg(ctx, cid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode != 0 {
			return fmt.Errorf("failed to add verified client: %d", mwait.Receipt.ExitCode)
		}

		return nil
	},
}

var filplusListNotariesCmd = &cli.Command{
	Name:  "list-notaries",
	Usage: "list all notaries",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		act, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
		if err != nil {
			return err
		}

		apibs := blockstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		st, err := verifreg.Load(store, act)
		if err != nil {
			return err
		}
		return st.ForEachVerifier(func(addr address.Address, dcap abi.StoragePower) error {
			_, err := fmt.Printf("%s: %s\n", addr, dcap)
			return err
		})
	},
}

var filplusListClientsCmd = &cli.Command{
	Name:  "list-clients",
	Usage: "list all verified clients",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		act, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
		if err != nil {
			return err
		}

		apibs := blockstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		st, err := verifreg.Load(store, act)
		if err != nil {
			return err
		}
		return st.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
			_, err := fmt.Printf("%s: %s\n", addr, dcap)
			return err
		})
	},
}

var filplusCheckClientCmd = &cli.Command{
	Name:  "check-client-datacap",
	Usage: "check verified client remaining bytes",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must specify client address to check")
		}

		caddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		dcap, err := api.StateVerifiedClientStatus(ctx, caddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		if dcap == nil {
			return xerrors.Errorf("client %s is not a verified client", caddr)
		}

		fmt.Println(*dcap)

		return nil
	},
}

var filplusCheckNotaryCmd = &cli.Command{
	Name:  "check-notary-datacap",
	Usage: "check a notary's remaining bytes",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must specify notary address to check")
		}

		vaddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		found, dcap, err := checkNotary(ctx, api, vaddr)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("not found")
		}

		fmt.Println(dcap)

		return nil
	},
}

func checkNotary(ctx context.Context, api v0api.FullNode, vaddr address.Address) (bool, abi.StoragePower, error) {
	vid, err := api.StateLookupID(ctx, vaddr, types.EmptyTSK)
	if err != nil {
		return false, big.Zero(), err
	}

	act, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
	if err != nil {
		return false, big.Zero(), err
	}

	apibs := blockstore.NewAPIBlockstore(api)
	store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

	st, err := verifreg.Load(store, act)
	if err != nil {
		return false, big.Zero(), err
	}

	return st.VerifierDataCap(vid)
}

var filplusSignRemoveDataCapProposal = &cli.Command{
	Name:  "sign-remove-data-cap-proposal",
	Usage: "allows a notary to sign a Remove Data Cap Proposal",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:     "id",
			Usage:    "specify the RemoveDataCapProposal ID (will look up on chain if unspecified)",
			Required: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return fmt.Errorf("must specify three arguments: notary address, client address, and allowance to remove")
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("failed to get full node api: %w", err)
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		act, err := api.StateGetActor(ctx, verifreg.Address, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("failed to get verifreg actor: %w", err)
		}

		apibs := blockstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

		st, err := verifreg.Load(store, act)
		if err != nil {
			return xerrors.Errorf("failed to load verified registry state: %w", err)
		}

		verifier, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}
		verifierIdAddr, err := api.StateLookupID(ctx, verifier, types.EmptyTSK)
		if err != nil {
			return err
		}

		client, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}
		clientIdAddr, err := api.StateLookupID(ctx, client, types.EmptyTSK)
		if err != nil {
			return err
		}

		allowanceToRemove, err := types.BigFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		_, dataCap, err := st.VerifiedClientDataCap(clientIdAddr)
		if err != nil {
			return xerrors.Errorf("failed to find verified client data cap: %w", err)
		}
		if dataCap.LessThanEqual(big.Zero()) {
			return xerrors.Errorf("client data cap %s is less than amount requested to be removed %s", dataCap.String(), allowanceToRemove.String())
		}

		found, _, err := checkNotary(ctx, api, verifier)
		if err != nil {
			return xerrors.Errorf("failed to check notary status: %w", err)
		}

		if !found {
			return xerrors.New("verifier address must be a notary")
		}

		id := cctx.Uint64("id")
		if id == 0 {
			_, id, err = st.RemoveDataCapProposalID(verifierIdAddr, clientIdAddr)
			if err != nil {
				return xerrors.Errorf("failed find remove data cap proposal id: %w", err)
			}
		}

		params := verifregtypes.RemoveDataCapProposal{
			RemovalProposalID: verifregtypes.RmDcProposalID{ProposalID: id},
			DataCapAmount:     allowanceToRemove,
			VerifiedClient:    clientIdAddr,
		}

		paramBuf := new(bytes.Buffer)
		paramBuf.WriteString(verifregtypes.SignatureDomainSeparation_RemoveDataCap)
		err = params.MarshalCBOR(paramBuf)
		if err != nil {
			return xerrors.Errorf("failed to marshall paramBuf: %w", err)
		}

		sig, err := api.WalletSign(ctx, verifier, paramBuf.Bytes())
		if err != nil {
			return xerrors.Errorf("failed to sign message: %w", err)
		}

		sigBytes := append([]byte{byte(sig.Type)}, sig.Data...)

		fmt.Println(hex.EncodeToString(sigBytes))

		return nil
	},
}
