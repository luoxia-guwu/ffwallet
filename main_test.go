package main

import (
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/firefly-wallet/db"
	"github.com/filecoin-project/firefly-wallet/impl"
	"github.com/filecoin-project/firefly-wallet/mnemonic"
	"github.com/filecoin-project/lotus/chain/types"
	"os"
	"testing"
)

func testinit() error {

	os.Setenv(repoENV, "/home/bajie/work/wallet/ffwallet/")
	if err := _initDb(); err != nil {
		fmt.Printf("初始化DB失败，err: %v\n", err)
		return err
	}

	encryptText, err := localdb.Get(db.KeyCommon, encryptKey)
	if err != nil {
		fmt.Printf("读取化DB失败，err: %v\n", err)
		return err
	}

	p := "@1"
	localMnenoic, err = mnemonic.Decrypt(encryptText, []byte(p))
	if err != nil {
		fmt.Printf("读取助记词失败，err: %v\n", err)
		return err
	}

	if valid := impl.VerifyPassword(string(localMnenoic), 0); !valid {
		return fmt.Errorf("密码错误！")
	}
	return nil

}
func TestGenerateOwnerKeyBatch(t *testing.T) {

	miners := []string{"f0122533 ", "f02420 ", "f0144528 ", "f0144530 ", "f0129422 ", "f0148452 ", "f088290 ", "f0161819 ", "f021695 ", "f021704 ", "f0419945 ", "f0601583 ", "f0104398 ", "f0402822 ", "f044315 ", "f0746945 ", "f0419944 ", "f0130686 ", "f099132 ", "f0674600 ", "f0592088 ", "f0515389 ", "f01111831 ", "f01115279 ", "f01105647 ", "f01402131 ", "f055446 ", "f0117450 ", "f0464884 ", "f0465677 ", "f0734053 ", "f0748137 ", "f0748101 ", "f0724097 ", "f0839084 ", "ff0839133 ", "f01208526"}

	err := testinit()
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, miner := range miners {
		createAddress(false, true, miner, string(db.OwnerAddr))
	}
}

func TestBatchGenerateWorkAddr(t *testing.T) {

}

func TestListAllAddrs(t *testing.T) {

	err := testinit()
	if err != nil {
		fmt.Println(err)
		return
	}

	minerId := ""
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
}

func TestFIL(t *testing.T) {
	fil:=types.FIL{}
	if fil.Int==nil{
		fmt.Println("fil = nil")
	}else{
		fmt.Println("fil != nil")
	}
	fil.Short()

}