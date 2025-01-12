package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"path/filepath"

	"github.com/fatih/color"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	power2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/power"
	power6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/power"
	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var withdrawCmd = &cli.Command{
	Name:      "withdraw",
	Usage:     "矿工提现,例如 withdraw f02420 100, 如果不填写提现金额，则提取miner所有余额",
	ArgsUsage: "[minerId (eg f01000) ] [amount (FIL)]",
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

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Printf("连接FULLNODE_API_INFO api失败。%v\n", err)
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			fmt.Printf("输入miner ID(%s)不正确。 %v\n", cctx.Args().First(), err)
			return err
		}

		//  版本判断
		v, err := api.Version(ctx)
		if err != nil {
			fmt.Printf("获取版本信息失败\n")
			return err
		}
		if !v.APIVersion.EqMajorMinor(lapi.FullAPIVersion0) {
			fmt.Printf("Remote API version didn't match (expected %s, remote %s)\n", lapi.FullAPIVersion1, v.APIVersion)
			return xerrors.Errorf("Remote API version didn't match (expected %s, remote %s)", lapi.FullAPIVersion1, v.APIVersion)
		}

		// 用于根据 矿工获取矿工owner账户
		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			fmt.Printf("输入miner ID(%s)不正确。 %v\n", cctx.Args().First(), err)
			return err
		}

		owner, err := api.StateAccountKey(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			fmt.Printf("%s\t%s: error getting account key: %s\n", "owner", owner, err)
			return err
		}
		fmt.Printf("--->%+v", owner)

		// 获取矿工可用余额
		available, err := api.StateMinerAvailableBalance(ctx, maddr, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取矿工(%s)余额失败。 %v\n", cctx.Args().First(), err)
			return err
		}

		amount := available
		f, err := types.ParseFIL(cctx.Args().Get(1))
		if err == nil {
			amount = abi.TokenAmount(f)
			//return xerrors.Errorf("parsing 'amount' argument: %w", err)
		} else {
			fmt.Printf("未指定提现 金额，将miner 所有可用余额（%s）提现，\n", types.FIL(amount).Short())
		}

		if amount.GreaterThan(available) {
			fmt.Printf("提现金额%s 超过miner可用余额(%s)，提现失败\n", amount, available)
			return xerrors.Errorf("can't withdraw more funds than available; requested: %s; available: %s", amount, available)
		}

		// 获取nonce
		a, err := api.StateGetActor(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取获取owner地址的nonce失败，err:%v\n", err)
			return err
		}

		params, err := actors.SerializeParams(&miner2.WithdrawBalanceParams{
			AmountRequested: amount, // Default to attempting to withdraw all the extra funds in the miner actor
		})
		if err != nil {
			fmt.Printf("序列化提现参数失败，err:%v\n", err)
			return err
		}

		msg, err := api.GasEstimateMessageGas(ctx, &types.Message{
			To:     maddr,
			From:   owner,
			Value:  types.NewInt(0),
			Method: builtin.MethodsMiner.WithdrawBalance,
			Nonce:  a.Nonce,
			Params: params,
		}, nil, types.EmptyTSK)
		if err != nil {
			fmt.Printf("评估消息的gas费用失败， err:%v\n", err)
			return xerrors.Errorf("GasEstimateMessageGas error: %w", err)
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

		fmt.Printf("Requested rewards withdrawal in message %s\n", cid.String())

		return nil
	},
}

