package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"mitosis/contracts"
	"os"
	"strings"
	"time"

	"mitosis/bind"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/term"
)

var (
	mainContract = common.HexToAddress("0x3267e72Dc8780A1512fa69DA7759eC66f30350E3")
	ethRPC       = "https://rpc.sepolia.org"
	arbRPC       = "https://sepolia-rollup.arbitrum.io/rpc"
	opRPC        = "https://sepolia.optimism.io/"
	baseRPC      = "https://sepolia.base.org"
	lineaRPC     = "https://linea-sepolia-rpc.publicnode.com"
)

func getBalanceAndDecimals(contract, account common.Address, p *ethclient.Client) (*big.Int, error) {
	c, err := contracts.NewContract(contract, p)
	if err != nil {
		return nil, err
	}

	balance, err := c.BalanceOf(nil, account)
	if err != nil {
		return nil, err
	}
	return balance, nil
}

func approve(contract, spender common.Address, p *ethclient.Client,
	chainId *big.Int, pk *ecdsa.PrivateKey) (*types.Transaction, error) {
	c, err := contracts.NewContract(contract, p)
	if err != nil {
		return nil, err
	}

	opts, err := bind.NewKeyedTransactorWithChainID(pk, chainId)
	if err != nil {
		return nil, err
	}

	return c.Approve(opts, spender, math.MaxBig256)
}

func needApprove(balance *big.Int, contract, owner, spender common.Address, p *ethclient.Client) (bool, error) {

	c, err := contracts.NewContract(contract, p)
	if err != nil {
		return false, err
	}
	a, err := c.Allowance(nil, owner, spender)
	if err != nil {
		return false, err
	}
	return a.Cmp(balance) <= 0, nil
}

type vault struct {
	name     string
	asset    common.Address
	contract common.Address
	networks []string
}

type account struct {
	pk      *ecdsa.PrivateKey
	address common.Address
}

func main() {

	vs, err := readVaultsFromFile("./vaults.json")
	if err != nil {
		panic(err)
	}

	cs := setupClients()

	fmt.Print("Enter your Private Key: ")
	privateKey, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}

	fmt.Println()
	pk, err := crypto.HexToECDSA(string(privateKey))
	if err != nil {
		panic(err)
	}

	a := account{
		pk:      pk,
		address: crypto.PubkeyToAddress(pk.PublicKey),
	}
	fmt.Println("account address: ", a.address)

	for _, v := range vs {
		for _, network := range v.networks {
			time.Sleep(3 * time.Second)

			client := cs[network]
			balance, err := getBalanceAndDecimals(v.asset, a.address, client.c)
			if err != nil {
				fmt.Println(fmt.Errorf("get balance %s:%s (%s)", v.name, network, err))
				fmt.Println()
				continue
			}

			if balance.Cmp(big.NewInt(100)) == 1 {

				fmt.Printf("balance of %s on %s is %s\n", v.name, network, balance.String())
				need, err := needApprove(balance, v.asset, a.address, mainContract, client.c)
				if err != nil {
					fmt.Println(fmt.Errorf("need approve %s:%s (%s)", v.name, network, err))
					fmt.Println()
					continue
				}
				if need {
					tx, err := approve(v.asset, mainContract, client.c, client.chainId, a.pk)
					if err != nil {
						fmt.Println(fmt.Errorf("approve %s:%s (%s)", v.name, network, err))
						fmt.Println()
						continue
					}
					fmt.Println("approve tx: ", tx.Hash())
					time.Sleep(5 * time.Second)
				}
				balance = big.NewInt(0).Sub(balance, big.NewInt(10))
				data := buildData(v.asset, a.address, v.contract, balance)

				bound := bind.NewBoundContract(mainContract, abi.ABI{}, client.c, client.c, client.c)
				opts, err := bind.NewKeyedTransactorWithChainID(pk, client.chainId)
				if err != nil {
					fmt.Println(fmt.Errorf("create opts %s:%s (%s)", v.name, network, err))
					fmt.Println()
					continue
				}

				for attempt := 0; attempt < 5; attempt++ {
					tx, err := bound.RawTransact(opts, data)
					if err == nil {
						fmt.Printf("%s %s deposited on %s tx: %s\n", balance.String(), v.name, network, tx.Hash())
						fmt.Println()
						break
					}

					if strings.Contains(err.Error(), "execution reverted") {
						fmt.Printf("transaction failed (attempt %d): %s\n", attempt+1, err)
						time.Sleep(5 * time.Second)
						continue
					} else {

						fmt.Println(fmt.Errorf("raw transact %s:%s (%s)", v.name, network, err))
						fmt.Println()
						break
					}
				}
			}
		}
	}
	fmt.Println("ALL DONE!")
}

func buildData(asset, account, messanger common.Address, amount *big.Int) []byte {
	s := strings.Join([]string{"62e4c545", common.HexToHash(asset.String()).String()[2:],
		common.HexToHash(account.String()).String()[2:],
		common.HexToHash(messanger.String()).String()[2:],
		common.BigToHash(amount).String()[2:]}, "")

	data, _ := hex.DecodeString(s)
	return data
}

func readVaultsFromFile(filePath string) ([]vault, error) {

	type v struct {
		Name     string   `json:"name"`
		Asset    string   `json:"asset"`
		Contract string   `json:"contract"`
		Networks []string `json:"networks"`
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	byteValue, _ := io.ReadAll(file)

	var vs []v
	err = json.Unmarshal(byteValue, &vs)
	if err != nil {
		return nil, err
	}

	var vaults []vault
	for _, v := range vs {
		vaults = append(vaults, vault{
			name:     v.Name,
			asset:    common.HexToAddress(v.Asset),
			contract: common.HexToAddress(v.Contract),
			networks: v.Networks,
		})

	}

	return vaults, nil
}

type client struct {
	c       *ethclient.Client
	chainId *big.Int
}

func setupClients() map[string]client {
	cs := make(map[string]client)

	ethC, err := ethclient.Dial(ethRPC)
	if err != nil {
		panic(err)
	}

	ethCID, err := ethC.ChainID(context.Background())
	if err != nil {
		panic(err)
	}
	cs["eth"] = client{
		c:       ethC,
		chainId: ethCID,
	}

	arbC, err := ethclient.Dial(arbRPC)
	if err != nil {
		panic(err)
	}

	arbCID, err := arbC.ChainID(context.Background())
	if err != nil {
		panic(err)
	}
	cs["arb"] = client{
		c:       arbC,
		chainId: arbCID,
	}

	opC, err := ethclient.Dial(opRPC)
	if err != nil {
		panic(err)
	}

	opCID, err := opC.ChainID(context.Background())
	if err != nil {
		panic(err)
	}
	cs["op"] = client{
		c:       opC,
		chainId: opCID,
	}

	baseC, err := ethclient.Dial(baseRPC)
	if err != nil {
		panic(err)
	}

	baseCID, err := baseC.ChainID(context.Background())
	if err != nil {
		panic(err)
	}
	cs["base"] = client{
		c:       baseC,
		chainId: baseCID,
	}

	lineaC, err := ethclient.Dial(lineaRPC)
	if err != nil {
		panic(err)
	}

	lineaCID, err := lineaC.ChainID(context.Background())
	if err != nil {
		panic(err)
	}
	cs["linea"] = client{
		c:       lineaC,
		chainId: lineaCID,
	}

	return cs
}
