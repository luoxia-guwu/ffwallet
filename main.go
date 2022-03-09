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
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
	"github.com/howeyc/gopass"
	"github.com/ipfs/go-cid"
	"github.com/mitchellh/go-homedir"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

func signMessage(msg []byte, addr address.Address) (*crypto.Signature, error) {
	faiByte, err := localdb.Get(db.KeyAddr, addr.String())
	if err != nil {
		fmt.Println("db中读取钱包地址", addr.String())
		return &crypto.Signature{}, err
		//panic(err)
	}

	fai := FilAddressInfo{}
	err = json.Unmarshal(faiByte, &fai)
	if err != nil {
		fmt.Printf("解析数据库信息是失败, err:%v\n", err)
		return &crypto.Signature{}, err
	}

	//mb, err := msg.ToStorageBlock()
	//if err != nil {
	//	fmt.Printf("序列化消息失败， err:%v", err)
	//	return &crypto.Signature{}, xerrors.Errorf("serializing message: %w", err)
	//}

	// 导入钱包地址签名
	if fai.Index == unRecoverIndex {
		sb, err := unRecoverAddrSign(msg, addr)
		if err != nil {
			fmt.Printf("签名失败,err: %v\n", err)
		}

		return sb, nil
	} else {
		// 派生钱包地址签名
		sb, err := impl.Sign(msg, addr, string(localMnenoic), fai.Index)
		if err != nil {
			fmt.Printf("签名失败,err: %v\n", err)
		}

		return sb, nil
	}
}

// 非派生钱包地址，即不可恢复钱包地址，签名
func unRecoverAddrSign(msg []byte, addr address.Address) (*crypto.Signature, error) {
	encryptKey, err := localdb.Get(db.KeyPriKey, addr.String())
	if err != nil {
		fmt.Printf("从数据库读取钱包失败！,err: %v", err)
		return &crypto.Signature{}, err
	}

	inpdata, err := mnemonic.Decrypt(encryptKey, passwd)
	if err != nil {
		fmt.Printf("读取私钥出错,err: %v", err)
		return &crypto.Signature{}, err
	}

	var ki types.KeyInfo
	data, err := hex.DecodeString(strings.TrimSpace(string(inpdata)))
	if err != nil {
		fmt.Println("输入的私钥格式不正确，解析出错！")
		return &crypto.Signature{}, err
	}

	if err := json.Unmarshal(data, &ki); err != nil {
		fmt.Println("输入的私钥格式不正确，序列化出错！")
		return &crypto.Signature{}, err
	}

	var sb *crypto.Signature
	switch ki.Type {
	case types.KTSecp256k1:
		b2sum := blake2b.Sum256(msg)

		priKey, err := crypto2.ToECDSA(ki.PrivateKey)
		if err != nil {
			return nil, err
		}
		sig, err := crypto2.Sign(b2sum[:], priKey)
		if err != nil {
			fmt.Printf("签名消息失败，err:%v", err)
			return &crypto.Signature{}, err
		}

		sb = &crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: sig,
		}
	case types.KTBLS:
		sb, err = impl.SignBls(ki.PrivateKey[:], msg)
		if err != nil {
			fmt.Printf("签名消息失败，err:%v", err)
			return &crypto.Signature{}, err
		}
	default:
		return nil, xerrors.Errorf("unsupported key type: %s", ki.Type)
	}

	return sb, nil
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
		signCmd,
		setOwnerCmd,
		proposeChangeWorker,
		localdbCmd,
		controlListCmd,
		controlSetCmd,
		toolsCmd,
		walletBalance,
		walletSendBatchCmd,
		batchGenerateKeyCmd,
		msigCmd,
		sectorsCmd,
	}

	app := &cli.App{
		Name:      "萤火虫钱包管理工具",
		Usage:     "萤火虫钱包管理工具， 用于矿工提现，转账，签名，以及节点控制等功能",
		UsageText: "通过环境变量 LOTUS_WALLET_TOOL_PATH 设置程序执行路径, 默认路径: ~/.lotuswallettool。 与链交互需要配置 FULLNODE_API_INFO 环境变量",
		Version:   build.UserVersion(),
		//Flags: []cli.Flag{
		//	&cli.StringFlag{
		//		Name:  "db-dir",
		//		Value: "./data",
		//	},
		//},
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

		msg.Method = 0

		// 获取nonce
		msg.Nonce, err = api.MpoolGetNonce(ctx, msg.From)
		if err != nil {
			fmt.Printf("读取获取消息池中的的nonce失败，err:%v\n", err)
			a, err := api.StateGetActor(ctx, msg.From, types.EmptyTSK)
			if err != nil {
				fmt.Printf("读取获取链数据上的的nonce失败，err:%v\n", err)
				return err
			}
			msg.Nonce = a.Nonce
		}

		msg, err = api.GasEstimateMessageGas(ctx, msg, nil, types.EmptyTSK)
		if err != nil {
			fmt.Printf("评估消息的gas费用失败， err:%v\n", err)
			return xerrors.Errorf("GasEstimateMessageGas error: %w", err)
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
		cid, err := api.MpoolPush(ctx, &types.SignedMessage{Message: *msg, Signature: *sb})
		if err != nil {
			fmt.Printf("推送消息上链失败，err:%v\n", err)
			return err
		}

		fmt.Printf("转账消息id： %s\n", cid.String())

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
		createAddress(false, false, "", "")
		return nil
	},
}