var controlSetCmd = &cli.Command{
	Name:      "control-set",
	Usage:     "Set control address(-es)",
	ArgsUsage: "[minerId (eg. f021704)] [...address]",
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		//nodeApi, closer, err := GetStorageMinerAPI(cctx)
		//if err != nil {
		//	return err
		//}
		//defer closer()
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Println(err)
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		//maddr, err := nodeApi.ActorAddress(ctx)
		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			fmt.Println(err)
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			fmt.Println(err)
			return err
		}

		del := map[address.Address]struct{}{}
		existing := map[address.Address]struct{}{}
		for _, controlAddress := range mi.ControlAddresses {
			ka, err := api.StateAccountKey(ctx, controlAddress, types.EmptyTSK)
			if err != nil {
				fmt.Println(err)
				return err
			}

			del[ka] = struct{}{}
			existing[ka] = struct{}{}
		}

		var toSet []address.Address

		for i, as := range cctx.Args().Tail() {
			a, err := address.NewFromString(as)
			if err != nil {
				fmt.Println(err)
				return xerrors.Errorf("parsing address %d: %w", i, err)
			}

			ka, err := api.StateAccountKey(ctx, a, types.EmptyTSK)
			if err != nil {
				fmt.Println(err)
				return err
			}

			// make sure the address exists on chain
			_, err = api.StateLookupID(ctx, ka, types.EmptyTSK)
			if err != nil {
				fmt.Println(err)
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

		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		cwp := &miner2.ChangeWorkerAddressParams{
			NewWorker:       mi.Worker,
			NewControlAddrs: toSet,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			fmt.Println(err)
			return xerrors.Errorf("serializing params: %w", err)
		}

		// 获取nonce
		a, err := api.StateGetActor(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取获取owner地址的nonce失败，err:%v\n", err)
			return err
		}

		owner, err := api.StateAccountKey(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			fmt.Printf("%s\t%s: error getting account key: %s\n", "owner", owner, err)
			return err
		}

		msg, err := api.GasEstimateMessageGas(ctx, &types.Message{
			To:     maddr,
			From:   owner,
			Value:  big.Zero(),
			Method: builtin.MethodsMiner.ChangeWorkerAddress,
			Nonce:  a.Nonce,
			Params: sp,
		}, nil, types.EmptyTSK)
		if err != nil {
			fmt.Printf("评估消息的gas费用失败， err:%v\n", err)
			return xerrors.Errorf("GasEstimateMessageGas error: %w", err)
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

		fmt.Println("Message CID:", cid.String())

		return nil
	},
}

var setOwnerCmd = &cli.Command{
	Name:      "set-owner",
	Usage:     "设置矿工的owner地址 (设置过程中这个命令需要被执行两次, 第一次用旧的ownr地址发送, 第二次用新的owner地址发送)",
	ArgsUsage: "[miner 新owner地址 发送地址]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "确定命令，防止误操作",
			Value: false,
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
		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		if cctx.NArg() != 3 {
			fmt.Println("必须输入矿工编号，新的owner地址，和发送钱包地址")
			return fmt.Errorf("must pass miner id, new owner address and sender address")
		}

		//nodeApi, closer, err := GetStorageMinerAPI(cctx)
		//if err != nil {
		//	return err
		//}
		//defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Printf("连接FULLNODE_API_INFO api失败。%v\n", err)
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		na, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			fmt.Printf("解析新的owner地址失败。%v\n", err)
			return err
		}

		newAddrId, err := api.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取新owner地址链上状态失败。%v\n", err)
			return err
		}

		fa, err := address.NewFromString(cctx.Args().Get(2))
		if err != nil {
			fmt.Printf("解析新的发送地址失败。%v\n", err)
			return err
		}

		fromAddrId, err := api.StateLookupID(ctx, fa, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取新发送地址链上状态失败。%v\n", err)
			return err
		}

		//maddr, err := nodeApi.ActorAddress(ctx)
		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			fmt.Println("读取矿工地址失败", err)
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			fmt.Println("从链上读取矿工状态失败", err)
			return err
		}

		if fromAddrId != mi.Owner && fromAddrId != newAddrId {
			fmt.Println("发送地址必须为新的owner地址或者旧的owner地址")
			return xerrors.New("from address must either be the old owner or the new owner")
		}

		sp, err := actors.SerializeParams(&newAddrId)
		if err != nil {
			fmt.Println("序列化发送参数失败", err)
			return xerrors.Errorf("serializing params: %w", err)
		}

		// 获取nonce
		a, err := api.StateGetActor(ctx, fromAddrId, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取获取owner地址的nonce失败，err:%v\n", err)
			return err
		}

		msg, err := api.GasEstimateMessageGas(ctx, &types.Message{
			From:   fa,
			To:     maddr,
			Method: builtin.MethodsMiner.ChangeOwnerAddress,
			Value:  big.Zero(),
			Params: sp,
			Nonce:  a.Nonce,
		}, nil, types.EmptyTSK)
		if err != nil {
			fmt.Println("评估消息gas失败，", err)
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Printf("\n%+v\n", msg)

		mb, err := msg.ToStorageBlock()
		if err != nil {
			fmt.Printf("序列化消息失败， err:%v\n", err)
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

		fmt.Println("Message CID:", cid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, cid, build.MessageConfidence)
		if err != nil {
			fmt.Println("等效消息返回失败,", err)
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Println("发送修改owner地址失败!")
			return err
		}

		fmt.Println("消息发送成功！")

		return nil
	},
}

