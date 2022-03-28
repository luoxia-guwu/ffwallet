package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/firefly-wallet/db"
	"github.com/filecoin-project/firefly-wallet/impl"
	"github.com/filecoin-project/firefly-wallet/mnemonic"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	"io/ioutil"
	"os"
	"strings"
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

func TestSize(t *testing.T) {
	fmt.Println(sectorsCountToGTP(136613,abi.RegisteredSealProof_StackedDrg32GiBV1))
}

func TestStringsTrim(t *testing.T)  {
	ss:="96,[172 205 246 19 66 15 107 178 107 105 118 41 195 118 114 21 177 13 193 249 199 22 41 153 233 245 107 87 171 164 30 61 119 67 96 150 86 28 14 214 60 161 146 171 214 110 35 200 146 215 18 8 5 83 81 100 88 134 76 38 197 3 64 221 151 34 100 215 245 127 117 45 203 171 28 208 213 196 27 35 0 39 151 0 247 12 47 72 13 56 116 147 0 99 73 33 17 15 104 181 12 154 2 100 145 162 158 76 25 229 30 231 50 9 216 218 8 122 225 71 158 144 110 154 194 255 179 134 86 148 125 217 234 240 52 120 146 45 219 231 225 80 141 97 185 188 173 180 230 93 124 200 246 166 109 204 95 180 57 84 189 120 110 196 242 167 176 14 163 1 167 190 224 179 204 174 136 10 186 102 254 188 170 76 58 76 38 192 42 104 57 205];"

	fmt.Println(strings.Trim(ss,"[]"))
}

// 将存储的文件转换成export的字节流
func TestConvertFILAddr(t *testing.T) {

	keyPath:="./t"
	file, err := os.Open(keyPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close() //nolint: errcheck // read only op

	data, err := ioutil.ReadAll(file)
	if err != nil {
		fmt.Println(err)
		return
	}

	var res types.KeyInfo
	err = json.Unmarshal(data, &res)
	if err != nil {
		fmt.Println(err)
		return
	}

	bytes, err := json.Marshal(res)
	if err != nil {
		fmt.Println(err)
		return
	}

	encoded := hex.EncodeToString(bytes)
	fmt.Println(encoded)
}