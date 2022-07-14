package main

import (
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	mbuildin "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"os"
	"strconv"
	"strings"
	"time"
)

var sectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "sectors manager",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		sectorsExtendCmd,
		sectorsExpiredCmd,
	},
}

var sectorsExtendCmd = &cli.Command{
	Name:      "extend",
	Usage:     "Extend miner`s sector expiration",
	ArgsUsage: "<sectorNumbers...>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "miner",
			Usage:    "miner id .eg(f02420)",
			Required: true,
		},
		&cli.Int64Flag{
			Name:     "new-expiration",
			Usage:    "new expiration epoch",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "v1-sectors",
			Usage:    "renews all v1 sectors up to the maximum possible lifetime",
			Required: false,
		},
		&cli.Int64Flag{
			Name:     "tolerance",
			//Value:    20160,
			Value:    160,
			Usage:    "when extending v1 sectors, don't try to extend sectors by fewer than this number of epochs",
			Required: false,
		},
		&cli.Int64Flag{
			Name:     "expiration-ignore",
			Value:    120,
			Usage:    "when extending v1 sectors, skip sectors whose current expiration is less than <ignore> epochs from now",
			Required: false,
		},
		&cli.Int64Flag{
			Name:     "expiration-cutoff",
			Usage:    "when extending v1 sectors, skip sectors whose current expiration is more than <cutoff> epochs from now (infinity if unspecified)",
			Required: false,
		},
		&cli.StringFlag{},
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

		api, nCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Println(err)
			return err
		}
		defer nCloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := address.NewFromString(cctx.String("miner"))
		if err != nil {
			fmt.Printf("输入miner ID(%s)不正确。 %v\n", cctx.Args().First(), err)
			return err
		}

		var params []miner5.ExtendSectorExpirationParams

		if cctx.Bool("v1-sectors") {

			head, err := api.ChainHead(ctx)
			if err != nil {
				fmt.Println(err)
				return err
			}

			nv, err := api.StateNetworkVersion(ctx, types.EmptyTSK)
			if err != nil {
				fmt.Println(err)
				return err
			}

			extensions := map[miner.SectorLocation]map[abi.ChainEpoch][]uint64{}

			// are given durations within tolerance epochs
			withinTolerance := func(a, b abi.ChainEpoch) bool {
				diff := a - b
				if diff < 0 {
					diff = b - a
				}

				return diff <= abi.ChainEpoch(cctx.Int64("tolerance"))
			}
			fmt.Println("tolerance = ",abi.ChainEpoch(cctx.Int64("tolerance")))

			sis, err := api.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
			if err != nil {
				fmt.Printf("getting miner sector infos: %v\n", err)
				return xerrors.Errorf("getting miner sector infos: %w", err)
			}

			fmt.Printf("total v1 sectors count: %d\n", len(sis))
			for _, si := range sis {

				// TODO just test
				if si.SectorNumber!=1951069{
					continue
				}

				if si.SealProof >= abi.RegisteredSealProof_StackedDrg2KiBV1_1 {
					fmt.Println(si.SealProof)
					continue
				}

				if si.Expiration < (head.Height() + abi.ChainEpoch(cctx.Int64("expiration-ignore"))) {
					fmt.Println("si.Expiration = ",si.Expiration," right = ",head.Height() + abi.ChainEpoch(cctx.Int64("expiration-ignore")))
					continue
				}

				if cctx.IsSet("expiration-cutoff") {
					fmt.Println("set expiration-cutoff")
					if si.Expiration > (head.Height() + abi.ChainEpoch(cctx.Int64("expiration-cutoff"))) {
						continue
					}
				}

				ml := policy.GetSectorMaxLifetime(si.SealProof, nv)
				// if the sector's missing less than "tolerance" of its maximum possible lifetime, don't bother extending it
				if withinTolerance(si.Expiration-si.Activation, ml) {
					fmt.Println("si.Expiration = ",si.Expiration," si.Activation = ",si.Activation," ml = ",ml)
					continue
				}

				// Set the new expiration to 48 hours less than the theoretical maximum lifetime
				newExp := ml - abi.ChainEpoch(miner5.WPoStProvingPeriod * 2) + si.Activation
				if withinTolerance(si.Expiration, newExp) || si.Expiration >= newExp {
					fmt.Println("si.Expiration = ",si.Expiration," newExp = ",newExp)
					continue
				}

				p, err := api.StateSectorPartition(ctx, maddr, si.SectorNumber, types.EmptyTSK)
				if err != nil {
					fmt.Printf("getting sector location for sector %d: %v\n", si.SectorNumber, err)
					return xerrors.Errorf("getting sector location for sector %d: %w", si.SectorNumber, err)
				}

				if p == nil {
					fmt.Printf("sector %d not found in any partition", si.SectorNumber)
					return xerrors.Errorf("sector %d not found in any partition", si.SectorNumber)
				}

				fmt.Println("find want extend sector: ",si.SectorNumber)
				es, found := extensions[*p]
				if !found {
					ne := make(map[abi.ChainEpoch][]uint64)
					ne[newExp] = []uint64{uint64(si.SectorNumber)}
					extensions[*p] = ne
				} else {
					added := false
					for exp := range es {
						if withinTolerance(exp, newExp) && newExp >= exp && exp > si.Expiration {
							es[exp] = append(es[exp], uint64(si.SectorNumber))
							added = true
							break
						}
					}

					if !added {
						es[newExp] = []uint64{uint64(si.SectorNumber)}
					}
				}
			}

			p := miner5.ExtendSectorExpirationParams{}
			scount := 0

			for l, exts := range extensions {
				for newExp, numbers := range exts {
					fmt.Println("numbers = ",numbers," newExp = ",newExp)
					scount += len(numbers)
					addressedMax, err := policy.GetAddressedSectorsMax(nv)
					if err != nil {
						fmt.Printf("failed to get addressed sectors max")
						return xerrors.Errorf("failed to get addressed sectors max")
					}
					declMax, err := policy.GetDeclarationsMax(nv)
					if err != nil {
						fmt.Println("failed to get declarations max")
						return xerrors.Errorf("failed to get declarations max")
					}
					if scount > addressedMax || len(p.Extensions) == declMax {
						params = append(params, p)
						p = miner5.ExtendSectorExpirationParams{}
						scount = len(numbers)
					}

					p.Extensions = append(p.Extensions, miner5.ExpirationExtension{
						Deadline:      l.Deadline,
						Partition:     l.Partition,
						Sectors:       bitfield.NewFromSet(numbers),
						NewExpiration: newExp,
					})
				}
			}

			// if we have any sectors, then one last append is needed here
			if scount != 0 {
				params = append(params, p)
			}

		} else {
			if !cctx.Args().Present() || !cctx.IsSet("new-expiration") {
				return xerrors.Errorf("must pass at least one sector number and new expiration")
			}
			sectors := map[miner.SectorLocation][]uint64{}

			for i, s := range cctx.Args().Slice() {
				id, err := strconv.ParseUint(s, 10, 64)
				if err != nil {
					return xerrors.Errorf("could not parse sector %d: %w", i, err)
				}

				p, err := api.StateSectorPartition(ctx, maddr, abi.SectorNumber(id), types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("getting sector location for sector %d: %w", id, err)
				}

				if p == nil {
					return xerrors.Errorf("sector %d not found in any partition", id)
				}

				sectors[*p] = append(sectors[*p], id)
			}

			p := miner5.ExtendSectorExpirationParams{}
			for l, numbers := range sectors {

				// TODO: Dedup with above loop
				p.Extensions = append(p.Extensions, miner5.ExpirationExtension{
					Deadline:      l.Deadline,
					Partition:     l.Partition,
					Sectors:       bitfield.NewFromSet(numbers),
					NewExpiration: abi.ChainEpoch(cctx.Int64("new-expiration")),
				})
			}

			params = append(params, p)
		}

		if len(params) == 0 {
			fmt.Println("nothing to extend")
			return nil
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		for i := range params {
			sp, aerr := actors.SerializeParams(&params[i])
			if aerr != nil {
				return xerrors.Errorf("serializing params: %w", err)
			}

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				From:   mi.Worker,
				To:     maddr,
				Method: mbuildin.MethodsMiner.ExtendSectorExpiration,

				Value:  big.Zero(),
				Params: sp,
			}, nil)
			if err != nil {
				return xerrors.Errorf("mpool push message: %w", err)
			}

			fmt.Println(smsg.Cid())
		}

		return nil
	},
}