var controlListCmd = &cli.Command{
	Name:      "control-list",
	Usage:     "Get currently set control addresses",
	ArgsUsage: "[minerId]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "verbose",
		},
		&cli.BoolFlag{
			Name:  "color",
			Value: true,
		},
	},
	//Before: func(context *cli.Context) error {
	//	if err := _init(); err != nil {
	//		passwdValid = false
	//	}
	//	return nil
	//},
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")
		//if !passwdValid {
		//	fmt.Println("密码错误.")
		//	return fmt.Errorf("密码错误")
		//}
		//nodeApi, closer, err := GetStorageMinerAPI(cctx)
		//if err != nil {
		//	return err
		//}
		//defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Println(err)
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		//maddr, err := nodeApi.ActorAddress(ctx)
		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			fmt.Println(err)
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			fmt.Println(err)
			return err
		}

		mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(api), blockstore.NewMemory())

		mas, err := miner.Load(adt.WrapStore(ctx, cbor.NewCborStore(tbs)), mact)
		if err != nil {
			return err
		}

		// 获取矿工可用余额
		available, err := api.StateMinerAvailableBalance(ctx, maddr, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取矿工(%s)余额失败。 %v\n", cctx.Args().First(), err)
			return err
		}

		tw := tablewriter.New(
			tablewriter.Col("name"),
			tablewriter.Col("ID"),
			tablewriter.Col("key"),
			tablewriter.Col("use"),
			tablewriter.Col("balance"),
		)

		//ac, err := nodeApi.ActorAddressConfig(ctx)
		//if err != nil {
		//	return err
		//}

		commit := map[address.Address]struct{}{}
		precommit := map[address.Address]struct{}{}
		terminate := map[address.Address]struct{}{}
		post := map[address.Address]struct{}{}

		for _, ca := range mi.ControlAddresses {
			post[ca] = struct{}{}
		}

		printKey := func(name string, a address.Address) {
			b, err := api.WalletBalance(ctx, a)
			if err != nil {
				fmt.Printf("%s\t%s: error getting balance: %s\n", name, a, err)
				return
			}

			k, err := api.StateAccountKey(ctx, a, types.EmptyTSK)
			if err != nil {
				fmt.Printf("%s\t%s: error getting account key: %s\n", name, a, err)
				return
			}

			kstr := k.String()
			//if !cctx.Bool("verbose") {
			//	kstr = kstr[:9] + "..."
			//}

			bstr := types.FIL(b).String()
			switch {
			case b.LessThan(types.FromFil(10)):
				bstr = color.RedString(bstr)
			case b.LessThan(types.FromFil(50)):
				bstr = color.YellowString(bstr)
			default:
				bstr = color.GreenString(bstr)
			}

			var uses []string
			if a == mi.Worker {
				uses = append(uses, color.YellowString("other"))
			}
			if _, ok := post[a]; ok {
				uses = append(uses, color.GreenString("post"))
			}
			if _, ok := precommit[a]; ok {
				uses = append(uses, color.CyanString("precommit"))
			}
			if _, ok := commit[a]; ok {
				uses = append(uses, color.BlueString("commit"))
			}
			if _, ok := terminate[a]; ok {
				uses = append(uses, color.YellowString("terminate"))
			}

			tw.Write(map[string]interface{}{
				"name":    name,
				"ID":      a,
				"key":     kstr,
				"use":     strings.Join(uses, " "),
				"balance": bstr,
			})
		}

		printKey("owner", mi.Owner)
		printKey("worker", mi.Worker)
		printKey("newWorker", mi.NewWorker)
		for i, ca := range mi.ControlAddresses {
			printKey(fmt.Sprintf("control-%d", i), ca)
		}
		tw.Write(map[string]interface{}{
			"name":    "miner",
			"ID":      maddr.String(),
			"key":     "",
			"use":     "available",
			"balance": color.HiGreenString(types.FIL(available).Short()),
		})

		lockedFunds, err := mas.LockedFunds()
		if err != nil {
			return xerrors.Errorf("getting locked funds: %w", err)
		}

		tw.Write(map[string]interface{}{
			"name":    "miner pledge",
			"ID":      maddr.String(),
			"key":     "",
			"use":     "init pledge",
			"balance": color.HiGreenString(types.FIL(lockedFunds.InitialPledgeRequirement).Short()),
		})

		tw.Write(map[string]interface{}{
			"name":    "miner vesting",
			"ID":      maddr.String(),
			"key":     "",
			"use":     "vesting",
			"balance": color.HiGreenString(types.FIL(lockedFunds.VestingFunds).Short()),
		})

		return tw.Flush(os.Stdout)
	},
}

