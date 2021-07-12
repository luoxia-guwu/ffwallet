package mnemonic

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"golang.org/x/crypto/scrypt"
	"io"
)

var (
	scryptN     = 1 << 18
	scryptP     = 1
	scryptR     = 8
	scryptDKLen = 32
)

type hidden struct {
	Mnemonic []byte `json:"mnemonic"`
	Iv       []byte `json:"iv"`
	Salt     []byte `json:"salt"`
}

const PlainPath string = "./data"

//func Encrypt() error {
//	passwd, err := gopass.GetPasswdMasked()
//	if err != nil {
//		return err
//	}
//	data, err := ioutil.ReadFile(PlainPath)
//	if err != nil {
//		return err
//	}
//	data = data[:len(data)-1]
//	mnemonic := string(data)
//	models.Mnemonic = mnemonic
//	ok := bip39.IsMnemonicValid(models.Mnemonic)
//	if !ok {
//		return errors.New("mnemonic is not valid")
//	}
//	usdtAddr, err := impl.CreateUsdtAddress(0)
//	if err != nil {
//		return err
//	}
//	o := orm.NewOrm()
//	userInfo := new(models.UserInfo)
//	num, err := o.QueryTable("fly_user_info").Filter("user_id", 0).All(userInfo)
//	if err != nil {
//		return err
//	}
//	if num != 0 {
//		return errors.New("mnemonic is already encode")
//	}
//	userInfo.UserId = 0
//	userInfo.UsdtAddress = usdtAddr
//	_, err = o.Insert(userInfo)
//	if err != nil {
//		return err
//	}
//	if err = encryptData(data, passwd); err != nil {
//		return err
//	}
//	if err = os.Remove(models.PlainPath); err != nil {
//		return err
//	}
//	return nil
//}

func EncryptData(data, auth []byte) ([]byte, error) {

	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		panic("reading from crypto/rand failed: " + err.Error())
	}

	derivedKey, err := scrypt.Key(auth, salt, scryptN, scryptR, scryptP, scryptDKLen)
	if err != nil {
		return []byte{}, err
	}

	encryptKey := derivedKey[:16]
	iv := make([]byte, aes.BlockSize) // 16
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic("reading from crypto/rand failed: " + err.Error())
	}

	cipherText, err := aesCTRXOR(encryptKey, data, iv)
	if err != nil {
		return []byte{}, err
	}

	hid := hidden{
		Mnemonic: cipherText,
		Iv:       iv,
		Salt:     salt,
	}

	hidData, err := json.Marshal(hid)
	if err != nil {
		return []byte{}, err
	}

	return hidData, nil
}

func aesCTRXOR(key, inText, iv []byte) ([]byte, error) {
	// AES-128 is selected due to size of encryptKey.
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(aesBlock, iv)
	outText := make([]byte, len(inText))
	stream.XORKeyStream(outText, inText)
	return outText, err
}
