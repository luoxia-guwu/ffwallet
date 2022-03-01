package impl

import (
	"fmt"
	"testing"
)

func TestBlsSign(t *testing.T) {
	mn := "tag volcano eight thank tide danger coast health above argue embrace heavy"

	pk, err := generateBLSPriviteKey(mn, 1)
	if err != nil {
		fmt.Printf("private key get err:%+v", err)
		return
	}
	fmt.Println(pk)

	//pk1, err := sigs.Generate(crypto.SigTypeBLS)
	//require.NoError(t, err)
	//fmt.Println(pk1)

	//ki := types.KeyInfo{
	//	Type:       types.KTBLS,
	//	PrivateKey: pk[:],
	//}
	//fmt.Println(ki)

	//k, err := wallet.NewKey(ki)
	//require.NoError(t, err)
	//fmt.Println(k)

	//p := []byte("potato")

	//si, err := sigs.Sign(crypto.SigTypeBLS, pk[:], p)
	//require.NoError(t, err)

	//err = sigs.Verify(si, k.Address, p)
	//require.NoError(t, err)
}

//func TestRoundtrip(t *testing.T) {
//	pk, err := sigs.Generate(wallet.ActSigType("bls"))
//	require.NoError(t, err)
//	fmt.Println(pk)
//
//	ki := types.KeyInfo{
//		Type:       types.KTBLS,
//		PrivateKey: pk,
//	}
//	fmt.Println(ki)
//	k, err := wallet.NewKey(ki)
//	require.NoError(t, err)
//
//	p := []byte("potato")
//
//	si, err := sigs.Sign(crypto.SigTypeBLS, pk, p)
//	require.NoError(t, err)
//
//	err = sigs.Verify(si, k.Address, p)
//	require.NoError(t, err)
//}
//