var sectorsExpiredCmd = &cli.Command{
	Name:      "expired",
	Usage:     "Query the sector set expired deadline of a miner and export to file",
	ArgsUsage: "[minerAddress,outFile]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "ignore-expired",
			Usage: "ignore all had expired sectors",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		api, nCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			fmt.Println(err)
			return err
		}
		defer nCloser()

		ctx := lcli.ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify miner to list sectors for")
		}

		outFile := cctx.Args().Get(1)
		if len(outFile) == 0 {
			return fmt.Errorf("must specify out file")
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		//sectors, err := api.StateMinerSectors(ctx, maddr, nil, ts.Key())
		sectors, err := api.StateMinerActiveSectors(ctx, maddr, ts.Key())
		if err != nil {
			return err
		}

		if len(sectors)<0{
			fmt.Println("no sectors ")
			return  nil
		}
		pt:=sectors[0].SealProof

		f, err := os.OpenFile(outFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
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

		_, err = f.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\n", "扇区编号", "生效高度", "到期高度", "生效时间", "到期时间","proof版本"))
		if err != nil {
			fmt.Println(err)
			return err
		}

		v1SectorsCount:=0
		v1_1SectorsCount:=0

		proofVer:= func(proof abi.RegisteredSealProof) string{
			if proof >= abi.RegisteredSealProof_StackedDrg2KiBV1_1 {
				v1_1SectorsCount++
				return "v1_1(MaxExtend 5years)"
			}
			v1SectorsCount++
			return "v1(MaxExtend 540days)"
		}

		curTime:=time.Now().Format("2006-01-02 04:05")
		for _, s := range sectors {
			if cctx.Bool("ignore-expired") && strings.Compare(curTime,EpochTimeHuman(ts.Height(),s.Expiration))>0{
				continue
			}
			_, err := f.WriteString(fmt.Sprintf("%d\t%d\t%d\t%s\t%s\t%s\n",
				s.SectorNumber, s.Activation, s.Expiration, EpochTimeHuman(ts.Height(), s.Activation), EpochTimeHuman(ts.Height(), s.Expiration),proofVer(s.SealProof)))
			if err != nil {
				fmt.Println(err)
				return err
			}
		}

		fmt.Println("有效sectors：", len(sectors),"(",sectorsCountToGTP(uint64(len(sectors)),pt),")")
		fmt.Printf("v1 sectors: %d(%s)  v1_1 sectors: %d(%s) \n",
			v1SectorsCount,sectorsCountToGTP(uint64(v1SectorsCount),pt),v1_1SectorsCount,sectorsCountToGTP(uint64(v1_1SectorsCount),pt))

		return nil
	},
}