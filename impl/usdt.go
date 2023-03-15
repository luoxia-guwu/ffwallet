package impl

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
)


var ethPath = "m/44'/60'/0'/0/"

func CreateUsdtAddress(userId int, mnemonic string) (string, error) {
	addr := common.HexToAddress("0x0").Hex()
	dPath := fmt.Sprintf("%s%d", ethPath, userId)
	account, err := createAccount(mnemonic, dPath)
	if err != nil {
		fmt.Printf("wallet derive account err:%+v", err)
		return addr, err
	}
	return account.Address.Hex(), nil
}

