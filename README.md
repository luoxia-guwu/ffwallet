<h1 align="center">firefly-wallet </h1>

<!--
<p align="center">
  <a href="https://circleci.com/gh/filecoin-project/lotus"><img src="https://circleci.com/gh/filecoin-project/lotus.svg?style=svg"></a>
  <a href="https://codecov.io/gh/filecoin-project/lotus"><img src="https://codecov.io/gh/filecoin-project/lotus/branch/master/graph/badge.svg"></a>
  <a href="https://goreportcard.com/report/github.com/filecoin-project/lotus"><img src="https://goreportcard.com/badge/github.com/filecoin-project/lotus" /></a>  
  <a href=""><img src="https://img.shields.io/badge/golang-%3E%3D1.15.5-blue.svg" /></a>
  <br>
</p>
-->

lotus-wallet-tool用于管理客户钱包账户，可以创建钱包，支持转账，miner提现，设置owner，worker key等等。

## 目录

- [项目说明](#项目说明)
- [安装](#安装)
- [使用](#使用)


## 安装

make 

## 使用
- 本工具一期只提供了钱包初始话助记词命令，转账命令，提现命令，创建钱包命令，导出钱包私钥命令，后续会陆续上线其他命令。
- 命令的使用请参考./lotus-wallet-tool -h 使用。
- 注意：第一次使用,请务必执行init命令，并且指定助记词，后续执行init将会覆盖之保存的助记词。