var newAddressCmd = &cli.Command{
	Name:  "new-address",
	Usage: "创建一个新的钱包地址,默认创建spec256类型的钱包地址",
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
		&cli.StringFlag{
			Name:  "miner-id",
			Usage: "所属miner",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "type",
			Usage: "指定钱包地址类型，owner，post，worker，此参数必须再指定miner-id的前提下才有效",
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

		miner := context.String("miner-id")
		if len(miner) > 0 && !strings.HasPrefix(miner, "f0") {
			fmt.Printf("minerid(%s) 输入不合法,请输入一个有效的minerid\n", miner)
			return fmt.Errorf("minerid(%s) 输入不合法,请输入一个有效的minerid\n", miner)
		}

		addrType := ""
		if len(miner) > 0 {
			addrType = context.String("type")
			if strings.Compare(addrType, string(db.PostAddr)) == 0 || strings.Compare(addrType, string(db.OwnerAddr)) == 0 || strings.Compare(addrType, string(db.WorkerAddr)) == 0 {
			} else {
				fmt.Errorf("type(%s) 类型必须为owner、post、worker", addrType)
				addrType = ""
			}
		}

		showPK := context.Bool("show-private-key")
		createAddress(showPK, context.Bool("bls"), miner, addrType)
		return nil
	},
}

