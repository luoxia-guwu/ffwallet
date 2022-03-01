package main

import (
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
	"io/ioutil"
	"strings"
)

var toolsCmd = &cli.Command{
	Name:  "tool",
	Usage: "查询本地数据库信息",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		listPostCmd,
	},
}

var listPostCmd = &cli.Command{
	Name:      "list-post",
	Usage:     "列出指定矿工的post账户余额情况",
	ArgsUsage: "miners.txt,样式：f02420;f0144528;",
	Flags:     []cli.Flag{},
	Action: func(cctx *cli.Context) error {

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Println(err)
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		data, err := ioutil.ReadFile(cctx.Args().First())
		if err != nil {
			fmt.Println(err)
			return err
		}
		miners := strings.Split(string(data), ";")

		if len(miners) <= 0 {
			fmt.Println("miners.txt 内容为空")
			return nil
		}

		for _, miner := range miners {
			if len(miner) < 3 {
				continue
			}
			maddr, err := address.NewFromString(miner)
			if err != nil {
				fmt.Println(err)
				continue
			}

			mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				fmt.Println(err)
				continue
			}

			addr := mi.Worker
			if len(mi.ControlAddresses) > 0 {
				addr = mi.ControlAddresses[0]
			}

			b, err := api.WalletBalance(ctx, addr)
			if err != nil {
				fmt.Println(err)
				continue
			}

			if len(mi.ControlAddresses) > 0 {
				fmt.Printf("%s - post %s : %s\n", miner, addr, types.FIL(b).String())
				addr = mi.ControlAddresses[0]
			} else {
				fmt.Printf("%s - work(post) %s : %s\n", miner, addr, types.FIL(b).String())
			}
		}

		return nil
	},
}