var proposeChangeWorker = &cli.Command{
	Name:      "propose-change-worker",
	Usage:     "修改worker钱包地址,（worker的地址必须是bls类型）",
	ArgsUsage: "[矿工地址, 新worker地址]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "确认执行的命令",
			Value: false,
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
		if cctx.Args().Len() != 2 {
			fmt.Println("miner地址和新worker地址必须要输入.")
			return fmt.Errorf("must pass address of new worker address")
		}

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Println("连接FULLNODE_API_INFO失败")
			return fmt.Errorf("must pass address of new worker address")
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		// 目标地址
		na, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			fmt.Println("获取新的worker地址失败")
			return err
		}

		newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			fmt.Println("从链上读取新worker地址失败")
			return err
		}

		// 矿工地址
		//maddr, err := nodeApi.ActorAddress(ctx)
		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			fmt.Println("解析矿工地址失败")
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			fmt.Println("从链上读取矿工地址失败")
			return err
		}

		if mi.NewWorker.Empty() {
			if mi.Worker == newAddr {
				fmt.Printf("钱包地址已经设置为#{%s}\n", na)
				return fmt.Errorf("worker address already set to %s", na)
			}
		} else {
			if mi.NewWorker == newAddr {
				fmt.Printf("新的钱包地址已经设置为#{%s}\n", na)
				return fmt.Errorf("change to worker address %s already pending", na)
			}
		}

		if !cctx.Bool("really-do-it") {
			fmt.Fprintln(cctx.App.Writer, "请输入 --really-do-it 参数执行命令")
			return nil
		}

		cwp := &miner2.ChangeWorkerAddressParams{
			NewWorker:       newAddr,
			NewControlAddrs: mi.ControlAddresses,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			fmt.Println("序列化消息参数失败")
			return xerrors.Errorf("serializing params: %w", err)
		}

		realOwner, err := api.StateAccountKey(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			fmt.Printf("%s\t%s: error getting account key: %s\n", maddr, mi.Owner, err)
			return nil
		}

		// 获取nonce
		a, err := api.StateGetActor(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取获取owner地址的nonce失败，err:%v\n", err)
			return err
		}

		msg, err := api.GasEstimateMessageGas(ctx, &types.Message{
			From:   realOwner,
			To:     maddr,
			Method: builtin.MethodsMiner.ChangeWorkerAddress,
			Value:  big.Zero(),
			Params: sp,
			Nonce:  a.Nonce,
		}, nil, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("GasEstimateMessageGas: %w", err)
		}

		fmt.Printf("\n%+v\n", msg)

		mb, err := msg.ToStorageBlock()
		if err != nil {
			fmt.Printf("序列化消息失败， err:%v\n", err)
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

		fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", cid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, cid, build.MessageConfidence)
		if err != nil {
			fmt.Println("等待消息返回失败")
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Fprintln(cctx.App.Writer, "Propose worker change failed!")
			return err
		}

		mi, err = api.StateMinerInfo(ctx, maddr, wait.TipSet)
		if err != nil {
			fmt.Println("获取miner状态失败")
			return err
		}
		if mi.NewWorker != newAddr {
			fmt.Printf("Proposed worker address change not reflected on chain: expected '%s', found '%s'\n", na, mi.NewWorker)
			return fmt.Errorf("Proposed worker address change not reflected on chain: expected '%s', found '%s'", na, mi.NewWorker)
		}

		fmt.Fprintf(cctx.App.Writer, "Worker key change to %s successfully proposed.\n", na)
		fmt.Fprintf(cctx.App.Writer, "Call 'confirm-change-worker' at or after height %d to complete.\n", mi.WorkerChangeEpoch)

		return nil
	},
}

