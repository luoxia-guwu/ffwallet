package main

import (
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/firefly-wallet/db"
	"github.com/urfave/cli/v2"
	"strings"
)

var localdbCmd = &cli.Command{
	Name:  "localdbs",
	Usage: "查询本地数据库信息",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		lsCmd,
		modCmd,
	},
}

var lsCmd = &cli.Command{
	Name:  "ls",
	Usage: "列出本地数据记录的所有记录",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "miner-id",
			Usage: "列出指定miner的地址,不指定则列出所有",
			Value: "",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(context *cli.Context) error {
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		minerId := context.String("miner-id")

		vmaps, err := localdb.GetAll(db.KeyAddr)
		if err != nil {
			panic(err)
		}

		m := map[string]map[FilAddressInfo]string{}
		for _, faiString := range vmaps {
			fmt.Println(faiString)
			fai := FilAddressInfo{}
			err = json.Unmarshal([]byte(faiString), &fai)
			if err != nil {
				panic(err)
			}

			if minerId != "" && minerId != fai.MinerId {
				continue
			}

			_, ok := m[fai.MinerId]
			if !ok {
				m[fai.MinerId] = map[FilAddressInfo]string{}
			}
			m[fai.MinerId][fai] = ""
		}

		for minerId, addrInfoMap := range m {
			fmt.Println(minerId, ":")
			addrs := map[string]map[int]string{}
			for addrInfo, _ := range addrInfoMap {
				_, ok := addrs[addrInfo.AddrType]
				if !ok {
					addrs[addrInfo.AddrType] = map[int]string{}
				}
				addrs[addrInfo.AddrType][addrInfo.Index] = addrInfo.Address
				//fmt.Println("     ", addrType, " : ", addr)
			}

			for addrType, addresses := range addrs {
				fmt.Println("    ", addrType, " : ")
				for index, addr := range addresses {
					fmt.Printf("        %d - %s\n", index, addr)
				}
			}
			fmt.Println("----------")
		}
		return nil
	},
}

var modCmd = &cli.Command{
	Name:  "mod",
	Usage: "修改指定地址的记录信息(miner信息，type类型",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "addr",
			Usage: "需要修改的地址",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "miner",
			Usage: "指定修改的地址所属的miner",
			Value: "",
		}, &cli.StringFlag{
			Name:  "type",
			Usage: "指定修改的地址类型，owner、worker、post、",
			Value: "",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(context *cli.Context) error {

		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		addr := context.String("addr")
		miner := context.String("miner")
		addrType := context.String("type")

		if !strings.HasPrefix(addr, "f3") && !strings.HasPrefix(addr, "f1") {
			fmt.Printf("不合法的 addr (%v)\n", addr)
			return nil
		}

		if len(addrType) == 0 || len(miner) == 0 {
			fmt.Printf("miner 和type必须都指定 \n")
			return nil
		}

		if !strings.HasPrefix(miner, "f0") {
			fmt.Printf("不合法的 addr (%v)\n", addr)
			return nil
		}

		if strings.Compare(addrType, string(db.OwnerAddr)) == 0 || strings.Compare(addrType, string(db.PostAddr)) == 0 || strings.Compare(addrType, string(db.WorkerAddr)) == 0 {
		} else {
			fmt.Printf("不合法的 type (%v), 必须为：worker、post、owner中的一个 ", addrType)
			return nil
		}

		filKey, err := localdb.Get(db.KeyAddr, addr)
		if err != nil {
			fmt.Printf("本地数据库没有查询到key（%v）!\n", addr)
			return err
		}

		fai := &FilAddressInfo{}
		err = json.Unmarshal(filKey, fai)
		if err != nil {
			fmt.Printf("解析数据(%s)失败（%v）!\n", filKey, err)
			return err
		}

		fai.MinerId = miner
		fai.AddrType = addrType

		data, err := json.Marshal(fai)
		if err != nil {
			fmt.Printf("序列化数据(%v)失败!, err: %v\n", fai, err)
			return err
		}

		err = localdb.Add(db.KeyAddr, addr, data)
		if err != nil {
			fmt.Printf("数据(%s)写入数据库失败!, err: %v\n", data, err)
			return err
		}
		fmt.Println("修改成功")

		return nil
	},
}
