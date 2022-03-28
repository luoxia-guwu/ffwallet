package bip39

import (
	"fmt"
	"github.com/tyler-smith/go-bip39"
	"io/ioutil"
	"testing"
)

func TestName(t *testing.T) {
	entropy, _ := bip39.NewEntropy(128)
	mnemonic, _ := bip39.NewMnemonic(entropy)

	// Generate a Bip32 HD wallet for the mnemonic and a user supplied password
	//seed := bip39.NewSeed(mnemonic, "Secret Passphrase")

	//masterKey, _ := bip32.NewMasterKey(seed)
	//publicKey := masterKey.PublicKey()

	// Display mnemonic and keys
	err := ioutil.WriteFile("./k.txt", []byte(mnemonic), 0655)
	if err != nil {
		return
	}
	fmt.Println("Mnemonic: ", mnemonic)

	//fmt.Println("Master private key: ", masterKey)
	//fmt.Println("Master public key: ", publicKey)
}

func TestWirteFile(t *testing.T) {
	k256 := ""
	err := ioutil.WriteFile("./k1.txt", []byte(k256), 0655)
	if err != nil {
		return
	}
}
