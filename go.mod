module github.com/filecoin-project/firefly-wallet

go 1.15

require (
	github.com/btcsuite/btcd v0.22.1
	github.com/btcsuite/btcutil v1.0.3-0.20201208143702-a53e38424cce
	github.com/ethereum/go-ethereum v1.10.4
	github.com/fatih/color v1.13.0
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20200910194244-f640612a1a1f
	github.com/filecoin-project/go-address v1.0.0
	github.com/filecoin-project/go-bitfield v0.2.4
	github.com/filecoin-project/go-fil-markets v1.24.0-v17
	github.com/filecoin-project/go-state-types v0.9.8
	github.com/filecoin-project/lotus v1.18.0
	github.com/filecoin-project/specs-actors/v2 v2.3.6
	github.com/filecoin-project/specs-actors/v5 v5.0.6
	github.com/filecoin-project/specs-actors/v6 v6.0.2
	github.com/gdamore/tcell/v2 v2.4.0 // indirect
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/howeyc/gopass v0.0.0-20190910152052-7cb4b85ec19c
	github.com/ipfs/go-cid v0.2.0
	github.com/ipfs/go-cidutil v0.1.0
	github.com/ipfs/go-ipld-cbor v0.0.6
	github.com/libp2p/go-libp2p-core v0.20.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-multibase v0.1.1
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	github.com/tyler-smith/go-bip39 v1.1.0
	github.com/urfave/cli/v2 v2.8.1
	github.com/whyrusleeping/cbor-gen v0.0.0-20220514204315-f29c37e9c44c
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e
	golang.org/x/xerrors v0.0.0-20220609144429-65e65417b02f
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi

replace github.com/ipfs/go-ipld-cbor v0.0.6-0.20211211231443-5d9b9e1f6fa8 => github.com/ipfs/go-ipld-cbor v0.0.5