var confirmChangeWorker = &cli.Command{
	Name:      "confirm-change-worker",
	Usage:     "Confirm a worker address change",
	ArgsUsage: "[旷工地址 newaddress]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
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
		if cctx.Args().Len() != 2 {
			fmt.Println("miner地址和新worker地址必须要输入.")
			return fmt.Errorf("must pass address of new worker address")
		}

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Println(err)
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		na, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			fmt.Println(err)
			return err
		}

		// 矿工地址
		//maddr, err := nodeApi.ActorAddress(ctx)
		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			fmt.Println("解析矿工地址失败")
			return err
		}

		newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
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
			fmt.Println(xerrors.Errorf("no worker key change proposed"))
			return xerrors.Errorf("no worker key change proposed")
		} else if mi.NewWorker != newAddr {
			fmt.Println(xerrors.Errorf("worker key %s does not match current worker key proposal %s", newAddr, mi.NewWorker))
			return xerrors.Errorf("worker key %s does not match current worker key proposal %s", newAddr, mi.NewWorker)
		}

		if head, err := api.ChainHead(ctx); err != nil {
			return xerrors.Errorf("failed to get the chain head: %w", err)
		} else if head.Height() < mi.WorkerChangeEpoch {
			fmt.Println(xerrors.Errorf("worker key change cannot be confirmed until %d, current height is %d", mi.WorkerChangeEpoch, head.Height()))
			return xerrors.Errorf("worker key change cannot be confirmed until %d, current height is %d", mi.WorkerChangeEpoch, head.Height())
		}

		if !cctx.Bool("really-do-it") {
			fmt.Fprintln(cctx.App.Writer, "Pass --really-do-it to actually execute this action")
			return nil
		}

		realOwner, err := api.StateAccountKey(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			fmt.Printf("%s\t%s: error getting account key: %s\n", maddr, mi.Owner, err)
			return nil
		}

		// 获取nonce
		a, err := api.StateGetActor(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取获取owner地址的nonce失败，err:%v\n", err)
			return err
		}

		// 构造msg
		msg, err := api.GasEstimateMessageGas(ctx, &types.Message{
			From:   realOwner,
			To:     maddr,
			Method: builtin.MethodsMiner.ConfirmChangeWorkerAddress,
			Value:  big.Zero(),
			Nonce:  a.Nonce,
		}, nil, types.EmptyTSK)

		if err != nil {
			fmt.Println(xerrors.Errorf("GasEstimateMessageGas: %w", err))
			return xerrors.Errorf("GasEstimateMessageGas: %w", err)
		}

		fmt.Printf("\n%+v\n", msg)

		mb, err := msg.ToStorageBlock()
		if err != nil {
			fmt.Printf("序列化消息失败， err:%v\n", err)
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

		fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", cid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, cid, build.MessageConfidence)
		if err != nil {
			fmt.Println(err)
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Fprintln(cctx.App.Writer, "Worker change failed!")
			return err
		}

		mi, err = api.StateMinerInfo(ctx, maddr, wait.TipSet)
		if err != nil {
			fmt.Println(err)
			return err
		}
		if mi.Worker != newAddr {
			fmt.Printf("Confirmed worker address change not reflected on chain: expected '%s', found '%s'\n", newAddr, mi.Worker)
			return fmt.Errorf("Confirmed worker address change not reflected on chain: expected '%s', found '%s'", newAddr, mi.Worker)
		}

		return nil
	},
}

