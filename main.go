package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/firefly-wallet/db"
	"github.com/filecoin-project/firefly-wallet/impl"
	"github.com/filecoin-project/firefly-wallet/mnemonic"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin"
	"github.com/howeyc/gopass"
	"github.com/mitchellh/go-homedir"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var localdb *db.LocalDb = nil
var localMnenoic []byte
var passwdValid=true

const NEXT = "next"
const encryptKey = "encryptText"
const repoENV = "LOTUS_WALLET_TOOL_PATH"
const defaultRepoPath = "~/.lotuswallettool"

//const signKeyFile = "sign.txt"

//const filPath = "m/44'/461'/0'/0/"

type FilAddressInfo struct {
	MinerId  string
	AddrType string
	Index    int
	Address  string
}

func getRepoPath() string {
	repoPath := os.Getenv(repoENV)
	if len(repoPath) == 0 {
		repoPath = defaultRepoPath
	}

	repoPath, err := homedir.Expand(repoPath)
	if err != nil {
		fmt.Println("读取程序配置目录失败")
		panic("读取程序配置目录失败")
	}
	return repoPath
}

func _initDb() error {

	_, err := os.Stat(filepath.Join(getRepoPath(), "db"))
	//if err != nil && os.IsNotExist(err) {
	if err != nil {
		os.RemoveAll(filepath.Join(getRepoPath(), "db"))
		err := os.MkdirAll(filepath.Join(getRepoPath(), "db"), 0755)
		if err != nil {
			fmt.Printf("初始化程序创建数据库目录失败！ err:%v\n", err)
			return xerrors.Errorf("初始化程序创建数据库目录失败！ err:%v\n", err)
		}
	}

	localdb, err = db.Init(filepath.Join(getRepoPath(), "db"))
	if err != nil {
		fmt.Printf("初始化程序创建数据库失败！ err:%v\n", err)
	}
	return err
}

func signMessage(msg *types.Message) (*types.SignedMessage, error) {
	faiByte, err := localdb.Get(db.KeyAddr, msg.From.String())
	if err != nil {
		panic(err)
	}

	fai := FilAddressInfo{}
	err = json.Unmarshal(faiByte, &fai)
	if err != nil {
		fmt.Printf("解析数据库信息是失败, err:%v\n", err)
		return &types.SignedMessage{}, err
	}

	signMsg, err := impl.SignMessage(msg, string(localMnenoic), fai.Index)
	if err != nil {
		fmt.Printf("签名失败,err: %v\n", err)
	}

	return signMsg, nil
}

func _init() error {

	if err := _initDb(); err != nil {
		fmt.Printf("初始化DB失败，err: %v\n", err)
		return err
	}

	encryptText, err := localdb.Get(db.KeyCommon, encryptKey)
	if err != nil {
		fmt.Printf("读取化DB失败，err: %v\n", err)
		return err
	}

	//fmt.Println(string(encryptText))
	passwd, err := getPassword()
	if err != nil {
		fmt.Printf("输入密码异常，err: %v\n", err)
		return err
	}

	localMnenoic, err = mnemonic.Decrypt(encryptText, passwd)
	if err != nil {
		fmt.Printf("读取助记词失败，err: %v\n", err)
		return err
	}

	if valid:=impl.VerifyPassword(string(localMnenoic),0) ;!valid{
		return fmt.Errorf("密码错误！")
	}
	return nil
}

func main() {

	local := []*cli.Command{
		initCmd,
		withdrawCmd,
		sendCmd,
		newAddressCmd,
		exportAddressCmd,
		listCmd,
		//signCmd,
	}

	app := &cli.App{
		Name:    "萤火虫钱包管理工具",
		Usage:   "萤火虫钱包管理工具， 用于矿工提现，转账，签名，以及节点控制等功能",
		Version: build.UserVersion(),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "db-dir",
				Value: "./data",
			},
		},
		Commands: local,
	}

	if err := app.Run(os.Args); err != nil {
		os.Exit(1)
	}
}

