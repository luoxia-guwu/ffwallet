package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	crypto2 "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/blake2b"
	"github.com/filecoin-project/firefly-wallet/db"
	"github.com/filecoin-project/firefly-wallet/impl"
	"github.com/filecoin-project/firefly-wallet/mnemonic"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
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
var passwdValid = true
var passwd []byte

const NEXT = "next"
const encryptKey = "encryptText"
const repoENV = "LOTUS_WALLET_TOOL_PATH"
const defaultRepoPath = "~/.lotuswallettool"
const unRecoverIndex = -1 // 导入钱包地址index为0。

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
		return &types.SignedMessage{}, err
		//panic(err)
	}

	fai := FilAddressInfo{}
	err = json.Unmarshal(faiByte, &fai)
	if err != nil {
		fmt.Printf("解析数据库信息是失败, err:%v\n", err)
		return &types.SignedMessage{}, err
	}

	// 导入钱包地址签名
	if fai.Index == unRecoverIndex {
		signMsg, err := unRecoverAddrSignMessage(msg, types.KeyType(fai.AddrType))
		if err != nil {
			fmt.Printf("签名失败,err: %v\n", err)
		}

		return signMsg, nil
	} else {
		// 派生钱包地址签名
		signMsg, err := impl.SignMessage(msg, string(localMnenoic), fai.Index)
		if err != nil {
			fmt.Printf("签名失败,err: %v\n", err)
		}

		return signMsg, nil
	}

}