const FlagMinerRepo = "./lotusstorage"

var newMinerCmd = &cli.Command{
	Name:  "newMiner",
	Usage: "创建新的miner : newMiner --owner fxxx --worker f3xxxx",
	//ArgsUsage: " --owner fxxxx",
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "owner",
			Aliases:  []string{"o"},
			Usage:    "指定 owner  地址",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "worker",
			Aliases:  []string{"w"},
			Usage:    "指定 worker  地址，必须是f3地址",
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {

		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			fmt.Printf("连接FULLNODE_API_INFO api失败。%v\n", err)
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		// 默认指定扇区大小就是32GiB
		ssize, err := abi.RegisteredSealProof_StackedDrg32GiBV1.SectorSize()
		if err != nil {
			return xerrors.Errorf("failed to calculate default sector size: %w", err)
		}
		fmt.Println("Checking if repo exists")

		repoPath := FlagMinerRepo
		r, err := repo.NewFS(repoPath)
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}
		if ok {
			return xerrors.Errorf("repo at '%s' is already initialized", cctx.String(FlagMinerRepo))
		}

		fmt.Println("Checking full node version")

		v, err := api.Version(ctx)
		if err != nil {
			return err
		}
		if !v.APIVersion.EqMajorMinor(lapi.FullAPIVersion0) {
			fmt.Println(xerrors.Errorf("Remote API version didn't match (expected %s, remote %s)", lapi.FullAPIVersion1, v.APIVersion))
			return xerrors.Errorf("Remote API version didn't match (expected %s, remote %s)", lapi.FullAPIVersion1, v.APIVersion)
		}

		fmt.Println("Initializing repo .")

		if err := r.Init(repo.StorageMiner); err != nil {
			fmt.Println("r.Init err : ", err)
			return err
		}

		{
			lr, err := r.Lock(repo.StorageMiner)
			if err != nil {
				fmt.Println(err)
				return err
			}

			var localPaths []storiface.LocalPath

			if pssb := cctx.StringSlice("pre-sealed-sectors"); len(pssb) != 0 {
				fmt.Printf("Setting up storage config with presealed sectors: %v\n", pssb)

				for _, psp := range pssb {
					psp, err := homedir.Expand(psp)
					if err != nil {
						fmt.Println(err)
						return err
					}
					localPaths = append(localPaths, storiface.LocalPath{
						Path: psp,
					})
				}
			}

			if !cctx.Bool("no-local-storage") {
				b, err := json.MarshalIndent(&storiface.LocalStorageMeta{
					ID:       storiface.ID(uuid.New().String()),
					Weight:   10,
					CanSeal:  true,
					CanStore: true,
				}, "", "  ")
				if err != nil {
					fmt.Println(err)
					return xerrors.Errorf("marshaling storage config: %w", err)
				}

				if err := os.WriteFile(filepath.Join(lr.Path(), "sectorstore.json"), b, 0644); err != nil {
					fmt.Println(err)
					return xerrors.Errorf("persisting storage metadata (%s): %w", filepath.Join(lr.Path(), "sectorstore.json"), err)
				}

				localPaths = append(localPaths, storiface.LocalPath{
					Path: lr.Path(),
				})
			}

			if err := lr.SetStorage(func(sc *storiface.StorageConfig) {
				sc.StoragePaths = append(sc.StoragePaths, localPaths...)
			}); err != nil {
				fmt.Println(err)
				return xerrors.Errorf("set storage config: %w", err)
			}

			if err := lr.Close(); err != nil {
				fmt.Println(err)
				return err
			}
		}

		gasPrice, _ := types.BigFromString("0")
		confidence := buildconstants.MessageConfidence
		if err := storageMinerInit(ctx, cctx, api, r, ssize, gasPrice, confidence); err != nil {
			fmt.Println("Failed to initialize lotus-miner: ", err)
			path, err := homedir.Expand(repoPath)
			if err != nil {
				fmt.Println(err)
				return err
			}
			fmt.Printf("Cleaning up %s after attempt...\n", path)
			if err := os.RemoveAll(path); err != nil {
				fmt.Println("Failed to clean up failed storage repo: ", err)
			}
			fmt.Println(err)
			return xerrors.Errorf("Storage-miner init failed")
		}

		// TODO: Point to setting storage price, maybe do it interactively or something
		fmt.Println("Miner successfully created, you can now start it with 'lotus-miner run'")

		return nil
	},
}

