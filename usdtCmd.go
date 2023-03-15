package main

import (
	"fmt"
	"github.com/filecoin-project/firefly-wallet/db"
	"github.com/filecoin-project/firefly-wallet/impl"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/urfave/cli/v2"
	"strconv"
)

var createUSDTCmd = &cli.Command{
	Name:  "new-usdt-account",
	Usage: "创建一个usdt账户",
	Flags: []cli.Flag{
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

		strIndex, err := localdb.Get(db.USDTKeyIndex, NEXT)
		if err != nil {
			fmt.Println(err)
			if err != errors.ErrNotFound {
				panic(err)
			}
			strIndex = []byte("0")
		}

		index, err := strconv.Atoi(string(strIndex))
		if err != nil {
			fmt.Println(err)
			panic(err)
		}

		address, err := impl.CreateUsdtAddress(index, string(localMnenoic))
		if err != nil {
			fmt.Println(err)
			return err
		}

		err = localdb.Add(db.USDTKeyIndex, NEXT, []byte(fmt.Sprintf("%d", index+1)))
		if err != nil {
			panic(err)
		}

		err = localdb.Add(db.KeyAddr, address, []byte(address))
		if err != nil {
			panic(err)
		}

		fmt.Println(address)

		return nil
	},
}