// 非派生钱包地址，即不可恢复钱包地址，签名
func unRecoverAddrSignMessage(msg *types.Message, keyType types.KeyType) (*types.SignedMessage, error) {
	encryptKey, err := localdb.Get(db.KeyPriKey, msg.From.String())
	if err != nil {
		fmt.Printf("从数据库读取钱包失败！,err: %v", err)
		return &types.SignedMessage{}, err
	}

	inpdata, err := mnemonic.Decrypt(encryptKey, passwd)
	if err != nil {
		fmt.Printf("读取私钥出错,err: %v", err)
		return &types.SignedMessage{}, err
	}

	var ki types.KeyInfo
	data, err := hex.DecodeString(strings.TrimSpace(string(inpdata)))
	if err != nil {
		fmt.Println("输入的私钥格式不正确，解析出错！")
		return &types.SignedMessage{}, err
	}

	if err := json.Unmarshal(data, &ki); err != nil {
		fmt.Println("输入的私钥格式不正确，序列化出错！")
		return &types.SignedMessage{}, err
	}

	mb, err := msg.ToStorageBlock()
	if err != nil {
		fmt.Printf("序列化消息失败， err:%v", err)
		return &types.SignedMessage{}, xerrors.Errorf("serializing message: %w", err)
	}

	var sb *crypto.Signature
	switch ki.Type {
	case types.KTSecp256k1:
		b2sum := blake2b.Sum256(mb.Cid().Bytes())

		priKey, err := crypto2.ToECDSA(ki.PrivateKey)
		if err != nil {
			return nil, err
		}
		sig, err := crypto2.Sign(b2sum[:], priKey)
		if err != nil {
			fmt.Printf("签名消息失败，err:%v", err)
			return &types.SignedMessage{}, err
		}

		sb = &crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: sig,
		}
	case types.KTBLS:
		sb, err = impl.SignBls(ki.PrivateKey[:], mb.Cid().Bytes())
		if err != nil {
			fmt.Printf("签名消息失败，err:%v", err)
			return &types.SignedMessage{}, err
		}
	default:
		return nil, xerrors.Errorf("unsupported key type: %s", ki.Type)
	}

	return &types.SignedMessage{
		Message:   *msg,
		Signature: *sb,
	}, nil
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
	passwd, err = getPassword()
	if err != nil {
		fmt.Printf("输入密码异常，err: %v\n", err)
		return err
	}

	localMnenoic, err = mnemonic.Decrypt(encryptText, passwd)
	if err != nil {
		fmt.Printf("读取助记词失败，err: %v\n", err)
		return err
	}

	if valid := impl.VerifyPassword(string(localMnenoic), 0); !valid {
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
		importAddressCmd,
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
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if !passwdValid || len(passwd) < 1 {
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
			fmt.Printf("从数据库读取钱包失败！,err: %v", err)
			return nil
		}

		fai := FilAddressInfo{}
		err = json.Unmarshal(faiByte, &fai)
		if err != nil {
			fmt.Printf("反序列化数据(%v)失败！,err: %v", faiByte, err)
			return nil
		}

		var privKey string
		if fai.Index == unRecoverIndex {
			encryptKey, err := localdb.Get(db.KeyPriKey, address)
			if err != nil {
				fmt.Printf("从数据库读取钱包失败！,err: %v", err)
				return nil
			}

			inpdata, err := mnemonic.Decrypt(encryptKey, passwd)
			if err != nil {
				fmt.Printf("读取私钥出错,err: %v", err)
				return nil
			}

			var ki types.KeyInfo
			data, err := hex.DecodeString(strings.TrimSpace(string(inpdata)))
			if err != nil {
				fmt.Println("输入的私钥格式不正确，解析出错！")
				return err
			}

			if err := json.Unmarshal(data, &ki); err != nil {
				fmt.Println("输入的私钥格式不正确，序列化出错！")
				return err
			}

			b, err := json.Marshal(ki)
			if err != nil {
				fmt.Println("序列化私钥出错，原因:", err.Error())
				return err
			}

			privKey = hex.EncodeToString(b)

		} else {
			if strings.HasPrefix(fai.Address, "f3") || strings.HasPrefix(fai.Address, "t3") {
				privKey, err = impl.ExportBlsAddress(string(localMnenoic), fai.Index)
				if err != nil {
					fmt.Printf("导出BLS钱包失败！,err: %v", err)
					return nil
				}
			} else {
				privKey, err = impl.ExportSecp256k1Address(string(localMnenoic), fai.Index)
				if err != nil {
					fmt.Printf("导出Secp256钱包失败！,err: %v", err)
					return nil
				}
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

		if cctx.String("from") == "" || cctx.String("to") == "" || cctx.String("amount") == "" {

			fmt.Println("必须指定--from，--to，--amount.")
			return nil
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
			fmt.Printf("签名失败， err:%v\n", err)
			return xerrors.Errorf("签名失败: %w", err)
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
		if err := _initDb(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if !passwdValid {
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

		encryptText, err := localdb.Get(db.KeyCommon, encryptKey)
		if err != nil {
			fmt.Printf("读取化DB失败，err: %v\n", err)
			return err
		}

		localMnenoic, err = mnemonic.Decrypt(encryptText, passwd)
		if err != nil {
			fmt.Printf("读取助记词失败，err: %v\n", err)
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

		showPK := context.Bool("show-private-key")
		createAddress(showPK, context.Bool("bls"))
		return nil
	},
}

var importAddressCmd = &cli.Command{
	Name:      "import",
	Usage:     "导入钱包地址，注意：导入的钱包地址请因不是助记词派生的地址，无法通过助记词找回来。",
	ArgsUsage: "[<path> (optional, will read from stdin if omitted)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "format",
			Usage: "specify input format for key",
			Value: "hex-lotus",
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		if !passwdValid || len(passwd) < 1 {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		var inpdata []byte
		if !cctx.Args().Present() || cctx.Args().First() == "-" {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("请输入私钥: ")
			indata, err := reader.ReadBytes('\n')
			if err != nil {
				return err
			}
			inpdata = indata

		} else {
			fdata, err := ioutil.ReadFile(cctx.Args().First())
			if err != nil {
				return err
			}
			inpdata = fdata
		}

		var ki types.KeyInfo
		switch cctx.String("format") {
		case "hex-lotus":
			data, err := hex.DecodeString(strings.TrimSpace(string(inpdata)))
			if err != nil {
				fmt.Println("输入的私钥格式不正确，解析出错！")
				return err
			}

			if err := json.Unmarshal(data, &ki); err != nil {
				fmt.Println("输入的私钥格式不正确，序列化出错！")
				return err
			}
		case "json-lotus":
			if err := json.Unmarshal(inpdata, &ki); err != nil {
				fmt.Println("输入的json格式的私钥格式不正确，序列化出错！")
				return err
			}
		case "gfc-json":
			var f struct {
				KeyInfo []struct {
					PrivateKey []byte
					SigType    int
				}
			}
			if err := json.Unmarshal(inpdata, &f); err != nil {
				fmt.Println("输入的gfc格式的私钥格式不正确，序列化出错！")
				return xerrors.Errorf("failed to parse go-filecoin key: %s", err)
			}

			gk := f.KeyInfo[0]
			ki.PrivateKey = gk.PrivateKey
			switch gk.SigType {
			case 1:
				ki.Type = types.KTSecp256k1
			case 2:
				ki.Type = types.KTBLS
			default:
				fmt.Println("解析私钥 信息失败，无法识别的私钥类型!!")
				return fmt.Errorf("unrecognized key type: %d", gk.SigType)
			}
		default:
			fmt.Println("解析私钥 信息失败，不识别的格式!!")
			return fmt.Errorf("unrecognized format: %s", cctx.String("format"))
		}

		//api, closer, err := lcli.GetFullNodeAPI(cctx)
		//if err != nil {
		//	fmt.Printf("连接FULLNODE_API_INFO api失败。%v\n", err)
		//	return err
		//}
		//defer closer()
		//ctx := lcli.ReqContext(cctx)

		// 解析出addr
		key, err := impl.NewKey(&ki)
		if err != nil {
			fmt.Println("创建钱包地址失败 ,原因：", err.Error())
			return err
		}

		filInfo := FilAddressInfo{Address: key.Address.String(), Index: unRecoverIndex, AddrType: string(key.Type)}
		filInfoByte, err := json.Marshal(&filInfo)
		if err != nil {
			fmt.Println("序列化filInfo失败!")
			return err
		}

		err = localdb.Add(db.KeyAddr, key.Address.String(), filInfoByte)
		if err != nil {
			fmt.Println("保存钱包地址到数据异常，原因：", err.Error())
			return err
		}

		// 保存privateKey,到数据库
		encryData, err := mnemonic.EncryptData(inpdata, passwd)
		if err != nil {
			fmt.Println("加密私钥失败！原因：", err.Error())
			return err
		}

		err = localdb.Add(db.KeyPriKey, key.Address.String(), encryData)
		if err != nil {
			fmt.Println("保存钱包地址到数据异常，原因：", err.Error())
			return err
		}

		fmt.Println("成功导入钱包：", key.Address.String())
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
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Printf("连接FULLNODE_API_INFO api失败。%v\n", err)
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		//addrs, err := localWallet.WalletList(ctx)
		addrs, err := localdb.GetAll(db.KeyAddr)
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
			fa := FilAddressInfo{}
			json.Unmarshal([]byte(a), &fa)
			addr, err := address.NewFromString(fa.Address)
			if err != nil {
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