var importAddressCmd = &cli.Command{
	Name:      "import",
	Usage:     "导入钱包地址，注意：导入的钱包地址,因不是助记词派生的地址，无法通过助记词找回来。",
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

var walletBalance = &cli.Command{
	Name:      "balance",
	Usage:     "Get account balance",
	ArgsUsage: "[address]",
	//Before: func(context *cli.Context) error {
	//	if err := _init(); err != nil {
	//		passwdValid = false
	//	}
	//	return nil
	//},
	Action: func(cctx *cli.Context) error {

		//if !passwdValid {
		//	fmt.Println("密码错误.")
		//	return fmt.Errorf("密码错误")
		//}
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Printf("连接FULLNODE_API_INFO api失败。%v\n", err)
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		var addr address.Address
		if cctx.Args().First() != "" {
			addr, err = address.NewFromString(cctx.Args().First())
		} else {
			addr, err = api.WalletDefaultAddress(ctx)
		}
		if err != nil {
			fmt.Println(err)
			return err
		}

		balance, err := api.WalletBalance(ctx, addr)
		if err != nil {
			fmt.Println(err)
			return err
		}

		if balance.Equals(types.NewInt(0)) {
			fmt.Printf("%s (warning: may display 0 if chain sync in progress)\n", types.FIL(balance))
		} else {
			fmt.Printf("%s   %s\n", addr.String(), types.FIL(balance))
		}

		return nil
	},
}

// 主网
//var outAddrStr = "f1dnyrev6qjndjioanm3gmzkgavs2u5xxzvs5uwhq"
//var inAddrStr = "f13anz7poviwzw765bvjyngvtnys3xm5pbhqe4jna"

// 出账账户
//var outAddrStr = "f12ijdsnibjsvxhticyy3ybk64j3nde6beoc4gawq"
//
//// 总账户
//var inAddrStr = "f1y2y6srzoihm6dr4fahh27exrctc52l6r4xmuwqy"

// 出账账户
var outAddrStr = "f1jfx7m2qigjdf7urjapadrzbkakkpn53tobp34hq"

// 总账户
var inAddrStr = "f1tqj3wdoxlrspmoyqdkaokq25wleilq347enpbtq"

var walletSendBatchCmd = &cli.Command{
	Name:      "send-batch",
	Usage:     "send batch amount file",
	ArgsUsage: "[batch-file]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:   "from",
			Usage:  "from",
			Hidden: true,
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		//from := cctx.String("from")
		//if from == "" {
		//	from = outAddrStr
		//	fmt.Printf("from address is empty, use default outAddr(%s)\n", outAddrStr)
		//}

		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		outAddr, err := address.NewFromString(outAddrStr)
		if err != nil {
			fmt.Println("outAddrStr invalid. ", outAddrStr)
			return err
		}

		inAddr, err := address.NewFromString(inAddrStr)
		if err != nil {
			fmt.Println("inAddrStr address invalid.  ", inAddrStr)
			return err
		}

		fromAddr := outAddr

		batchSendFile := cctx.Args().First()
		fmt.Println(batchSendFile)
		if !strings.Contains(batchSendFile, "withdraw.list") {
			fmt.Println("invalid batch file")
			return nil
		}

		batchSendBytes, err := ioutil.ReadFile(batchSendFile)
		if err != nil {
			fmt.Printf("open and read file(%s) failed. err:%v\n", "./t", err)
			return err
		}

		//fmt.Printf("%v\n",[]byte("\n"))
		type Trans struct {
			To     string
			Amount string
		}

		var willTrans []Trans
		var totalAmount float64
		items := bytes.Split(batchSendBytes, []byte{10})
		for count, item := range items {
			//fmt.Println(item)
			subItem := bytes.Split(item, []byte{9})
			if len(subItem) == 2 {
				trans := Trans{To: string(subItem[1]), Amount: string(subItem[0])}
				willTrans = append(willTrans, trans)
				fmt.Printf("%d ./firefly-wallet send --from %s --to %s --amount %s\n", count+1, fromAddr.String(), trans.To, trans.Amount)

				amount, err := strconv.ParseFloat(trans.Amount, 64)
				if err != nil {
					fmt.Println("parse ", trans.Amount, " failed.")
					return nil
				}
				totalAmount += amount
			}
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		// 查询outAddr余额是否足够支付totalAmount+手续费，够则直接支付
		// 不够，则查询inAddr余额是否足够支付totalAmount，如果够，则算出手续费，+totalAmount并转帐到outAddr中去，然后下发
		// 不够，则退出提示归集资金
		outAddrBalance, err := api.WalletBalance(ctx, outAddr)
		if err != nil {
			fmt.Println(outAddr, " get balance failed. err: ", err)
			return err
		}

		fmt.Printf("\n出账账户 %s 余额： %s\n", outAddrStr, types.FIL(outAddrBalance).Short())
		fmt.Printf("总共转账 %d 笔， 共计： %f FIL\n\n", len(willTrans), totalAmount)

		// 转账手续费,按照每笔0.01fil计算
		transFee := float64(len(willTrans)+1) * 0.01
		outBalanceFloat, err := strconv.ParseFloat(types.FIL(outAddrBalance).Unitless(), 64)
		if err != nil {
			fmt.Println(outAddrBalance, " parse float. err: ", err)
			return err
		}

		if outBalanceFloat-transFee < totalAmount {
			// outAddr余额不足

			inAddrBalance, err := api.WalletBalance(ctx, inAddr)
			if err != nil {
				fmt.Println(inAddr, " get balance failed. err: ", err)
				return err
			}

			inBalanceFloat, err := strconv.ParseFloat(types.FIL(inAddrBalance).Unitless(), 64)
			if err != nil {
				fmt.Println(inAddrBalance, " parse float. err: ", err)
				return err
			}

			fmt.Printf("总账户 %s 余额： %f\n", inAddrStr, inBalanceFloat)
			fmt.Printf("出账账户 %s 余额： %f\n", outAddrStr, outBalanceFloat)

			if inBalanceFloat+outBalanceFloat-transFee < totalAmount {
				// 总账户+出账账户+需要手续费不足以提现，退出
				fmt.Printf("inAddr(%f) + outAddr(%f) - transFee(%f) = %f 小于提现金额 %f。 请归集资金再来！\n", inBalanceFloat, outBalanceFloat, transFee, inBalanceFloat+outBalanceFloat-transFee, totalAmount)
				return nil
			}

			// 总账户+出账账户足以支持提现，那么从总账户转入差额，再提现
			fmt.Printf("提现总金额（提现金额+手续费）: %f ，从 总账户 转入 %f 到出账账户。\n", totalAmount+transFee, totalAmount+transFee-outBalanceFloat+0.02)
			fmt.Printf("./firefly-wallet send --from %s %s %f\n", inAddr, outAddr, totalAmount+transFee-outBalanceFloat+0.02)

			var sure string
			fmt.Printf("请核对转账信息：Y/n : ")
			_, err = fmt.Scanf("%s", &sure)
			if err != nil {
				fmt.Println("获取终端输入异常，err: ", err)
				return nil
			}
			if sure != "Y" {
				fmt.Println("取消转账")
				return nil
			}

			var resure string
			fmt.Printf("请核对转账信息：yes/n : ")
			_, err = fmt.Scanf("%s", &resure)
			if err != nil {
				fmt.Println("获取终端输入异常，err: ", err)
				return nil
			}
			if resure != "yes" {
				fmt.Println("取消转账")
				return nil
			}

			cid, err := send(cctx, api, inAddr, outAddr.String(), fmt.Sprintf("%f", totalAmount+transFee-outBalanceFloat+0.02), 0)
			if err != nil {
				fmt.Println("transfor err: \n", err)
				return nil
			}
			fmt.Println(cid.String())
			ticker := time.NewTicker(time.Minute * 3)
			canTransFil := false
			for {
				select {
				case <-time.After(time.Second * 5):
					outAddrBalanceTmp, err := api.WalletBalance(ctx, outAddr)
					if err != nil {
						fmt.Println(outAddr, " get balance failed. err: ", err)
						return err
					}
					outBalanceFloatTmp, err := strconv.ParseFloat(types.FIL(outAddrBalanceTmp).Unitless(), 64)
					if err != nil {
						fmt.Println(outAddrBalance, " parse float. err: ", err)
						return err
					}
					if outBalanceFloatTmp-transFee >= totalAmount {
						fmt.Printf("\n出账账户 %s 余额： %s 足以支付提现金额: %f \n", outAddrStr, types.FIL(outAddrBalanceTmp).Short(), totalAmount)
						canTransFil = true
						break
					}

				case <-ticker.C:
					fmt.Printf("从总账户(%s) 转入 %f 到出账账户（%s） 超过2min还未到账，程序退出请手动确认转账是否成功  \n", inAddrStr, totalAmount+transFee-outBalanceFloat+0.02, outAddrStr)
					return nil
				}

				if canTransFil {
					break
				}
			}
		}

		var sure string
		fmt.Printf("请核对转账信息：Y/n : ")
		_, err = fmt.Scanf("%s", &sure)
		if err != nil {
			fmt.Println("获取终端输入异常，err: ", err)
			return nil
		}
		if sure != "Y" {
			fmt.Println("取消转账")
			return nil
		}

		var resure string
		fmt.Printf("请核对转账信息：yes/n : ")
		_, err = fmt.Scanf("%s", &resure)
		if err != nil {
			fmt.Println("获取终端输入异常，err: ", err)
			return nil
		}
		if resure != "yes" {
			fmt.Println("取消转账")
			return nil
		}

		a, err := api.StateGetActor(ctx, fromAddr, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取获取msg.From地址的nonce失败，err:%v\n", err)
			return err
		}

		//删除提现文件数据
		defer os.Remove(batchSendFile)
		var successCount int
		var successAmount float64
		nonce := a.Nonce
		for count, trans := range willTrans {
			amount, err := strconv.ParseFloat(trans.Amount, 64)
			if err != nil {
				fmt.Println("parse ", trans.Amount, " failed.")
				break
			}
			fmt.Printf("%d ./firefly-wallet send --from %s %s %s", count+1, fromAddr.String(), trans.To, trans.Amount)

			cid, err := send(cctx, api, fromAddr, trans.To, trans.Amount, nonce)
			if err != nil {
				if strings.Compare("转入和转出地址相同", err.Error()) == 0 {
					continue
				}

				fmt.Println("transfor err: \n", err)
				break
			}
			fmt.Println(cid.String())
			nonce++
			successCount++
			successAmount += amount
		}

		fmt.Printf("计划转 %d 笔， 共计 %f FIL\n实际完成 %d 笔，共计 %f FIL, 失败 %d 笔\n", len(willTrans), totalAmount, successCount, successAmount, len(willTrans)-successCount)
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

var signCmd = &cli.Command{
	Name:      "sign",
	Usage:     "签名消息命令",
	ArgsUsage: "<signing address> <hexMessage>",
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(cctx *cli.Context) error {
		//api, closer, err := GetFullNodeAPI(cctx)
		//if err != nil {
		//	return err
		//}
		//defer closer()
		//ctx := ReqContext(cctx)
		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		if !cctx.Args().Present() || cctx.NArg() != 2 {
			fmt.Println("必须指定签名钱包地址和要签名的消息")
			return fmt.Errorf("必须指定签名钱包地址和要签名的消息")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			fmt.Println("输入签名的钱包地址异常,", err)
			return err
		}

		msg, err := hex.DecodeString(cctx.Args().Get(1))
		if err != nil {
			fmt.Println("解析要签名的内容异常,", err)
			return err
		}

		sig, err := signMessage(msg, addr)
		if err != nil {
			return err
		}

		sigBytes := append([]byte{byte(sig.Type)}, sig.Data...)

		fmt.Println(hex.EncodeToString(sigBytes))
		return nil
	},
}

var batchGenerateKeyCmd = &cli.Command{
	Name:  "batch-gen-key",
	Usage: "批量创建钱包地址,并将私钥写入./tmpfile.txt",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show-private-key",
			Value: false,
			Usage: "show private key",
		},
		&cli.BoolFlag{
			Name:  "bls",
			Usage: "bls address",
			Value: false,
		},
		&cli.IntFlag{
			Name:  "num",
			Usage: "number of want create",
			Value: 1,
		},
		&cli.BoolFlag{
			Name:  "save-private-key",
			Usage: "指定这个命令则写私钥到./tmpfile.txt文件中，否则不写入",
			Value: false,
		},
	},
	Before: func(context *cli.Context) error {
		if err := _init(); err != nil {
			passwdValid = false
		}
		return nil
	},
	Action: func(context *cli.Context) error {

		num := context.Int("num")
		if num <= 0 {
			fmt.Printf("num(%d) must > 0\n", num)
			return nil
		}

		if !passwdValid {
			fmt.Println("密码错误.")
			return fmt.Errorf("密码错误")
		}

		showPK := context.Bool("show-private-key")

		f, err := os.OpenFile("./tmpfile.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			fmt.Println(err.Error())
			return nil
		}

		defer func(f *os.File) {
			err = f.Close()
			if err != nil {
				fmt.Println(err.Error())
			}
		}(f)

		for i := 0; i < num; i++ {
			fmt.Printf("%d ------------>>>>>>>>>>>>\n", i)
			priKey := createAddress(showPK, context.Bool("bls"), "", "")

			if context.Bool("save-private-key") {
				_, err = f.WriteString(fmt.Sprintf("%s\n", priKey))
				if err != nil {
					fmt.Println(err.Error())
					return nil
				}
			}
		}
		return nil
	},
}

func createAddress(show, bls bool, miner, addrType string) string {
	priKey := ""
	if bls {
		// t3...
		priKey = generateBlsFilAddress(show, miner, addrType)
	} else {
		// t1....
		priKey = generateFilAddress(show, miner, addrType)
	}
	return priKey
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

func generateBlsFilAddress(showPK bool, miner, addrType string) string {
	index := getNextIndex()
	filAddr, err := impl.CreateBlsFilAddress(string(localMnenoic), index)
	if err != nil {
		panic(err)
	}

	fai := FilAddressInfo{
		Address:  filAddr,
		Index:    index,
		MinerId:  miner,
		AddrType: addrType,
	}

	fmt.Println(fai)

	priKey, err := impl.ExportBlsAddress(string(localMnenoic), index)
	if err != nil {
		panic(err)
	}

	if showPK {
		fmt.Println(priKey)
	}

	faiByte, err := json.Marshal(&fai)
	//fmt.Println(filAddr)
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
	return priKey
}

func generateFilAddress(showPK bool, miner, addrType string) string {
	mnenoic := string(localMnenoic)
	index := getNextIndex()
	filAddr, err := impl.CreateSecp256k1FilAddress(mnenoic, index)
	if err != nil {
		panic(err)
	}

	fai := FilAddressInfo{
		Address:  filAddr,
		Index:    index,
		MinerId:  miner,
		AddrType: addrType,
	}

	//fmt.Println(filAddr)
	fmt.Println(fai)

	priKey, err := impl.ExportSecp256k1Address(mnenoic, index)
	if err != nil {
		panic(err)
	}

	if showPK {
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

	return priKey
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

func send(cctx *cli.Context, api v0api.FullNode, from address.Address, to, amount string, nonce uint64) (cid.Cid, error) {
	ctx := lcli.ReqContext(cctx)
	msg := &types.Message{}
	var err error
	msg.To, err = address.NewFromString(to)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("failed to parse target address: %w", err)
	}

	val, err := types.ParseFIL(amount)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("failed to parse amount: %w", err)
	}
	msg.Value = abi.TokenAmount(val)

	msg.From = from

	if strings.Compare(msg.From.String(), msg.To.String()) == 0 {
		return cid.Cid{}, fmt.Errorf("转入和转出地址相同")
	}

	if nonce != 0 {
		msg.Nonce = nonce
	} else {
		// 获取nonce
		a, err := api.StateGetActor(ctx, msg.From, types.EmptyTSK)
		if err != nil {
			fmt.Printf("读取获取msg.From地址的nonce失败，err:%v\n", err)
			return cid.Cid{}, err
		}
		msg.Nonce = a.Nonce
	}

	//msgCid, err := srv.Send(ctx, params)
	msg, err = api.GasEstimateMessageGas(ctx, msg, nil, types.EmptyTSK)
	if err != nil {
		fmt.Printf("评估消息的gas费用失败， err:%v\n", err)
		return cid.Cid{}, xerrors.Errorf("GasEstimateMessageGas error: %w", err)
	}

	// 签名
	signMsg, err := signMessage(msg.Cid().Bytes(), msg.From)
	if err != nil {
		return cid.Cid{}, err
	}

	// 推送消息

	ccid, err := api.MpoolPush(ctx, &types.SignedMessage{Message: *msg, Signature: *signMsg})
	if err != nil {
		fmt.Printf("推送消息上链失败，err:%v\n", err)
		return cid.Cid{}, err
	}
	return ccid, nil
}
