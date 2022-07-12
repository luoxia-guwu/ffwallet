module github.com/filecoin-project/firefly-wallet

go 1.15

require (
	github.com/btcsuite/btcd v0.22.0-beta
	github.com/btcsuite/btcutil v1.0.3-0.20201208143702-a53e38424cce
	github.com/ethereum/go-ethereum v1.10.4
	github.com/fatih/color v1.13.0
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20200910194244-f640612a1a1f
	github.com/filecoin-project/go-address v0.0.6
	github.com/filecoin-project/go-bitfield v0.2.4
	github.com/filecoin-project/go-state-types v0.1.10
	github.com/filecoin-project/lotus v1.16.0
	github.com/filecoin-project/specs-actors/v2 v2.3.6
	github.com/filecoin-project/specs-actors/v5 v5.0.6
	github.com/filecoin-project/specs-actors/v6 v6.0.2
	github.com/gdamore/tcell/v2 v2.4.0 // indirect
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/howeyc/gopass v0.0.0-20190910152052-7cb4b85ec19c
	github.com/ipfs/go-cid v0.1.0
	github.com/ipfs/go-ipld-cbor v0.0.6
	github.com/mitchellh/go-homedir v1.1.0
	github.com/syndtr/goleveldb v1.0.1-0.20210305035536-64b5b1c73954
	github.com/tyler-smith/go-bip39 v1.1.0
	github.com/urfave/cli/v2 v2.3.0
	github.com/whyrusleeping/cbor-gen v0.0.0-20220323183124-98fa8256a799
	golang.org/x/crypto v0.0.0-20220411220226-7b82a4e95df4
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi

replace github.com/ipfs/go-ipld-cbor v0.0.6-0.20211211231443-5d9b9e1f6fa8 => github.com/ipfs/go-ipld-cbor v0.0.5