func storageMinerInit(ctx context.Context, cctx *cli.Context, api v1api.FullNode, r repo.Repo, ssize abi.SectorSize, gasPrice types.BigInt, confidence uint64) error {
	lr, err := r.Lock(repo.StorageMiner)
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer lr.Close() //nolint:errcheck

	fmt.Println("Initializing libp2p identity")

	p2pSk, err := makeHostKey(lr)
	if err != nil {
		fmt.Println(err)
		return xerrors.Errorf("make host key: %w", err)
	}

	peerid, err := peer.IDFromPrivateKey(p2pSk)
	if err != nil {
		fmt.Println(err)
		return xerrors.Errorf("peer ID from private key: %w", err)
	}

	mds, err := lr.Datastore(ctx, "/metadata")
	if err != nil {
		fmt.Println(err)
		return err
	}

	var addr address.Address
	a, err := createStorageMiner(ctx, api, ssize, peerid, gasPrice, confidence, cctx)
	if err != nil {
		fmt.Println(err)
		return xerrors.Errorf("creating miner failed: %w", err)
	}

	addr = a

	fmt.Printf("Created new miner: %s\n", addr)
	if err := mds.Put(ctx, datastore.NewKey("miner-address"), addr.Bytes()); err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func makeHostKey(lr repo.LockedRepo) (crypto.PrivKey, error) {
	pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	ks, err := lr.KeyStore()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	kbytes, err := crypto.MarshalPrivateKey(pk)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	if err := ks.Put("libp2p-host", types.KeyInfo{
		Type:       "libp2p-host",
		PrivateKey: kbytes,
	}); err != nil {
		fmt.Println(err)
		return nil, err
	}

	return pk, nil
}

func createStorageMiner(ctx context.Context, api v1api.FullNode, ssize abi.SectorSize, peerid peer.ID, _ types.BigInt, confidence uint64, cctx *cli.Context) (address.Address, error) {
	var err error
	var owner address.Address
	if cctx.String("owner") != "" {
		owner, err = address.NewFromString(cctx.String("owner"))
	} else {
		fmt.Println("必须指定 --owner ")
		return address.Undef, err
	}

	worker := owner
	if cctx.String("worker") != "" {
		worker, err = address.NewFromString(cctx.String("worker"))
	} else {
		fmt.Println("必须指定 --worker 且worker必须是 f3 地址")
		return address.Undef, err
	}

	// make sure the sender account exists on chain
	_, err = api.StateLookupID(ctx, owner, types.EmptyTSK)
	if err != nil {
		fmt.Printf("钱包地址在链上不存在，请先往地址（%s）转1 fil 然后等3分钟再执行此操作. \n",owner)
		return address.Undef, xerrors.Errorf("sender must exist on chain: %w", err)
	}

	// Note: the correct thing to do would be to call SealProofTypeFromSectorSize if actors version is v3 or later, but this still works
	nv, err := api.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		fmt.Println(err)
		return address.Undef, xerrors.Errorf("failed to get network version: %w", err)
	}
	spt, err := miner.WindowPoStProofTypeFromSectorSize(ssize, nv)
	if err != nil {
		fmt.Println(err)
		return address.Undef, xerrors.Errorf("getting post proof type: %w", err)
	}

	params, err := actors.SerializeParams(&power6.CreateMinerParams{
		Owner:               owner,
		Worker:              worker,
		WindowPoStProofType: spt,
		Peer:                abi.PeerID(peerid),
	})
	if err != nil {
		fmt.Println(err)
		return address.Undef, err
	}

	// 本地签名操作： 1 获取地址的nonce， 2 评估gas，3 本地签名，4 推送消息
	// 1 获取nonce
	a, err := api.StateGetActor(ctx, owner, types.EmptyTSK)
	if err != nil {
		fmt.Printf("读取获取owner地址的nonce失败，err:%v\n", err)
		return address.Undef, err
	}

	// 2 评估gas
	msg, err := api.GasEstimateMessageGas(ctx, &types.Message{
		From:  owner,
		To:    power.Address,
		Value: big.Zero(),

		Method: power.Methods.CreateMiner,
		Params: params,
		Nonce:  a.Nonce,
	}, nil, types.EmptyTSK)
	if err != nil {
		fmt.Println("评估消息gas失败，", err)
		return address.Undef, xerrors.Errorf("mpool push: %w", err)
	}

	fmt.Printf("\n%+v\n", msg)

	mb, err := msg.ToStorageBlock()
	if err != nil {
		fmt.Printf("序列化消息失败， err:%v\n", err)
		return address.Undef, xerrors.Errorf("serializing message: %w", err)
	}

	// 3 签名
	sb, err := signMessage(mb.Cid().Bytes(), msg.From)
	if err != nil {
		fmt.Printf("签名失败， err:%v\n", err)
		return address.Undef, xerrors.Errorf("签名失败: %w", err)
	}

	// 4 推送消息
	cid, err := api.MpoolPush(ctx, &types.SignedMessage{Message: *msg, Signature: *sb})
	if err != nil {
		fmt.Printf("推送消息上链失败，err:%v\n", err)
		return address.Undef, xerrors.Errorf("pushing createMiner message: %w", err)
	}

	fmt.Printf("Pushed CreateMiner message: %s\n", cid.String())
	fmt.Println("Waiting for confirmation")

	mw, err := api.StateWaitMsg(ctx, cid, confidence, lapi.LookbackNoLimit, true)
	if err != nil {
		fmt.Println(xerrors.Errorf("waiting for createMiner message: %w", err))
		return address.Undef, xerrors.Errorf("waiting for createMiner message: %w", err)
	}

	if mw.Receipt.ExitCode != 0 {
		fmt.Println(xerrors.Errorf("create miner failed: exit code %d", mw.Receipt.ExitCode))
		return address.Undef, xerrors.Errorf("create miner failed: exit code %d", mw.Receipt.ExitCode)
	}

	var retval power2.CreateMinerReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(mw.Receipt.Return)); err != nil {
		fmt.Println(err)
		return address.Undef, err
	}

	fmt.Printf("New miners address is: %s (%s)\n", retval.IDAddress, retval.RobustAddress)
	return retval.IDAddress, nil
}