var exportAddressCmd = &cli.Command{
	Name:  "export-address",
	Usage: "导出钱包私钥",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "address",
			Usage: "导出地址",
		},
	},
	Before: func(context *cli.Context) error {
		if err:=_init();err!=nil{
			passwdValid=false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if !passwdValid{
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}
		address := cctx.String("address")
		if address == "" {
			fmt.Println("address is  empty")
			return nil
		}

		faiByte, err := localdb.Get(db.KeyAddr, address)
		if err != nil {
			panic(err)
		}

		fai := FilAddressInfo{}
		err = json.Unmarshal(faiByte, &fai)
		if err != nil {
			panic(err)
		}

		var privKey string
		if strings.HasPrefix(fai.Address, "f3") || strings.HasPrefix(fai.Address, "t3") {
			privKey, err = impl.ExportBlsAddress(string(localMnenoic), fai.Index)
			if err != nil {
				panic(err)
			}
		} else {
			privKey, err = impl.ExportSecp256k1Address(string(localMnenoic), fai.Index)
			if err != nil {
				panic(err)
			}
		}

		fmt.Println(privKey)
		return nil
	},
}
var sendCmd = &cli.Command{
	Name:  "send",
	Usage: "转账",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "转账源账户",
		},
		&cli.StringFlag{
			Name:  "to",
			Usage: "转账目标账户",
		},
		&cli.StringFlag{
			Name:  "amount",
			Usage: "转账金额",
		},
	},
	Before: func(context *cli.Context) error {
		if err:=_init();err!=nil{
			passwdValid=false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if !passwdValid{
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Printf("连接FULLNODE_API_INFO api失败。%v\n", err)
			return err
		}
		defer closer() //nolint:errcheck

		ctx := lcli.ReqContext(cctx)
		msg := &types.Message{}

		msg.From, err = address.NewFromString(cctx.String("from"))
		if err != nil {
			fmt.Printf("解析转账源地址失败: %v", err)
			return fmt.Errorf("failed to parse source address: %w\n", err)
		}

		msg.To, err = address.NewFromString(cctx.String("to"))
		if err != nil {
			fmt.Printf("解析转账目标地址失败: %v", err)
			return fmt.Errorf("failed to parse target address: %w\n", err)
		}

		amount, err := types.ParseFIL(cctx.String("amount"))
		if err != nil {

			fmt.Printf("解析转账金额地址失败: %v\n", err)
			return fmt.Errorf("解析转账金额失败: %v", err)
		}
		msg.Value = abi.TokenAmount(amount)

		if strings.Compare(msg.From.String(), msg.To.String()) == 0 {

			fmt.Printf("转入和转出地址相同\n")
			return fmt.Errorf("转入和转出地址相同")
		}

		msg.Method = builtin.MethodSend

		// 获取nonce
		a, err := api.StateGetActor(ctx, msg.From, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取获取owner地址的nonce失败，err:%v\n", err)
			return err
		}
		msg.Nonce = a.Nonce

		msg, err = api.GasEstimateMessageGas(ctx, msg, nil, types.EmptyTSK)
		if err != nil {
			fmt.Printf("评估消息的gas费用失败， err:%v\n", err)
			return xerrors.Errorf("GasEstimateMessageGas error: %w", err)
		}

		// 签名
		signMsg, err := signMessage(msg)
		if err != nil {
			return err
		}

		// 推送消息
		cid, err := api.MpoolPush(ctx, signMsg)
		if err != nil {
			fmt.Printf("推送消息上链失败，err:%v\n", err)
			return err
		}

		fmt.Printf("Requested rewards withdrawal in message %s\n", cid.String())

		return nil
	},
}

var withdrawCmd = &cli.Command{
	Name:      "withdraw",
	Usage:     "矿工提现,例如 withdraw f02420 100, 如果不填写提现金额，则提取miner所有余额",
	ArgsUsage: "[minerId (eg f01000) ] [amount (FIL)]",
	Before: func(context *cli.Context) error {
		if err:=_init();err!=nil{
			passwdValid=false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if !passwdValid{
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		/**
		1 获取nonce，
		2 签名，使用本地签名
		3 使用fullnodeapi推送消息
		*/

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
		//fmt.Printf("--->%+v", owner)

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
			fmt.Printf("未指定提现 金额，将miner 所有可用余额（%s）提现，\n", amount)
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
			Method: miner.Methods.WithdrawBalance,
			Nonce:  a.Nonce,
			Params: params,
		}, nil, types.EmptyTSK)
		if err != nil {
			fmt.Printf("评估消息的gas费用失败， err:%v\n", err)
			return xerrors.Errorf("GasEstimateMessageGas error: %w", err)
		}

		fmt.Printf("\n%+v\n", msg)

		// 签名
		signMsg, err := signMessage(msg)
		if err != nil {
			return err
		}

		// 推送消息
		cid, err := api.MpoolPush(ctx, signMsg)
		if err != nil {
			fmt.Printf("推送消息上链失败，err:%v\n", err)
			return err
		}

		fmt.Printf("Requested rewards withdrawal in message %s\n", cid.String())

		return nil
	},
}

