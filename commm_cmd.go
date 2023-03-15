package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
	"github.com/urfave/cli/v2"
	"os"
	"strings"
)

// 用来保存一些常用的命令工具
var commCmd = &cli.Command{
	Name:  "comm",
	Usage: "用来保存一些常用的命令工具",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		countAvailableFil,
	},
}

// 统计可用的fil
var countAvailableFil = &cli.Command{
	Name:      "count-list",
	Usage:     "查询给定miner和钱包地址的可用余额",
	ArgsUsage: "[minerId,minerID...]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "verbose",
		},
		&cli.BoolFlag{
			Name:  "color",
			Value: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Println(err)
			return err
		}
		defer acloser()
		ctx := lcli.ReqContext(cctx)

		tw := tablewriter.New(
			tablewriter.Col("name"),
			tablewriter.Col("ID"),
			tablewriter.Col("key"),
			tablewriter.Col("use"),
			tablewriter.Col("balance"),
		)


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
			bstr := types.FIL(b).String()
			switch {
			case b.LessThan(types.FromFil(10)):
				bstr = color.RedString(bstr)
			case b.LessThan(types.FromFil(50)):
				bstr = color.YellowString(bstr)
			default:
				bstr = color.GreenString(bstr)
			}

			tw.Write(map[string]interface{}{
				"name":    name,
				"ID":      a,
				"key":     kstr,
				"use":     "",
				"balance": bstr,
			})
		}

		dododo:= func(maddr address.Address,total *types.BigInt) {

			mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				fmt.Println(err)
				return
			}

			// 获取矿工可用余额
			available, err := api.StateMinerAvailableBalance(ctx, maddr, types.EmptyTSK)
			if err != nil {
				fmt.Printf("读取矿工(%s)余额失败。 %v\n", cctx.Args().First(), err)
				return
			}

			printKey("owner", mi.Owner)
			printKey("worker", mi.Worker)
			if !mi.NewWorker.Empty(){
				printKey("newWorker", mi.NewWorker)
			}
			for i, ca := range mi.ControlAddresses {
				printKey(fmt.Sprintf("control-%d", i), ca)
			}

			tw.Write(map[string]interface{}{
				"name":    "miner",
				"ID":      maddr.String(),
				"key":     "",
				"use":     "available",
				"balance": color.HiGreenString(types.FIL(available).String()),
			})
			tw.Write(map[string]interface{}{"key":"--------------------------------------------------"})
			b, err := api.WalletBalance(ctx, mi.Owner)
			if total.NilOrZero(){
				*total=available
			}else{
				*total=types.BigAdd(*total,available)
			}
			*total=types.BigAdd(*total,b)
		}

		targAddrs:=cctx.Args().Slice()
		totalAvailable:=types.BigInt{}
		for _, addr := range targAddrs {
			a, err := address.NewFromString(addr)
			if err != nil {
				fmt.Println(err)
				continue
			}

			if strings.HasPrefix(addr,"f0"){
				dododo(a,&totalAvailable)
			}else{
				b, err := api.WalletBalance(ctx, a)
				if err!=nil{
					fmt.Println(err)
					continue
				}
				totalAvailable=types.BigAdd(totalAvailable,b)
				printKey("-",a)
			}
		}
		tw.Flush(os.Stdout)
		fmt.Printf("总可用额:%s\n",types.FIL(totalAvailable).Short())
		return nil
	},
}