var initCmd = &cli.Command{
	Name:  "init",
	Usage: "初始化配置钱包助记词，用于后续签名，第一次使用本工具必须先执行此命令初始化。助记词会通过输入密码加密保存，后面启动无需再次输入助记词。只需输入密码即可。",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "key-file",
			Usage: "指定助记词文件",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "强制执行，当本地存在一个助记词加密文件，不允许再次初始化，指定--force命令，可以强制初始化，将覆盖之前的助记词加密文件。请谨慎操作。",
		},
	},
	Before: func(context *cli.Context) error {
		if err:=_initDb();err!=nil{
			passwdValid=false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if !passwdValid{
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		// 读取助记词
		keyFileBytes, err := ioutil.ReadFile(cctx.String("key-file"))
		if err != nil {
			fmt.Printf("从 %s 读取助记词失败。原因: %v\n", cctx.String("key-file"), err.Error())
			return nil
		}

		if keyExist(localdb) {
			if cctx.Bool("force") {
				fmt.Println("旧的助记词将被新的覆盖掉！")
			} else {
				fmt.Println("已经存在一个助记词，请勿重复初始化, 如果确认想更换助记词，请加--force命令")
				return nil
			}
		}

		// 输入密码
		passwd, err := getPassword()
		if err != nil {
			return nil
		}

		fmt.Print("请再次输入密码：")
		passwdRe, err := gopass.GetPasswdMasked()
		if err != nil {
			fmt.Printf("输入密码异常，%v\n", err)
			return nil
		}

		if bytes.Compare(passwd, passwdRe) != 0 {
			fmt.Printf("两次输入密码不一致，%v\n", err)
			return nil
		}

		// 加密助记词
		if err := encryptAndSaveKey(keyFileBytes, passwd, localdb); err != nil {
			fmt.Printf("加密保存助记词失败！err: %v\n", err)
			return err
		}

		// 初始化创建一个钱包地址,用于后续验证密码使用
		createAddress(false, false)
		return nil
	},
}

var newAddressCmd = &cli.Command{
	Name:  "new-address",
	Usage: "创建一个新的钱包地址",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "bls",
			Usage: "创建bls类型的钱包地址",
			Value: false,
		},
		&cli.BoolFlag{
			Name:   "show-private-key",
			Usage:  "显示私钥",
			Value:  false,
			Hidden: true,
		},
	},
	Before: func(context *cli.Context) error {
		if err:=_init();err!=nil{
			passwdValid=false
		}
		return nil
	},
	Action: func(context *cli.Context) error {
		if !passwdValid{
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		showPK := context.Bool("show-private-key")
		createAddress(showPK, context.Bool("bls"))
		return nil
	},
}


var listCmd = &cli.Command{
	Name:  "list",
	Usage: "展示钱包列表",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "addr-only",
			Usage:   "只展示钱包地址",
			Aliases: []string{"a"},
		},
		&cli.BoolFlag{
			Name:    "id",
			Usage:   "展示actorID",
			Aliases: []string{"i"},
		},
		&cli.BoolFlag{
			Name:    "market",
			Usage:   "展示market余额",
			Aliases: []string{"m"},
		},
	},
	Before: func(context *cli.Context) error {
		if err:=_init();err!=nil{
			passwdValid=false
			//panic(err)
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if !passwdValid{
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		//addrs, err := localWallet.WalletList(ctx)
		addrs,err:=localdb.GetAll(db.KeyAddr)
		if err != nil {
			fmt.Println("读取数据库获取钱包地址失败")
			return err
		}

		// Assume an error means no default key is set
		def, _ := api.WalletDefaultAddress(ctx)

		tw := tablewriter.New(
			tablewriter.Col("Address"),
			tablewriter.Col("ID"),
			tablewriter.Col("Balance"),
			tablewriter.Col("Market(Avail)"),
			tablewriter.Col("Market(Locked)"),
			tablewriter.Col("Nonce"),
			tablewriter.Col("Default"),
			tablewriter.NewLineCol("Error"))

		for _, a := range addrs {
			fa:=FilAddressInfo{}
			json.Unmarshal([]byte(a),&fa)
			addr,err:=address.NewFromString(fa.Address)
			if err!=nil{
				return err
			}

			if cctx.Bool("addr-only") {
				fmt.Println(addr)
			} else {
				a, err := api.StateGetActor(ctx, addr, types.EmptyTSK)
				if err != nil {
					if !strings.Contains(err.Error(), "actor not found") {
						tw.Write(map[string]interface{}{
							"Address": addr,
							"Error":   err,
						})
						continue
					}

					a = &types.Actor{
						Balance: big.Zero(),
					}
				}

				row := map[string]interface{}{
					"Address": addr,
					"Balance": types.FIL(a.Balance),
					"Nonce":   a.Nonce,
				}
				if addr == def {
					row["Default"] = "X"
				}

				if cctx.Bool("id") {
					id, err := api.StateLookupID(ctx, addr, types.EmptyTSK)
					if err != nil {
						row["ID"] = "n/a"
					} else {
						row["ID"] = id
					}
				}

				if cctx.Bool("market") {
					mbal, err := api.StateMarketBalance(ctx, addr, types.EmptyTSK)
					if err == nil {
						row["Market(Avail)"] = types.FIL(types.BigSub(mbal.Escrow, mbal.Locked))
						row["Market(Locked)"] = types.FIL(mbal.Locked)
					}
				}

				tw.Write(row)
			}
		}

		if !cctx.Bool("addr-only") {
			return tw.Flush(os.Stdout)
		}

		return nil
	},
}

func createAddress(show, bls bool) {
	if bls {
		// t3...
		generateBlsFilAddress(show)
	} else {
		// t1....
		generateFilAddress(show)
	}
}
func getNextIndex() int {

	strIndex, err := localdb.Get(db.KeyIndex, NEXT)
	if err != nil {
		if err != errors.ErrNotFound {
			panic(err)
		}
		strIndex = []byte("0")
	}

	index, err := strconv.Atoi(string(strIndex))
	if err != nil {
		panic(err)
	}

	return index
}

func generateBlsFilAddress(showPK bool) {
	index := getNextIndex()
	filAddr, err := impl.CreateBlsFilAddress(string(localMnenoic), index)
	if err != nil {
		panic(err)
	}

	fai := FilAddressInfo{
		Address: filAddr,
		Index:   index,
	}

	//fmt.Println(fai)

	if showPK {
		priKey, err := impl.ExportBlsAddress(string(localMnenoic), index)
		if err != nil {
			panic(err)
		}
		fmt.Println(priKey)
	}

	faiByte, err := json.Marshal(&fai)
	fmt.Println(filAddr)
	if err != nil {
		panic(err)
	}

	err = localdb.Add(db.KeyAddr, filAddr, faiByte)
	if err != nil {
		panic(err)
	}

	err = localdb.Add(db.KeyIndex, NEXT, []byte(fmt.Sprintf("%d", index+1)))
	if err != nil {
		panic(err)
	}
}

func generateFilAddress(showPK bool) {
	mnenoic := string(localMnenoic)
	index := getNextIndex()
	filAddr, err := impl.CreateSecp256k1FilAddress(mnenoic, index)
	if err != nil {
		panic(err)
	}

	fai := FilAddressInfo{
		Address: filAddr,
		Index:   index,
	}

	fmt.Println(filAddr)

	if showPK {
		priKey, err := impl.ExportSecp256k1Address(mnenoic, index)
		if err != nil {
			panic(err)
		}
		fmt.Println(priKey)
	}

	faiByte, err := json.Marshal(&fai)
	if err != nil {
		panic(err)
	}

	err = localdb.Add(db.KeyAddr, filAddr, faiByte)
	if err != nil {
		panic(err)
	}

	err = localdb.Add(db.KeyIndex, NEXT, []byte(fmt.Sprintf("%d", index+1)))
	if err != nil {
		panic(err)
	}
}

func getPassword() ([]byte, error) {
	var passwd []byte
	var err error
	fmt.Print("请输入密码(长度至少6位):")
	for tryTime := 3; tryTime > 0; tryTime-- {
		passwd, err = gopass.GetPasswdMasked()
		if err != nil {
			fmt.Printf("输入密码异常，%v\n", err)
			continue
		}
		if len(passwd) < 6 {
			fmt.Printf("输入密码不合法: %v\n", "长度太短")
			if tryTime == 1 {
				fmt.Println("重试次数超过3次")
				return []byte{}, xerrors.Errorf("重试次数超过3次")
			}
			continue
		}
		break
	}

	return passwd, nil
}

func encryptAndSaveKey(mne, pass []byte, localdb *db.LocalDb) error {
	encryptData, err := mnemonic.EncryptData(mne, pass)
	if err != nil {
		return err
	}
	fmt.Println(string(encryptData))
	return localdb.Add(db.KeyCommon, encryptKey, encryptData)
}

func keyExist(localdb *db.LocalDb) bool {

	_, err := localdb.Get(db.KeyCommon, encryptKey)
	if err != nil {
		//fmt.Printf("读取化DB失败，err: %v\n", err)
		return false
	}

	//encryptData, err := mnemonic.EncryptData(mne, pass)
	//if err != nil {
	//	panic(err)
	//}

	//if bytes.Compare(encryptData, encryptText) == 0 {
	//	return true
	//}

	return true
}
